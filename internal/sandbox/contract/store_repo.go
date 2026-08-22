package contract

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// StoreRepository adapts state.Store to Repository.
type StoreRepository struct {
	Store *state.Store
}

// NewStoreRepository returns a Repository backed by the SQLite state store.
func NewStoreRepository(store *state.Store) *StoreRepository {
	return &StoreRepository{Store: store}
}

func (r *StoreRepository) Get(_ context.Context, sandbox string) (Contract, error) {
	if r == nil || r.Store == nil {
		return Contract{}, fmt.Errorf("contract repository is not configured")
	}
	snap, err := r.Store.GetContract(sandbox)
	if err != nil {
		return Contract{}, err
	}
	c, err := ParseJSON([]byte(snap.ContractJSON))
	if err != nil {
		return Contract{}, err
	}
	if c.ContentHash == "" {
		c.ContentHash = snap.ContractHash
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = ContractVersion(snap.SchemaVersion)
	}
	return c, nil
}

func (r *StoreRepository) Put(_ context.Context, sandbox string, c Contract) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("contract repository is not configured")
	}
	c.Normalize()
	hashed, err := c.WithHash()
	if err != nil {
		return err
	}
	raw, err := hashed.RedactedJSON()
	if err != nil {
		return err
	}
	return r.Store.PutContract(state.ContractSnapshot{
		Sandbox:       sandbox,
		SchemaVersion: int(hashed.SchemaVersion),
		ContractJSON:  string(raw),
		ContractHash:  hashed.ContentHash,
		SourceConfig:  hashed.Provenance.ConfigSource,
		ResolvedAt:    time.Now().UTC(),
	})
}

func (r *StoreRepository) Delete(_ context.Context, sandbox string) error {
	if r == nil || r.Store == nil {
		return fmt.Errorf("contract repository is not configured")
	}
	return r.Store.DeleteContract(sandbox)
}

// IsNotFound reports whether err is a missing-contract error.
func IsNotFound(err error) bool {
	return errors.Is(err, state.ErrNotFound)
}

// GetOrNil loads a contract or returns nil when no contract row exists yet.
func GetOrNil(ctx context.Context, repo Repository, sandbox string) (*Contract, error) {
	c, err := repo.Get(ctx, sandbox)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}
