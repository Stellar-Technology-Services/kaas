package identity

import "context"

// Filter selects identities when listing.
type Filter struct {
	// WorkspaceRepository matches stored guest workspace repository when set.
	WorkspaceRepository string
}

// Repository is the persistence seam for sandbox identity.
// The SQLite adapter is provided by a later slice; this interface is the domain boundary.
type Repository interface {
	Get(ctx context.Context, id SandboxID) (Identity, error)
	Create(ctx context.Context, identity Identity) error
	Update(ctx context.Context, identity Identity) error
	Delete(ctx context.Context, id SandboxID) error
	List(ctx context.Context, filter Filter) ([]Identity, error)
}
