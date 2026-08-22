package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// ForkInput configures the Fork use case (D4).
type ForkInput struct {
	Source string
	Target string
}

// ForkResult is returned on successful fork (including partial source-restore failure).
type ForkResult struct {
	Source string
	Target identity.Identity
	State  LifecycleState
}

// Fork clones source to target and restores the source to its prior observed state (D4).
func (s *Service) Fork(ctx context.Context, in ForkInput) (ForkResult, error) {
	if s.Provider == nil || s.Store == nil {
		return ForkResult{}, fmt.Errorf("lifecycle service is not configured")
	}
	if in.Source == "" || in.Target == "" {
		return ForkResult{}, fmt.Errorf("source and target names are required")
	}
	if in.Source == in.Target {
		return ForkResult{}, fmt.Errorf("source and new sandbox name must differ")
	}
	targetID, err := identity.ParseSandboxID(in.Target)
	if err != nil {
		return ForkResult{}, fmt.Errorf("new sandbox name: %w", err)
	}
	target := targetID.Name
	source := in.Source

	if err := s.Provider.Available(ctx); err != nil {
		return ForkResult{}, err
	}

	srcSB, err := s.loadSandbox(source)
	if err != nil {
		return ForkResult{}, err
	}
	srcIdent, err := identity.FromState(*srcSB)
	if err != nil {
		return ForkResult{}, err
	}
	if _, err := s.Store.GetSandbox(target); err == nil {
		return ForkResult{}, fmt.Errorf("sandbox %q already exists", target)
	} else if !errors.Is(err, state.ErrNotFound) {
		return ForkResult{}, err
	}

	srcObserved, srcStatus, err := s.providerObserved(ctx, source)
	if err != nil {
		return ForkResult{}, err
	}
	if !srcStatus.Exists {
		return ForkResult{}, fmt.Errorf("provider profile %q not found", source)
	}
	priorObserved := srcObserved
	if priorObserved != ObservedRunning && priorObserved != ObservedStopped {
		// Normalize transitional/unknown to a restorable prior: treat running-like as running.
		if srcStatus.Status != "" && ObservedFromProvider(srcStatus.Status, true) == ObservedRunning {
			priorObserved = ObservedRunning
		} else {
			priorObserved = ObservedStopped
		}
	}

	tgtStatus, err := s.Provider.Status(ctx, target)
	if err != nil {
		return ForkResult{}, err
	}
	if tgtStatus.Exists {
		return ForkResult{}, fmt.Errorf("provider profile %q already exists", target)
	}

	used, err := s.usedPorts(ctx)
	if err != nil {
		return ForkResult{}, err
	}
	op, err := s.claim(ctx, target, "fork")
	if err != nil {
		return ForkResult{}, err
	}
	if err := s.Store.UpdateOperation(op.ID, state.OpRunning, fmt.Sprintf("source=%s", source)); err != nil {
		return ForkResult{}, err
	}
	port, err := s.Ports.Allocate(ctx, s.PortBase, used)
	if err != nil {
		_ = s.finishOp(op, state.OpFailed, err.Error())
		return ForkResult{}, err
	}
	if err := s.Ports.Reserve(ctx, port, target, op.ID, s.now().Add(defaultPortReservationTTL)); err != nil {
		_ = s.finishOp(op, state.OpFailed, err.Error())
		return ForkResult{}, err
	}

	stoppedForClone := false
	if !srcObserved.IsStopped() {
		if err := s.Provider.Stop(ctx, source); err != nil {
			_ = s.releaseOpPort(ctx, port, op)
			_ = s.finishOp(op, state.OpFailed, err.Error())
			return ForkResult{}, fmt.Errorf("stop source sandbox %q: %w", source, err)
		}
		stoppedForClone = true
		_ = s.updateLifecycle(srcSB, DesiredState(srcSB.DesiredState), ObservedStopped)
	}

	if err := s.Provider.Clone(ctx, source, target, port); err != nil {
		_ = s.Provider.Delete(ctx, target, true)
		restoreErr := s.restoreSource(ctx, srcSB, priorObserved, stoppedForClone)
		_ = s.releaseOpPort(ctx, port, op)
		_ = s.finishOp(op, state.OpFailed, err.Error())
		if restoreErr != nil {
			return ForkResult{}, errors.Join(fmt.Errorf("clone sandbox %q: %w", source, err), restoreErr)
		}
		return ForkResult{}, fmt.Errorf("clone sandbox %q: %w", source, err)
	}

	dstIdent := identity.FromFork(srcIdent, targetID, identity.Endpoint{Port: port})
	if reconciled, sshErr := s.reconcileSSH(ctx, dstIdent, target); sshErr == nil {
		dstIdent = reconciled
	} else {
		_ = s.Provider.Delete(ctx, target, true)
		_ = s.restoreSource(ctx, srcSB, priorObserved, stoppedForClone)
		_ = s.releaseOpPort(ctx, port, op)
		_ = s.finishOp(op, state.OpFailed, sshErr.Error())
		return ForkResult{}, sshErr
	}

	dst := state.Sandbox{
		LastProvisionedAt: srcSB.LastProvisionedAt,
	}
	identity.ApplyToState(&dst, dstIdent)
	dst.EgressMode, dst.EgressStatus, dst.EgressCAFingerprint, dst.EgressPolicyHash = "", "", "", ""
	dst.EgressPort = 0
	dst.SetupStatus = ""
	dst.SetupCompletedAt = nil
	s.setLifecycle(&dst, DesiredRunning, ObservedRunning)
	if err := s.Store.CreateSandbox(dst); err != nil {
		_ = s.Provider.Delete(ctx, target, true)
		_ = s.restoreSource(ctx, srcSB, priorObserved, stoppedForClone)
		_ = s.releaseOpPort(ctx, port, op)
		_ = s.finishOp(op, state.OpFailed, err.Error())
		return ForkResult{}, fmt.Errorf("register fork: %w", err)
	}

	if err := s.releaseOpPort(ctx, port, op); err != nil {
		return ForkResult{}, err
	}
	if err := s.finishOp(op, state.OpSucceeded, fmt.Sprintf("source=%s port=%d", source, dst.SSHPort)); err != nil {
		return ForkResult{}, err
	}
	if err := s.record(ctx, target, "fork", "ok", fmt.Sprintf("source=%s port=%d", source, dst.SSHPort)); err != nil {
		return ForkResult{}, err
	}
	if err := s.record(ctx, source, "fork", "ok", fmt.Sprintf("target=%s", target)); err != nil {
		return ForkResult{}, err
	}

	if err := s.restoreSource(ctx, srcSB, priorObserved, stoppedForClone); err != nil {
		recovery := fmt.Sprintf("kaas start %s", source)
		if priorObserved.IsStopped() {
			recovery = fmt.Sprintf("kaas stop %s", source)
		}
		return ForkResult{
			Source: source,
			Target: dstIdent,
			State:  LifecycleState{Desired: DesiredRunning, Observed: ObservedRunning},
		}, &PartialError{
			Source:         source,
			Target:         target,
			Expected:       priorObserved,
			Cause:          err,
			RecoveryAction: recovery,
		}
	}

	return ForkResult{
		Source: source,
		Target: dstIdent,
		State:  LifecycleState{Desired: DesiredRunning, Observed: ObservedRunning},
	}, nil
}

func (s *Service) restoreSource(ctx context.Context, srcSB *state.Sandbox, prior ObservedState, stoppedForClone bool) error {
	name := srcSB.Name
	switch prior {
	case ObservedRunning:
		if err := s.Provider.Start(ctx, startSpecFromSandbox(srcSB)); err != nil {
			_ = s.updateLifecycle(srcSB, DesiredRunning, ObservedFailed)
			return err
		}
		return s.updateLifecycle(srcSB, DesiredRunning, ObservedRunning)
	case ObservedStopped:
		// Ensure stopped; may already be stopped after clone.
		st, err := s.Provider.Status(ctx, name)
		if err != nil {
			return err
		}
		obs := ObservedFromProvider(st.Status, st.Exists)
		if obs.IsRunning() {
			if err := s.Provider.Stop(ctx, name); err != nil {
				return err
			}
		}
		desired := DesiredStopped
		if srcSB.DesiredState == string(DesiredRunning) && !stoppedForClone {
			// Preserve prior desired when we didn't change it; typically stopped source had desired stopped.
			desired = DesiredStopped
		}
		return s.updateLifecycle(srcSB, desired, ObservedStopped)
	default:
		return nil
	}
}
