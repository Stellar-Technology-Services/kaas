package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/lifecycle"
	"github.com/spf13/cobra"
)

func newStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop [name]",
		Short: "Stop a sandbox VM while preserving its disk and registry entry",
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
			// Optional repository pre-stop hook (scripts/kaas-prestop): flush app
			// state before the VM goes away. Never blocks the stop.
			runPreStopHook(cmd, store, *sb)

			svc := newLifecycleService(store, app)
			app.Log.Info("stopping sandbox", "name", name)
			if err := svc.Stop(cmd.Context(), name); err != nil {
				var ee *lifecycle.EgressError
				if errors.As(err, &ee) {
					reloadIngressBestEffort(store, app)
					clearAutoStopDeadline(store, name)
					printStyled(cmd, styleSuccess, "Stopped sandbox %s; its disk and registry entry are preserved\n", name)
					return err
				}
				return err
			}
			reloadIngressBestEffort(store, app)
			clearAutoStopDeadline(store, name)
			// Detached guest commands do not survive a VM stop (it is a reboot).
			_ = store.MarkCommandsLost(name)
			printStyled(cmd, styleSuccess, "Stopped sandbox %s; its disk and registry entry are preserved\n", name)
			return nil
		},
	}
}

func newDestroyCmd() *cobra.Command {
	var yes bool
	var force bool

	cmd := &cobra.Command{
		Use:   "destroy <name>",
		Short: "Delete a Colima sandbox VM and remove it from the registry",
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

			if !yes {
				if !confirmPrompt(cmd, fmt.Sprintf("Destroy sandbox %q and permanently delete its VM and data? [y/N] ", name)) {
					printStyled(cmd, styleWarn, "Destroy cancelled\n")
					return nil
				}
			}

			if sb, err := loadSandbox(store, name); err == nil && sb.RepoPath != "" {
				app.Config.RepoRoot = sb.RepoPath
				app.Config.Paths.AnsibleDir = filepath.Join(sb.RepoPath, "ansible")
			}

			svc := newLifecycleService(store, app)
			app.Log.Info("destroying sandbox", "name", name, "force", force)
			if err := svc.Destroy(cmd.Context(), lifecycle.DestroyInput{Name: name, Force: force}); err != nil {
				return err
			}
			reloadIngressBestEffort(store, app)
			stopIngressIfIdle(store, app)
			printStyled(cmd, styleSuccess, "Destroyed sandbox %s\n", name)
			return nil
		},
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip confirmation")
	cmd.Flags().BoolVar(&force, "force", false, "remove registry even if provider delete fails")
	return cmd
}
