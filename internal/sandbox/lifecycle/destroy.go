package lifecycle

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// DestroyInput configures Destroy (D5).
type DestroyInput struct {
	Name  string
	Force bool
}

// Destroy deletes the provider profile then the registry row.
// On provider failure the registry is retained unless Force is set (D5).
func (s *Service) Destroy(ctx context.Context, in DestroyInput) error {
	if s.Provider == nil || s.Store == nil {
		return fmt.Errorf("lifecycle service is not configured")
	}
	if in.Name == "" {
		return fmt.Errorf("sandbox name is required")
	}
	if err := s.Provider.Available(ctx); err != nil {
		return err
	}
	name := in.Name

	sb, err := s.Store.GetSandbox(name)
	registryMissing := errors.Is(err, state.ErrNotFound)
	if err != nil && !registryMissing {
		return err
	}

	observed, st, err := s.providerObserved(ctx, name)
	if err != nil {
		return err
	}

	// Idempotent: provider absent and no registry row.
	if !st.Exists && registryMissing {
		_ = s.Egress.RemoveRuntime(ctx, name)
		if err := s.record(ctx, name, "destroy", "ok", "already absent"); err != nil {
			return err
		}
		return nil
	}

	if sb != nil {
		if err := s.updateLifecycle(sb, DesiredDestroyed, ObservedDestroying); err != nil {
			return err
		}
	}

	// Best-effort egress stop before provider delete.
	_ = s.Egress.Stop(ctx, name)

	providerOK := true
	if st.Exists {
		if err := s.Provider.Delete(ctx, name, true); err != nil {
			providerOK = false
			// Confirm absence — delete may have partially succeeded.
			after, _, statusErr := s.providerObserved(ctx, name)
			if statusErr == nil && after.IsAbsent() {
				providerOK = true
			} else if !in.Force {
				if sb != nil {
					failObs := ObservedFailed
					if statusErr != nil {
						failObs = ObservedUnknown
					}
					_ = s.updateLifecycle(sb, DesiredDestroyed, failObs)
				}
				detail := err.Error()
				if recErr := s.record(ctx, name, "destroy", "failed", detail); recErr != nil {
					return errors.Join(err, recErr)
				}
				return fmt.Errorf("provider delete failed for %q; registry retained (retry destroy or pass --force): %w", name, err)
			}
		} else {
			after, _, statusErr := s.providerObserved(ctx, name)
			if statusErr != nil || !after.IsAbsent() {
				providerOK = false
				if !in.Force {
					if sb != nil {
						_ = s.updateLifecycle(sb, DesiredDestroyed, ObservedUnknown)
					}
					msg := "provider profile still present after delete"
					if statusErr != nil {
						msg = statusErr.Error()
					}
					if recErr := s.record(ctx, name, "destroy", "failed", msg); recErr != nil {
						return errors.Join(fmt.Errorf("%s", msg), recErr)
					}
					return fmt.Errorf("provider profile %q still present after delete; registry retained (retry or --force)", name)
				}
			}
		}
	}

	if err := s.Egress.RemoveRuntime(ctx, name); err != nil && !in.Force {
		// Registry removal still proceeds when provider is confirmed gone;
		// surface egress cleanup failure.
		if !registryMissing {
			if delErr := s.Store.DeleteSandbox(name); delErr != nil {
				return errors.Join(err, delErr)
			}
		}
		if recErr := s.record(ctx, name, "destroy", "failed", err.Error()); recErr != nil {
			return errors.Join(err, recErr)
		}
		return &EgressError{Op: "destroy", Sandbox: name, Err: err}
	}
	_ = s.Egress.RemoveRuntime(ctx, name)

	if !registryMissing {
		if err := s.Store.DeleteSandbox(name); err != nil {
			return err
		}
	}
	detail := ""
	if !providerOK && in.Force {
		detail = "forced registry removal; provider profile may still exist"
	}
	if err := s.record(ctx, name, "destroy", "ok", detail); err != nil {
		return err
	}
	_ = observed
	return nil
}
