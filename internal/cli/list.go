package cli

import (
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var (
		refresh    bool
		repoFilter string
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List all sandboxes in the user-global registry",
		Long: `Print every sandbox registered in the kaas state database.

Columns: NAME, PORT, DESIRED, OBSERVED, REPO, REPO PATH.
Use --refresh to reconcile registry rows with the provider and report
discrepancies without silently rewriting state.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}

			store, err := state.Open(app.Config.State.Path)
			if err != nil {
				return err
			}
			defer store.Close()

			sandboxes, err := store.ListSandboxes(repoFilter)
			if err != nil {
				return err
			}

			if refresh {
				svc := newLifecycleService(store, app)
				discrepancies, err := svc.Reconcile(cmd.Context(), "")
				if err != nil {
					return fmt.Errorf("reconcile: %w", err)
				}
				if jsonlEnabled(cmd) {
					for _, d := range discrepancies {
						emitJSONL("discrepancy", map[string]any{"command": cmd.CommandPath(), "discrepancy": d})
					}
				} else if len(discrepancies) > 0 {
					printStyled(cmd, styleWarn, "Reconciliation found %d discrepancy(ies):\n", len(discrepancies))
					for _, d := range discrepancies {
						printMessage(cmd, "  - %s: %s (%s)\n", d.Sandbox, d.Detail, d.Kind)
					}
				} else {
					printStyled(cmd, styleSuccess, "Reconciliation: no discrepancies\n")
				}
				// Re-read for display; Reconcile does not rewrite rows.
				sandboxes, err = store.ListSandboxes(repoFilter)
				if err != nil {
					return err
				}
			}

			if jsonlEnabled(cmd) {
				for _, sandbox := range sandboxes {
					emitJSONL("sandbox", map[string]any{"command": cmd.CommandPath(), "sandbox": sandbox})
				}
				return nil
			}

			rows := make([][]string, 0, len(sandboxes))
			if len(sandboxes) == 0 {
				rows = append(rows, []string{"-", "-", "-", "-", "-", "(no sandboxes registered)"})
			}
			for _, sb := range sandboxes {
				desired := sb.DesiredState
				if desired == "" {
					desired = "-"
				}
				observed := sb.ObservedState
				if observed == "" {
					observed = "-"
				}
				rows = append(rows, []string{
					sb.Name, fmt.Sprintf("%d", sb.SSHPort), desired, observed, sb.RepoName, sb.RepoPath,
				})
			}
			writeTable(commandOutput(cmd), []string{"NAME", "PORT", "DESIRED", "OBSERVED", "REPO", "REPO PATH"}, rows, tableOpts{
				StatusCols: []int{2, 3},
			})
			return nil
		},
	}

	cmd.Flags().BoolVar(&refresh, "refresh", false, "reconcile registry with provider; report discrepancies")
	cmd.Flags().StringVar(&repoFilter, "repo", "", "filter by repo path or repo name")

	return cmd
}
