package lifecycle

import (
	"context"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// EventRecorder persists ordered lifecycle audit events.
type EventRecorder interface {
	Record(ctx context.Context, sandbox, action, status, detail string) error
}

// EgressCoordinator stops/restores/removes host-owned egress runtime for a sandbox.
type EgressCoordinator interface {
	Stop(ctx context.Context, name string) error
	Restore(ctx context.Context, name string) error
	RemoveRuntime(ctx context.Context, name string) error
}

// PortAllocator allocates and reserves SSH ports.
type PortAllocator interface {
	Allocate(ctx context.Context, base int, used []int) (int, error)
	Reserve(ctx context.Context, port int, sandbox, opID string, expires time.Time) error
	Release(ctx context.Context, port int) error
}

// StoreEvents adapts state.Store to EventRecorder.
type StoreEvents struct {
	Store *state.Store
}

func (e StoreEvents) Record(_ context.Context, sandboxName, action, status, detail string) error {
	if e.Store == nil {
		return nil
	}
	return e.Store.AddEvent(sandboxName, action, status, detail)
}

// StorePortAllocator implements PortAllocator using SQLite reservations.
type StorePortAllocator struct {
	Store *state.Store
}

// NewStorePortAllocator returns a PortAllocator backed by state.Store.
func NewStorePortAllocator(store *state.Store) *StorePortAllocator {
	return &StorePortAllocator{Store: store}
}

func (a *StorePortAllocator) Allocate(_ context.Context, base int, used []int) (int, error) {
	return sandbox.NextPort(base, used)
}

func (a *StorePortAllocator) Reserve(_ context.Context, port int, sandboxName, opID string, expires time.Time) error {
	if a == nil || a.Store == nil {
		return nil
	}
	return a.Store.ReservePort(port, sandboxName, opID, expires)
}

func (a *StorePortAllocator) Release(_ context.Context, port int) error {
	if a == nil || a.Store == nil {
		return nil
	}
	return a.Store.ReleasePort(port)
}

// NoopEgress is an EgressCoordinator that ignores all operations.
type NoopEgress struct{}

func (NoopEgress) Stop(context.Context, string) error          { return nil }
func (NoopEgress) Restore(context.Context, string) error       { return nil }
func (NoopEgress) RemoveRuntime(context.Context, string) error { return nil }
