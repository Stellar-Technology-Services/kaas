package lifecycle

import (
	"context"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// DiscrepancyKind classifies a registry/provider mismatch.
type DiscrepancyKind string

const (
	DiscrepancyRegistryWithoutProvider DiscrepancyKind = "registry_without_provider"
	DiscrepancyProviderWithoutRegistry DiscrepancyKind = "provider_without_registry"
	DiscrepancyStatusMismatch          DiscrepancyKind = "status_mismatch"
	DiscrepancyPortMismatch            DiscrepancyKind = "port_mismatch"
)

// Discrepancy is a typed reconciliation finding. Reconcile never silently rewrites state.
type Discrepancy struct {
	Kind             DiscrepancyKind
	Sandbox          string
	Detail           string
	RegistryObserved ObservedState
	RegistryDesired  DesiredState
	ProviderStatus   string
	RegistryPort     int
	ProviderPort     int
}

// Reconcile compares registry and provider state. name empty means all sandboxes
// plus orphan provider profiles. Findings are returned; state is not rewritten.
func (s *Service) Reconcile(ctx context.Context, name string) ([]Discrepancy, error) {
	if s.Provider == nil || s.Store == nil {
		return nil, fmt.Errorf("lifecycle service is not configured")
	}
	if err := s.Provider.Available(ctx); err != nil {
		return nil, err
	}

	profiles, err := s.Provider.List(ctx)
	if err != nil {
		return nil, err
	}
	providerByName := make(map[string]Profile, len(profiles))
	for _, p := range profiles {
		providerByName[p.Name] = p
	}

	var discrepancies []Discrepancy
	seen := map[string]struct{}{}

	if name != "" {
		sb, err := s.loadSandbox(name)
		if err != nil {
			if p, ok := providerByName[name]; ok {
				discrepancies = append(discrepancies, Discrepancy{
					Kind:           DiscrepancyProviderWithoutRegistry,
					Sandbox:        name,
					Detail:         "provider profile exists without registry row",
					ProviderStatus: p.Status,
				})
			}
			return discrepancies, err
		}
		return s.reconcileOne(ctx, sb, providerByName)
	}

	list, err := s.Store.ListSandboxes("")
	if err != nil {
		return nil, err
	}
	for i := range list {
		sb := &list[i]
		seen[sb.Name] = struct{}{}
		d, err := s.reconcileOne(ctx, sb, providerByName)
		if err != nil {
			return nil, err
		}
		discrepancies = append(discrepancies, d...)
	}
	for _, p := range profiles {
		if _, ok := seen[p.Name]; ok {
			continue
		}
		discrepancies = append(discrepancies, Discrepancy{
			Kind:           DiscrepancyProviderWithoutRegistry,
			Sandbox:        p.Name,
			Detail:         "provider profile exists without registry row",
			ProviderStatus: p.Status,
		})
	}
	return discrepancies, nil
}

func (s *Service) reconcileOne(ctx context.Context, sb *state.Sandbox, providerByName map[string]Profile) ([]Discrepancy, error) {
	var out []Discrepancy
	regObs := ObservedState(sb.ObservedState)
	regDesired := DesiredState(sb.DesiredState)

	p, ok := providerByName[sb.Name]
	if !ok {
		out = append(out, Discrepancy{
			Kind:             DiscrepancyRegistryWithoutProvider,
			Sandbox:          sb.Name,
			Detail:           "registry row exists but provider profile is missing",
			RegistryObserved: regObs,
			RegistryDesired:  regDesired,
			RegistryPort:     sb.SSHPort,
		})
		return out, nil
	}

	provObs := ObservedFromProvider(p.Status, true)
	if regObs != "" && regObs != ObservedUnknown && !compatibleObserved(regObs, provObs) {
		out = append(out, Discrepancy{
			Kind:             DiscrepancyStatusMismatch,
			Sandbox:          sb.Name,
			Detail:           fmt.Sprintf("registry observed %q but provider reports %q", regObs, p.Status),
			RegistryObserved: regObs,
			RegistryDesired:  regDesired,
			ProviderStatus:   p.Status,
			RegistryPort:     sb.SSHPort,
		})
	}

	ep, err := s.Provider.SSHConfig(ctx, sb.Name)
	if err == nil && ep.Port > 0 && sb.SSHPort > 0 && ep.Port != sb.SSHPort {
		out = append(out, Discrepancy{
			Kind:             DiscrepancyPortMismatch,
			Sandbox:          sb.Name,
			Detail:           fmt.Sprintf("registry ssh_port %d differs from provider port %d", sb.SSHPort, ep.Port),
			RegistryObserved: regObs,
			RegistryDesired:  regDesired,
			ProviderStatus:   p.Status,
			RegistryPort:     sb.SSHPort,
			ProviderPort:     ep.Port,
		})
	}
	return out, nil
}

func compatibleObserved(registry, provider ObservedState) bool {
	return registry == provider
}
