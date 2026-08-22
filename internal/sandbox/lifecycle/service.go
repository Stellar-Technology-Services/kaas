package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

const defaultPortReservationTTL = 10 * time.Minute

// Service owns sandbox lifecycle use cases.
type Service struct {
	Provider Provider
	Store    *state.Store
	Events   EventRecorder
	Egress   EgressCoordinator
	Ports    PortAllocator
	Clock    func() time.Time
	PortBase int
}

// Deps configures a lifecycle Service.
type Deps struct {
	Provider Provider
	Store    *state.Store
	Events   EventRecorder
	Egress   EgressCoordinator
	Ports    PortAllocator
	Clock    func() time.Time
	PortBase int
}

// NewService constructs a lifecycle Service from dependencies.
func NewService(d Deps) *Service {
	svc := &Service{
		Provider: d.Provider,
		Store:    d.Store,
		Events:   d.Events,
		Egress:   d.Egress,
		Ports:    d.Ports,
		Clock:    d.Clock,
		PortBase: d.PortBase,
	}
	if svc.Events == nil && d.Store != nil {
		svc.Events = StoreEvents{Store: d.Store}
	}
	if svc.Egress == nil {
		svc.Egress = NoopEgress{}
	}
	if svc.Ports == nil && d.Store != nil {
		svc.Ports = NewStorePortAllocator(d.Store)
	}
	if svc.Clock == nil {
		svc.Clock = func() time.Time { return time.Now().UTC() }
	}
	if svc.PortBase <= 0 {
		svc.PortBase = 6022
	}
	return svc
}

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) record(ctx context.Context, name, action, status, detail string) error {
	if s.Events == nil {
		return nil
	}
	return s.Events.Record(ctx, name, action, status, detail)
}

func (s *Service) setLifecycle(sb *state.Sandbox, desired DesiredState, observed ObservedState) {
	sb.DesiredState = string(desired)
	sb.ObservedState = string(observed)
}

func (s *Service) updateLifecycle(sb *state.Sandbox, desired DesiredState, observed ObservedState) error {
	s.setLifecycle(sb, desired, observed)
	return s.Store.UpdateSandbox(*sb)
}

func (s *Service) providerObserved(ctx context.Context, name string) (ObservedState, ProviderStatus, error) {
	st, err := s.Provider.Status(ctx, name)
	if err != nil {
		return ObservedUnknown, ProviderStatus{}, err
	}
	return ObservedFromProvider(st.Status, st.Exists), st, nil
}

func (s *Service) usedPorts(ctx context.Context) ([]int, error) {
	dbPorts, err := s.Store.UsedPorts()
	if err != nil {
		return nil, err
	}
	providerPorts, err := s.Provider.UsedPorts(ctx)
	if err != nil {
		// Best-effort merge; allocation still consults registry/reservations.
		return dbPorts, nil
	}
	return sandbox.MergeUsed(dbPorts, providerPorts), nil
}

func (s *Service) claim(ctx context.Context, name, action string) (*state.Operation, error) {
	_ = ctx
	return s.Store.ClaimOperation(name, action)
}

func (s *Service) finishOp(op *state.Operation, status, detail string) error {
	if op == nil {
		return nil
	}
	return s.Store.UpdateOperation(op.ID, status, detail)
}

func (s *Service) releaseOpPort(ctx context.Context, port int, op *state.Operation) error {
	if port > 0 && s.Ports != nil {
		if err := s.Ports.Release(ctx, port); err != nil {
			return err
		}
	}
	if op != nil {
		return s.Store.ReleasePortsForOperation(op.ID)
	}
	return nil
}

func (s *Service) loadSandbox(name string) (*state.Sandbox, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("lifecycle store is not configured")
	}
	return s.Store.GetSandbox(name)
}

func (s *Service) reconcileSSH(ctx context.Context, ident identity.Identity, name string) (identity.Identity, error) {
	ep, err := s.Provider.SSHConfig(ctx, name)
	if err != nil {
		return ident, err
	}
	return identity.WithSSHConfig(ident, identity.SSHConfig{
		Host:         ep.Host,
		Port:         ep.Port,
		User:         ep.User,
		IdentityFile: ep.IdentityFile,
	})
}

func startSpecFromSandbox(sb *state.Sandbox) StartSpec {
	return StartSpec{
		Name:    sb.Name,
		CPUs:    sb.CPUs,
		Memory:  sb.MemoryGiB,
		Disk:    sb.DiskGiB,
		Arch:    sb.Arch,
		SSHPort: sb.SSHPort,
	}
}
