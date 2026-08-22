package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// StoreRepository adapts state.Store to the identity.Repository interface.
type StoreRepository struct {
	Store *state.Store
}

// NewStoreRepository returns a Repository backed by the SQLite state store.
func NewStoreRepository(store *state.Store) *StoreRepository {
	return &StoreRepository{Store: store}
}

func (r *StoreRepository) Get(_ context.Context, id SandboxID) (Identity, error) {
	if r == nil || r.Store == nil {
		return Identity{}, fmt.Errorf("identity repository is not configured")
	}
	sb, err := r.Store.GetSandbox(id.Name)
	if err != nil {
		return Identity{}, err
	}
	return FromState(*sb)
}

func (r *StoreRepository) Create(_ context.Context, ident Identity) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("identity repository is not configured")
	}
	if err := Validate(ident); err != nil {
		return err
	}
	var sb state.Sandbox
	ApplyToState(&sb, ident)
	sb.DesiredState = state.DesiredRunning
	sb.ObservedState = state.ObservedCreating
	return r.Store.CreateSandbox(sb)
}

func (r *StoreRepository) Update(_ context.Context, ident Identity) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("identity repository is not configured")
	}
	existing, err := r.Store.GetSandbox(ident.ID.Name)
	if err != nil {
		return err
	}
	if err := ValidateName(ident.ID.Name); err != nil {
		return err
	}
	if err := validateEndpoint(ident.Endpoint); err != nil {
		return err
	}
	if err := validateResources(ident.Resources); err != nil {
		return err
	}
	ApplyToState(existing, ident)
	return r.Store.UpdateSandbox(*existing)
}

func (r *StoreRepository) Delete(_ context.Context, id SandboxID) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("identity repository is not configured")
	}
	return r.Store.DeleteSandbox(id.Name)
}

func (r *StoreRepository) List(_ context.Context, filter Filter) ([]Identity, error) {
	if r == nil || r.Store == nil {
		return nil, fmt.Errorf("identity repository is not configured")
	}
	rows, err := r.Store.ListSandboxes(filter.WorkspaceRepository)
	if err != nil {
		return nil, err
	}
	out := make([]Identity, 0, len(rows))
	for _, sb := range rows {
		id, err := FromState(sb)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// IsNotFound reports whether err is a missing-sandbox error.
func IsNotFound(err error) bool {
	return errors.Is(err, state.ErrNotFound)
}
