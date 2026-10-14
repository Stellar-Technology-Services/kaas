package cli

import (
	"fmt"
	"path"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

// Trust presets collapse the two kaas credential models into one switch, the
// way boat.dev's "safe for third parties" toggle does.
const (
	TrustTrusted   = "trusted"   // deploy SecretSpec values into the guest (current default)
	TrustUntrusted = "untrusted" // proxy_only delegated egress; the guest never holds a value
)

// upOpts are the ergonomics flags shared by `kaas up` (and the UI create form).
type upOpts struct {
	createOpts
	Size     string
	Repo     string
	Branch   string
	Trust    string
	From     string // template (or sandbox) whose disk is cloned instead of provisioning
	AutoStop string // TTL spec; "" means defaults.auto_stop
}

// applySize fills unset CPU/memory/disk from a named preset. Explicit
// --cpus/--memory/--disk still win over the preset.
func applySize(cfg *config.Config, size string, opts *createOpts) error {
	if strings.TrimSpace(size) == "" {
		return nil
	}
	s, err := cfg.Defaults.ResolveSize(size)
	if err != nil {
		return err
	}
	if opts.CPUs == 0 {
		opts.CPUs = s.CPUs
	}
	if opts.Memory == 0 {
		opts.Memory = s.MemoryGiB
	}
	if opts.Disk == 0 {
		opts.Disk = s.DiskGiB
	}
	return nil
}

// applyRepo makes a one-off repository the primary workspace source so the
// persisted contract records it without a project kaas.yaml.
func applyRepo(cfg *config.Config, repo, branch string) error {
	repo = strings.TrimSpace(repo)
	if repo == "" {
		if strings.TrimSpace(branch) != "" {
			return fmt.Errorf("--branch requires --repo")
		}
		return nil
	}
	dir := "~/workspace/" + repoBaseName(repo)
	cfg.Workspace.Sources = []config.WorkspaceSource{{Repository: repo, Branch: strings.TrimSpace(branch), Directory: dir}}
	return nil
}

// repoBaseName derives a directory name from a git URL or path.
func repoBaseName(repo string) string {
	name := strings.TrimSuffix(strings.TrimRight(repo, "/"), ".git")
	if i := strings.LastIndexAny(name, "/:"); i >= 0 {
		name = name[i+1:]
	}
	name = path.Base(name)
	if name == "" || name == "." || name == "/" {
		return "project"
	}
	return name
}

// applyTrust switches the credential model. "untrusted" forces proxy_only
// delegated egress with a default-deny policy; when the project declares no
// policy, GitHub hosts are allowed (without credentials) so checkout works.
func applyTrust(cfg *config.Config, trust string) error {
	switch strings.ToLower(strings.TrimSpace(trust)) {
	case "", TrustTrusted:
		return nil
	case TrustUntrusted:
		if cfg.Egress.Mode == "" || cfg.Egress.Mode == "open" {
			if len(cfg.Egress.Rules) == 0 {
				cfg.Egress.Rules = []config.EgressRule{
					{Host: "github.com"},
					{Host: "api.github.com"},
					{Host: "objects.githubusercontent.com"},
					{Host: "codeload.github.com"},
				}
			}
			cfg.Egress.SchemaVersion = 1
		}
		cfg.Egress.Mode = "proxy_only"
		cfg.Egress.Default = "deny"
		return cfg.Egress.Validate()
	default:
		return fmt.Errorf("--trust must be %s or %s", TrustTrusted, TrustUntrusted)
	}
}
