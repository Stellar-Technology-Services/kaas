package identity

import (
	"fmt"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// FromState maps a registry row into an Identity. It is a permissive mapper and
// does not enforce the strict name grammar; call Validate for new-identity
// invariants.
func FromState(sb state.Sandbox) (Identity, error) {
	if strings.TrimSpace(sb.Name) == "" {
		return Identity{}, fmt.Errorf("sandbox name is required")
	}
	host := sb.SSHHost
	if host == "" {
		host = DefaultSSHHost
	}
	return Identity{
		ID:       SandboxID{Name: sb.Name},
		Provider: ProviderIdentity{ProfileName: sb.Name},
		ControlRepo: ControlRepository{
			Path: sb.RepoPath,
			Name: sb.RepoName,
		},
		Workspace: WorkspaceIdentity{
			Repository: sb.WorkspaceRepository,
			Branch:     sb.WorkspaceBranch,
			Directory:  sb.WorkspaceDirectory,
			Commit:     sb.WorkspaceCommit,
		},
		Endpoint: Endpoint{
			Host:         host,
			Port:         sb.SSHPort,
			User:         sb.SSHUser,
			IdentityFile: sb.SSHKey,
		},
		Resources: Resources{
			CPUs:      sb.CPUs,
			MemoryGiB: sb.MemoryGiB,
			DiskGiB:   sb.DiskGiB,
			Arch:      sb.Arch,
		},
		ConfigSource: sb.ConfigSource,
	}, nil
}

// ApplyToState writes identity fields onto a registry row without touching
// lifecycle status, setup execution state, or egress runtime metadata.
func ApplyToState(sb *state.Sandbox, id Identity) {
	if sb == nil {
		return
	}
	sb.Name = id.ID.Name
	sb.RepoPath = id.ControlRepo.Path
	sb.RepoName = id.ControlRepo.Name
	sb.WorkspaceRepository = id.Workspace.Repository
	sb.WorkspaceBranch = id.Workspace.Branch
	sb.WorkspaceDirectory = id.Workspace.Directory
	sb.WorkspaceCommit = id.Workspace.Commit
	sb.SSHHost = id.Endpoint.Host
	if sb.SSHHost == "" {
		sb.SSHHost = DefaultSSHHost
	}
	sb.SSHPort = id.Endpoint.Port
	sb.SSHUser = id.Endpoint.User
	sb.SSHKey = id.Endpoint.IdentityFile
	sb.CPUs = id.Resources.CPUs
	sb.MemoryGiB = id.Resources.MemoryGiB
	sb.DiskGiB = id.Resources.DiskGiB
	sb.Arch = id.Resources.Arch
	sb.ConfigSource = id.ConfigSource
}
