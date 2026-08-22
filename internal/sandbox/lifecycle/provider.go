package lifecycle

import (
	"context"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
)

// Profile is a provider-side sandbox profile summary.
type Profile struct {
	Name   string
	Status string
	Arch   string
	CPUs   int
	Memory int
	Disk   int
}

// ProviderStatus is the provider-reported status for one profile.
type ProviderStatus struct {
	Name   string
	Status string
	Exists bool
}

// CreateSpec configures provider profile creation.
type CreateSpec struct {
	Name    string
	CPUs    int
	Memory  int // GiB
	Disk    int // GiB
	Arch    string
	SSHPort int
}

// StartSpec configures starting an existing provider profile.
type StartSpec struct {
	Name    string
	CPUs    int
	Memory  int
	Disk    int
	Arch    string
	SSHPort int
}

// ProviderResult is returned after a successful provider create.
type ProviderResult struct {
	Profile string
	Status  ProviderStatus
}

// Provider abstracts Colima/Lima (or a fake) behind lifecycle use cases.
type Provider interface {
	Available(ctx context.Context) error
	List(ctx context.Context) ([]Profile, error)
	Status(ctx context.Context, profile string) (ProviderStatus, error)
	Create(ctx context.Context, spec CreateSpec) (ProviderResult, error)
	Start(ctx context.Context, spec StartSpec) error
	Stop(ctx context.Context, profile string) error
	Delete(ctx context.Context, profile string, force bool) error
	Clone(ctx context.Context, source, target string, sshPort int) error
	SSHConfig(ctx context.Context, profile string) (identity.Endpoint, error)
	UsedPorts(ctx context.Context) ([]int, error)
}
