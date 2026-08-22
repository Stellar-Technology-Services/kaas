package cli

import (
	"github.com/spf13/cobra"
	"path/filepath"
)

func newPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping [name]",
		Short: "Confirm SSH/Ansible connectivity to a sandbox",
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
			if sb.RepoPath != "" {
				app.Config.RepoRoot = sb.RepoPath
				app.Config.Paths.AnsibleDir = filepath.Join(sb.RepoPath, "ansible")
			}

			runner := newRunner(app)
			if err := runner.Ping(cmd.Context(), *sb); err != nil {
				recordEvent(store, name, "ping", "failed", err.Error())
				return err
			}
			recordEvent(store, name, "ping", "ok", "")
			return nil
		},
	}
}
