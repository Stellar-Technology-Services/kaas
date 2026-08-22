package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
)

// Start starts a stopped sandbox, reconciling SSH and restoring egress best-effort.
// Already-running is idempotent success.
func (s *Service) Start(ctx context.Context, name string) error {
	if s.Provider == nil || s.Store == nil {
		return fmt.Errorf("lifecycle service is not configured")
	}
	if err := s.Provider.Available(ctx); err != nil {
		return err
	}
	sb, err := s.loadSandbox(name)
	if err != nil {
		return err
	}
	observed, st, err := s.providerObserved(ctx, name)
	if err != nil {
		return err
	}
	if !st.Exists {
		return fmt.Errorf("provider profile %q not found; use create to recreate", name)
	}
	if observed.IsRunning() {
		ident, _ := identity.FromState(*sb)
		if reconciled, sshErr := s.reconcileSSH(ctx, ident, name); sshErr == nil {
			identity.ApplyToState(sb, reconciled)
		}
		if err := s.updateLifecycle(sb, DesiredRunning, ObservedRunning); err != nil {
			return err
		}
		if err := s.record(ctx, name, "start", "ok", "already running"); err != nil {
			return err
		}
		return nil
	}

	if err := s.updateLifecycle(sb, DesiredRunning, ObservedStarting); err != nil {
		return err
	}
	if err := s.Provider.Start(ctx, startSpecFromSandbox(sb)); err != nil {
		_ = s.updateLifecycle(sb, DesiredRunning, ObservedFailed)
		if recErr := s.record(ctx, name, "start", "failed", err.Error()); recErr != nil {
			return errors.Join(err, recErr)
		}
		return fmt.Errorf("start sandbox %s: %w", name, err)
	}
	ident, _ := identity.FromState(*sb)
	if reconciled, sshErr := s.reconcileSSH(ctx, ident, name); sshErr == nil {
		identity.ApplyToState(sb, reconciled)
	}
	if err := s.updateLifecycle(sb, DesiredRunning, ObservedRunning); err != nil {
		return err
	}
	if err := s.record(ctx, name, "start", "ok", ""); err != nil {
		return err
	}
	if err := s.Egress.Restore(ctx, name); err != nil {
		return &EgressError{Op: "start", Sandbox: name, Err: err}
	}
	return nil
}
