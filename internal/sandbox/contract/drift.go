package contract

import (
	"context"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

// ResolveFromConfig builds the authority stack from an effective, already
// merged config and resolves it against the persisted contract. The CLI and
// the local UI share it so both compute the same contract and the same drift.
func ResolveFromConfig(cfg *config.Config, persisted *Contract, flags Layer, adopt bool) (Resolved, error) {
	if cfg == nil {
		return Resolved{}, fmt.Errorf("config is required")
	}
	in := Input{
		BuiltIn:   LayerFromConfig(config.BuiltIn(), "builtin"),
		Persisted: persisted,
		Adopt:     adopt,
		Reconcile: adopt,
		Flags:     flags,
	}
	// Effective cfg is already merged. Expose it as the project layer so
	// resolution sees workspace/egress/resources from the active files.
	source := cfg.ConfigPath
	if source == "" {
		source = cfg.UserConfig
	}
	if source == "" {
		source = "effective"
	}
	in.Project = LayerFromConfig(*cfg, source)
	return Resolver{}.Resolve(context.Background(), in)
}

// DriftFromProject loads the project config found at repoPath and reports the
// field-level changes the stack would make to the persisted contract. SSH port
// fields are bound at create time and are excluded. An empty repoPath or an
// unreadable project is an error, not drift.
func DriftFromProject(statePath, repoPath string, persisted Contract) ([]ContractChange, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("sandbox has no recorded project directory")
	}
	cfg, err := config.Load(config.LoadOptions{CWD: repoPath, StatePath: statePath})
	if err != nil {
		return nil, err
	}
	resolved, err := ResolveFromConfig(cfg, &persisted, Layer{}, false)
	if err != nil {
		return nil, err
	}
	var out []ContractChange
	for _, c := range resolved.Changes {
		if c.Path == "resources.ssh_port" || c.Path == "resources.ssh_port_policy" {
			continue
		}
		out = append(out, c)
	}
	return out, nil
}
