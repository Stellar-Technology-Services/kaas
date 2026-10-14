package cli

import (
	"encoding/json"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/spf13/cobra"
)

// `kaas contract` exposes the persisted, redacted contract and the explicit
// re-adopt path (boat: "running sandboxes stay on their version until you
// upgrade them"). It never prints secret values — the type has none.
func newContractCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "contract",
		Short: "Show the persisted sandbox contract or re-adopt it from the project kaas.yaml",
	}
	cmd.AddCommand(newContractShowCmd(), newContractAdoptCmd())
	return cmd
}

func newContractShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show [name]",
		Short: "Print the persisted contract and any drift from the project config",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			name, err := resolveSandboxName(cmd, store, args)
			if err != nil {
				return err
			}
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			persisted, err := loadPersistedContract(store, name)
			if err != nil {
				return err
			}
			changes, driftErr := ContractDrift(app.Config.State.Path, sb.RepoPath, persisted)
			if jsonlEnabled(cmd) {
				emitJSONL("contract", map[string]any{"sandbox": name, "contract": persisted, "changes": changes, "drift_error": errString(driftErr)})
				return nil
			}
			body, _ := json.MarshalIndent(persisted, "", "  ")
			fmt.Fprintln(commandOutput(cmd), string(body))
			switch {
			case driftErr != nil:
				printStyled(cmd, styleWarn, "drift check unavailable: %v\n", driftErr)
			case len(changes) == 0:
				printStyled(cmd, styleSuccess, "No drift: project config matches the persisted contract\n")
			default:
				printStyled(cmd, styleWarn, "%d change(s) between project config and persisted contract (run kaas contract adopt %s to accept):\n", len(changes), name)
				for _, c := range changes {
					printMessage(cmd, "  %s: %s → %s\n", c.Path, c.From, c.To)
				}
			}
			return nil
		},
	}
}

func newContractAdoptCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "adopt [name]",
		Short: "Replace the persisted contract with the current project config stack (explicit reconcile)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			name, err := resolveSandboxName(cmd, store, args)
			if err != nil {
				return err
			}
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			cfg := app.Config
			if sb.RepoPath != "" {
				// Resolve from the sandbox's own project, not wherever the operator
				// happens to be standing.
				if loaded, lerr := config.Load(config.LoadOptions{CWD: sb.RepoPath, StatePath: app.Config.State.Path}); lerr == nil {
					cfg = loaded
				}
			}
			repo := contract.NewStoreRepository(store)
			persisted, err := contract.GetOrNil(cmd.Context(), repo, name)
			if err != nil {
				return err
			}
			resolved, err := resolveContractStack(cfg, persisted, contract.Layer{}, true)
			if err != nil {
				return err
			}
			resolved.Contract.Resources.SSHPort = sb.SSHPort
			if hashed, hashErr := resolved.Contract.WithHash(); hashErr == nil {
				resolved.Contract = hashed
			}
			if err := repo.Put(cmd.Context(), name, resolved.Contract); err != nil {
				return err
			}
			recordEvent(store, name, "contract", "ok", fmt.Sprintf("adopted hash %s (%d changes)", short(resolved.Contract.ContentHash), len(resolved.Changes)))
			printContractAdoption(cmd, name, resolved.Contract)
			for _, c := range resolved.Changes {
				printMessage(cmd, "  %s: %s → %s\n", c.Path, c.From, c.To)
			}
			return nil
		},
	}
}

// ContractDrift resolves the contract the sandbox's project would produce
// today and diffs it against the persisted one.
func ContractDrift(statePath, repoPath string, persisted contract.Contract) ([]contract.ContractChange, error) {
	return contract.DriftFromProject(statePath, repoPath, persisted)
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func short(hash string) string {
	if len(hash) > 12 {
		return hash[:12]
	}
	return hash
}
