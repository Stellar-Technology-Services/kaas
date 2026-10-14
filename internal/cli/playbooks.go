package cli

import (
	"io"
	"os"
	"path/filepath"

	"github.com/Stellar-Technology-Services/kaas/internal/ansible"
	"github.com/spf13/cobra"
)

func newAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents [name]",
		Short: "Install coding agents (pi, omp, prime-agent)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNamedPlaybook(cmd, args, "agents", func(app *App) string {
				return app.Config.Ansible.Playbooks.Agents
			})
		},
	}
}

func newPubkeyCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pubkey [name]",
		Short: "Generate sandbox SSH key if missing and print the public key",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runNamedPlaybook(cmd, args, "pubkey", func(app *App) string {
				return app.Config.Ansible.Playbooks.Pubkey
			})
		},
	}
}

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check [name]",
		Short: "Dry-run provision (ansible --check)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			name, err := resolveSandboxName(cmd, store, args)
			if err != nil {
				return err
			}
			return runProvision(cmd, name, true)
		},
	}
}

func runNamedPlaybook(cmd *cobra.Command, args []string, action string, playbookFn func(*App) string, extra ...ansible.PlaybookOpts) error {
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
	if sb.RepoPath != "" {
		app.Config.RepoRoot = sb.RepoPath
	}

	if err := ansible.Preflight(false); err != nil {
		return err
	}

	opts := ansible.PlaybookOpts{
		Playbook: playbookFn(app),
		Sandbox:  *sb,
	}
	if len(extra) > 0 {
		opts.Check = extra[0].Check
		opts.ExtraVars = extra[0].ExtraVars
		opts.ExtraArgs = extra[0].ExtraArgs
		opts.Limit = extra[0].Limit
	}

	runner := newRunner(app)
	log := attachPhaseLog(app, runner, name, action)
	defer log.Close()
	app.Log.Info("running playbook", "action", action, "playbook", opts.Playbook, "name", name)
	if err := runner.RunPlaybook(cmd.Context(), opts); err != nil {
		recordEvent(store, name, action, "failed", err.Error())
		return err
	}
	recordEvent(store, name, action, "ok", "")
	printStyled(cmd, styleSuccess, "Completed %s on %s\n", action, name)
	return nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
