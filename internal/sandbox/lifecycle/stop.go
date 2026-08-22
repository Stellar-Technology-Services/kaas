package lifecycle

import (
	"context"
	"errors"
	"fmt"
)

// Stop stops a running sandbox while preserving registry and disk.
// Already-stopped is idempotent. Egress stop failures are surfaced without
// undoing a successful VM stop.
func (s *Service) Stop(ctx context.Context, name string) error {
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
		return fmt.Errorf("provider profile %q not found; use destroy to remove its stale registry entry", name)
	}
	if observed.IsStopped() {
		if err := s.updateLifecycle(sb, DesiredStopped, ObservedStopped); err != nil {
			return err
		}
		if err := s.record(ctx, name, "stop", "ok", "already stopped"); err != nil {
			return err
		}
		return nil
	}

	if err := s.updateLifecycle(sb, DesiredStopped, ObservedStopping); err != nil {
		return err
	}
	if err := s.Provider.Stop(ctx, name); err != nil {
		_ = s.updateLifecycle(sb, DesiredStopped, ObservedFailed)
		if recErr := s.record(ctx, name, "stop", "failed", err.Error()); recErr != nil {
			return errors.Join(err, recErr)
		}
		return fmt.Errorf("stop sandbox %s: %w", name, err)
	}
	if err := s.updateLifecycle(sb, DesiredStopped, ObservedStopped); err != nil {
		return err
	}
	if err := s.record(ctx, name, "stop", "ok", ""); err != nil {
		return err
	}
	if err := s.Egress.Stop(ctx, name); err != nil {
		return &EgressError{Op: "stop", Sandbox: name, Err: err}
	}
	return nil
}
