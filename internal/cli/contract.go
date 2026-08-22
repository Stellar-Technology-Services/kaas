package cli

import (
	"context"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/spf13/cobra"
)

// resolveContractStack builds Resolver input from the effective app config.
// Persisted authority and first-time adoption are applied inside Resolve.
func resolveContractStack(cfg *config.Config, persisted *contract.Contract, flags contract.Layer, adopt bool) (contract.Resolved, error) {
	if cfg == nil {
		return contract.Resolved{}, fmt.Errorf("config is required")
	}
	in := contract.Input{
		BuiltIn:   contract.LayerFromConfig(config.BuiltIn(), "builtin"),
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
	in.Project = contract.LayerFromConfig(*cfg, source)
	return contract.Resolver{}.Resolve(context.Background(), in)
}

// ensureContract loads the persisted contract or adopts one when none exists.
func ensureContract(ctx context.Context, app *App, name string, repo *contract.StoreRepository) (contract.Contract, bool, error) {
	if app == nil || app.Config == nil {
		return contract.Contract{}, false, fmt.Errorf("app not initialized")
	}
	persisted, err := contract.GetOrNil(ctx, repo, name)
	if err != nil {
		return contract.Contract{}, false, err
	}
	resolved, err := resolveContractStack(app.Config, persisted, contract.Layer{}, false)
	if err != nil {
		return contract.Contract{}, false, err
	}
	if resolved.Adopted || persisted == nil {
		if err := repo.Put(ctx, name, resolved.Contract); err != nil {
			return contract.Contract{}, false, fmt.Errorf("persist adopted contract: %w", err)
		}
		return resolved.Contract, true, nil
	}
	return resolved.Contract, false, nil
}

func printContractAdoption(cmd *cobra.Command, name string, c contract.Contract) {
	printStyled(cmd, styleSuccess, "Adopted contract for %s (schema %d hash %s)\n", name, c.SchemaVersion, c.ContentHash)
	printMessage(cmd, "  resources: %d CPU / %d GiB RAM / %d GiB disk (%s)\n", c.Resources.CPUs, c.Resources.MemoryGiB, c.Resources.DiskGiB, c.Resources.Arch)
	if primary, ok := c.Workspace.Primary(); ok && primary.Repository != "" {
		printMessage(cmd, "  workspace: %s\n", primary.Repository)
		if n := len(c.Workspace.Sources); n > 1 {
			printMessage(cmd, "  workspace sources: %d\n", n)
		}
	}
	if c.Egress.Mode != "" {
		printMessage(cmd, "  egress: %s\n", c.Egress.Mode)
	}
}
