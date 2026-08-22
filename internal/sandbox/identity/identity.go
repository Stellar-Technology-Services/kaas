package identity

import (
	"fmt"
	"strings"
)

// DefaultSSHHost is the centralized default for sandbox SSH endpoints.
const DefaultSSHHost = "127.0.0.1"

// ProviderIdentity is the provider-side profile identity for a sandbox.
type ProviderIdentity struct {
	ProfileName string
}

// ControlRepository is host-side control/repo provenance for the sandbox.
// It is distinct from the guest workspace identity.
type ControlRepository struct {
	Path string
	Name string
}

// WorkspaceIdentity is the guest workspace the sandbox is intended to serve.
type WorkspaceIdentity struct {
	Repository string
	Branch     string
	Directory  string
	Commit     string
}

// Endpoint is the SSH reachability identity for a sandbox.
type Endpoint struct {
	Host         string
	Port         int
	User         string
	IdentityFile string
}

// Resources are desired VM resource requirements.
type Resources struct {
	CPUs      int
	MemoryGiB int
	DiskGiB   int
	Arch      string
}

// SSHConfig is provider-reported SSH configuration used to reconcile an endpoint.
type SSHConfig struct {
	Host         string
	Port         int
	User         string
	IdentityFile string
}

// Identity is the secret-free stable identity of a sandbox.
// It must not contain credential values or private egress runtime state.
type Identity struct {
	ID           SandboxID
	Provider     ProviderIdentity
	ControlRepo  ControlRepository
	Workspace    WorkspaceIdentity
	Endpoint     Endpoint
	Resources    Resources
	ConfigSource string
}

// NewInput carries fields required to construct a new Identity.
type NewInput struct {
	ID           SandboxID
	ControlRepo  ControlRepository
	Workspace    WorkspaceIdentity
	Endpoint     Endpoint
	Resources    Resources
	ConfigSource string
}

// New constructs and validates a new sandbox identity.
// Provider profile identity is derived from the sandbox ID.
func New(in NewInput) (Identity, error) {
	id := Identity{
		ID:           in.ID,
		Provider:     ProviderIdentity{ProfileName: in.ID.Name},
		ControlRepo:  in.ControlRepo,
		Workspace:    in.Workspace,
		Endpoint:     in.Endpoint,
		Resources:    in.Resources,
		ConfigSource: in.ConfigSource,
	}
	if id.Endpoint.Host == "" {
		id.Endpoint.Host = DefaultSSHHost
	}
	if err := Validate(id); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// FromFork builds a target identity from a source.
//
// Copied: control repository provenance, workspace intent, resources, config source.
// Regenerated: sandbox ID, provider profile, SSH endpoint.
//
// Host-owned egress runtime and credentials are not part of Identity and must
// not be copied by callers when persisting the fork.
func FromFork(source Identity, target SandboxID, endpoint Endpoint) Identity {
	if endpoint.Host == "" {
		endpoint.Host = DefaultSSHHost
	}
	return Identity{
		ID:           target,
		Provider:     ProviderIdentity{ProfileName: target.Name},
		ControlRepo:  source.ControlRepo,
		Workspace:    source.Workspace,
		Endpoint:     endpoint,
		Resources:    source.Resources,
		ConfigSource: source.ConfigSource,
	}
}

// WithSSHConfig returns a copy of identity with endpoint fields reconciled from
// provider SSH configuration. Empty cfg fields leave the existing values in place
// except Host/Port, which must be valid when provided.
func WithSSHConfig(id Identity, cfg SSHConfig) (Identity, error) {
	out := id
	if cfg.Host != "" {
		out.Endpoint.Host = cfg.Host
	}
	if cfg.Port != 0 {
		out.Endpoint.Port = cfg.Port
	}
	if cfg.User != "" {
		out.Endpoint.User = cfg.User
	}
	if cfg.IdentityFile != "" {
		out.Endpoint.IdentityFile = cfg.IdentityFile
	}
	if out.Endpoint.Host == "" {
		out.Endpoint.Host = DefaultSSHHost
	}
	if err := validateEndpoint(out.Endpoint); err != nil {
		return Identity{}, err
	}
	return out, nil
}

// Validate enforces identity invariants for a fully constructed identity.
// Sandbox names must satisfy the strict grammar (D1).
func Validate(id Identity) error {
	if err := ValidateName(id.ID.Name); err != nil {
		return err
	}
	if strings.TrimSpace(id.Provider.ProfileName) == "" {
		return fmt.Errorf("provider profile name is required")
	}
	if id.Provider.ProfileName != id.ID.Name {
		return fmt.Errorf("provider profile %q must match sandbox name %q", id.Provider.ProfileName, id.ID.Name)
	}
	if strings.TrimSpace(id.ControlRepo.Path) == "" || strings.TrimSpace(id.ControlRepo.Name) == "" {
		return fmt.Errorf("control repository path and name are required")
	}
	if err := validateEndpoint(id.Endpoint); err != nil {
		return err
	}
	if err := validateResources(id.Resources); err != nil {
		return err
	}
	return nil
}

func validateEndpoint(ep Endpoint) error {
	host := ep.Host
	if host == "" {
		host = DefaultSSHHost
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("ssh host is required")
	}
	if ep.Port < 1 || ep.Port > 65535 {
		return fmt.Errorf("invalid ssh port: %d", ep.Port)
	}
	return nil
}

func validateResources(r Resources) error {
	if r.CPUs < 1 {
		return fmt.Errorf("cpus must be >= 1")
	}
	if r.MemoryGiB < 1 {
		return fmt.Errorf("memory_gib must be >= 1")
	}
	if r.DiskGiB < 1 {
		return fmt.Errorf("disk_gib must be >= 1")
	}
	if strings.TrimSpace(r.Arch) == "" {
		return fmt.Errorf("arch is required")
	}
	return nil
}
