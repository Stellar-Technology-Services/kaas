package contract

import "context"

// Repository is the persistence seam for redacted sandbox contracts.
type Repository interface {
	// Get returns the persisted contract. ErrNotFound means no row exists yet.
	Get(ctx context.Context, sandbox string) (Contract, error)
	// Put stores a redacted contract snapshot for sandbox.
	Put(ctx context.Context, sandbox string, c Contract) error
	// Delete removes a contract row. Sandbox deletion also cascades via FK.
	Delete(ctx context.Context, sandbox string) error
}
