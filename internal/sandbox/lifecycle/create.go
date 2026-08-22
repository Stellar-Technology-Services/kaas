package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// CreateInput is the input for the Create use case.
type CreateInput struct {
	Identity identity.Identity
	// Port is an optional explicit SSH port; 0 allocates from PortBase.
	Port int
}

// CreateResult is returned on successful create (or idempotent already-running).
type CreateResult struct {
	Identity       identity.Identity
	State          LifecycleState
	Created        bool // true when a new registry row was created
	AlreadyRunning bool // true when create was a no-op because the VM was already up
}

// Create validates identity, allocates a port, claims an operation, creates/starts
// the provider profile, reconciles SSH, and persists running state.
func (s *Service) Create(ctx context.Context, in CreateInput) (CreateResult, error) {
	if s.Provider == nil || s.Store == nil {
		return CreateResult{}, fmt.Errorf("lifecycle service is not configured")
	}
	if err := s.Provider.Available(ctx); err != nil {
		return CreateResult{}, err
	}
	ident := in.Identity
	name := ident.ID.Name
	if name == "" {
		return CreateResult{}, fmt.Errorf("sandbox name is required")
	}

	existing, err := s.Store.GetSandbox(name)
	if err == nil {
		// Existing rows already satisfied the name grammar when first created.
		return s.createExisting(ctx, existing, ident)
	}
	if !errors.Is(err, state.ErrNotFound) {
		return CreateResult{}, err
	}
	if err := identity.ValidateName(name); err != nil {
		return CreateResult{}, err
	}

	used, err := s.usedPorts(ctx)
	if err != nil {
		return CreateResult{}, err
	}
	sshPort := in.Port
	if sshPort == 0 {
		sshPort = ident.Endpoint.Port
	}
	if sshPort == 0 {
		sshPort, err = s.Ports.Allocate(ctx, s.PortBase, used)
		if err != nil {
			return CreateResult{}, err
		}
	} else if err := sandbox.EnsurePortFree(sshPort, used); err != nil {
		return CreateResult{}, err
	}
	ident.Endpoint.Port = sshPort
	if ident.Endpoint.Host == "" {
		ident.Endpoint.Host = identity.DefaultSSHHost
	}
	if err := identity.Validate(ident); err != nil {
		return CreateResult{}, err
	}

	op, err := s.claim(ctx, name, "create")
	if err != nil {
		return CreateResult{}, err
	}
	if err := s.Store.UpdateOperation(op.ID, state.OpRunning, ""); err != nil {
		return CreateResult{}, err
	}
	expires := s.now().Add(defaultPortReservationTTL)
	if err := s.Ports.Reserve(ctx, sshPort, name, op.ID, expires); err != nil {
		_ = s.finishOp(op, state.OpFailed, err.Error())
		return CreateResult{}, err
	}

	var sb state.Sandbox
	identity.ApplyToState(&sb, ident)
	s.setLifecycle(&sb, DesiredRunning, ObservedCreating)
	if err := s.Store.CreateSandbox(sb); err != nil {
		_ = s.releaseOpPort(ctx, sshPort, op)
		_ = s.finishOp(op, state.OpFailed, err.Error())
		return CreateResult{}, err
	}

	_, err = s.Provider.Create(ctx, CreateSpec{
		Name: name, CPUs: ident.Resources.CPUs, Memory: ident.Resources.MemoryGiB,
		Disk: ident.Resources.DiskGiB, Arch: ident.Resources.Arch, SSHPort: sshPort,
	})
	if err != nil {
		sb.DesiredState = string(DesiredRunning)
		sb.ObservedState = string(ObservedFailed)
		if upErr := s.Store.UpdateSandbox(sb); upErr != nil {
			_ = s.releaseOpPort(ctx, sshPort, op)
			_ = s.finishOp(op, state.OpFailed, err.Error())
			if recErr := s.record(ctx, name, "create", "failed", err.Error()); recErr != nil {
				return CreateResult{}, errors.Join(err, upErr, recErr)
			}
			return CreateResult{}, errors.Join(err, upErr)
		}
		_ = s.releaseOpPort(ctx, sshPort, op)
		if finErr := s.finishOp(op, state.OpFailed, err.Error()); finErr != nil {
			if recErr := s.record(ctx, name, "create", "failed", err.Error()); recErr != nil {
				return CreateResult{}, errors.Join(err, finErr, recErr)
			}
			return CreateResult{}, errors.Join(err, finErr)
		}
		if recErr := s.record(ctx, name, "create", "failed", err.Error()); recErr != nil {
			return CreateResult{}, errors.Join(err, recErr)
		}
		return CreateResult{}, fmt.Errorf("provider create failed (sandbox left registered; retry or destroy %s): %w", name, err)
	}

	if reconciled, sshErr := s.reconcileSSH(ctx, ident, name); sshErr == nil {
		ident = reconciled
		identity.ApplyToState(&sb, ident)
	}
	s.setLifecycle(&sb, DesiredRunning, ObservedRunning)
	if err := s.Store.UpdateSandbox(sb); err != nil {
		_ = s.releaseOpPort(ctx, sshPort, op)
		_ = s.finishOp(op, state.OpFailed, err.Error())
		return CreateResult{}, err
	}
	if err := s.releaseOpPort(ctx, sshPort, op); err != nil {
		return CreateResult{}, err
	}
	if err := s.finishOp(op, state.OpSucceeded, fmt.Sprintf("port=%d", sb.SSHPort)); err != nil {
		return CreateResult{}, err
	}
	if err := s.record(ctx, name, "create", "ok", fmt.Sprintf("port=%d", sb.SSHPort)); err != nil {
		return CreateResult{}, err
	}
	return CreateResult{
		Identity: ident,
		State:    LifecycleState{Desired: DesiredRunning, Observed: ObservedRunning},
		Created:  true,
	}, nil
}

func (s *Service) createExisting(ctx context.Context, existing *state.Sandbox, ident identity.Identity) (CreateResult, error) {
	name := existing.Name
	observed, st, err := s.providerObserved(ctx, name)
	if err != nil {
		return CreateResult{}, err
	}
	if observed.IsRunning() {
		loaded, _ := identity.FromState(*existing)
		if reconciled, sshErr := s.reconcileSSH(ctx, loaded, name); sshErr == nil {
			loaded = reconciled
			identity.ApplyToState(existing, loaded)
		}
		if err := s.updateLifecycle(existing, DesiredRunning, ObservedRunning); err != nil {
			return CreateResult{}, err
		}
		if err := s.record(ctx, name, "create", "ok", "already running"); err != nil {
			return CreateResult{}, err
		}
		return CreateResult{
			Identity:       loaded,
			State:          LifecycleState{Desired: DesiredRunning, Observed: ObservedRunning},
			Created:        false,
			AlreadyRunning: true,
		}, nil
	}
	if !st.Exists {
		return CreateResult{}, fmt.Errorf("sandbox %q is registered but provider profile is absent; destroy or recreate", name)
	}
	// Start existing stopped (or transitional) profile.
	if err := s.updateLifecycle(existing, DesiredRunning, ObservedStarting); err != nil {
		return CreateResult{}, err
	}
	if err := s.Provider.Start(ctx, startSpecFromSandbox(existing)); err != nil {
		_ = s.updateLifecycle(existing, DesiredRunning, ObservedFailed)
		if recErr := s.record(ctx, name, "create", "failed", err.Error()); recErr != nil {
			return CreateResult{}, errors.Join(err, recErr)
		}
		return CreateResult{}, err
	}
	loaded, _ := identity.FromState(*existing)
	if reconciled, sshErr := s.reconcileSSH(ctx, loaded, name); sshErr == nil {
		loaded = reconciled
		identity.ApplyToState(existing, loaded)
	}
	if err := s.updateLifecycle(existing, DesiredRunning, ObservedRunning); err != nil {
		return CreateResult{}, err
	}
	if err := s.record(ctx, name, "create", "ok", "started existing"); err != nil {
		return CreateResult{}, err
	}
	_ = ident
	return CreateResult{
		Identity: loaded,
		State:    LifecycleState{Desired: DesiredRunning, Observed: ObservedRunning},
		Created:  false,
	}, nil
}
