package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/ansible"
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "up <name>",
		Short: "Create sandbox VM and provision toolchains",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			if err := installGalaxyCollections(cmd.Context(), cmd, app); err != nil {
				if store, openErr := state.Open(app.Config.State.Path); openErr == nil {
					recordEvent(store, name, "dependencies", "failed", err.Error())
					_ = store.Close()
				}
				return fmt.Errorf("dependencies phase: %w", err)
			}
			if store, openErr := state.Open(app.Config.State.Path); openErr == nil {
				recordEvent(store, name, "dependencies", "ok", "Ansible Galaxy collections installed")
				_ = store.Close()
			}
			if err := runCreate(cmd.Context(), cmd, name, createOpts{}); err != nil {
				return err
			}
			if err := runProvision(cmd, name, false); err != nil {
				return err
			}
			// `up` is the complete first-boot workflow: provision the base
			// system, install agents, then deploy the configured credentials.
			if err := newAgentsCmd().RunE(cmd, []string{name}); err != nil {
				return fmt.Errorf("install coding agents: %w", err)
			}
			if app.Config.Egress.Mode == "" || app.Config.Egress.Mode == "open" {
				if err := newSecretsDeployCmd().RunE(cmd, []string{name}); err != nil {
					return fmt.Errorf("deploy secrets: %w", err)
				}
			} else {
				// Delegated mode must never route resolved values through Ansible or guest files.
				if err := configureEgress(cmd, name); err != nil {
					return fmt.Errorf("configure delegated egress: %w", err)
				}
			}
			if err := runWorkspaceSetup(cmd, name); err != nil {
				return fmt.Errorf("project setup: %w", err)
			}
			store, err := state.Open(app.Config.State.Path)
			if err != nil {
				return err
			}
			defer store.Close()
			if _, err := syncSandboxServices(cmd, store, app, name); err != nil {
				return fmt.Errorf("sync services: %w", err)
			}
			return printPublishedURLs(cmd, store, app, name)
		},
	}
}

func newSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use: "setup [name]", Short: "Run the repository's standard scripts/kaas-setup contract", Args: cobra.MaximumNArgs(1),
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
			if c, adopted, err := ensureContract(cmd.Context(), app, name, contract.NewStoreRepository(store)); err != nil {
				return err
			} else if adopted {
				printContractAdoption(cmd, name, c)
			}
			return runWorkspaceSetup(cmd, name)
		},
	}
}

func runWorkspaceSetup(cmd *cobra.Command, name string) error {
	app := AppFrom(cmd)
	if app == nil || app.Config == nil {
		return fmt.Errorf("app not initialized")
	}
	store, err := state.Open(app.Config.State.Path)
	if err != nil {
		return err
	}
	defer store.Close()
	sb, err := loadSandbox(store, name)
	if err != nil {
		return err
	}
	sources := append([]config.WorkspaceSource(nil), app.Config.Workspace.Sources...)
	// Prefer persisted contract sources when present (resume authority).
	if row, getErr := contract.NewStoreRepository(store).Get(cmd.Context(), name); getErr == nil {
		row.Normalize()
		if len(row.Workspace.Sources) > 0 {
			sources = make([]config.WorkspaceSource, 0, len(row.Workspace.Sources))
			for _, src := range row.Workspace.Sources {
				sources = append(sources, config.WorkspaceSource{
					Repository: src.Repository,
					Branch:     src.Branch,
					Directory:  src.Directory,
				})
			}
		}
	}
	if len(sources) == 0 {
		recordEvent(store, name, "checkout", "skipped", "no workspace configured")
		recordEvent(store, name, "environment", "skipped", "no workspace configured")
		recordEvent(store, name, "setup", "skipped", "no workspace configured")
		printStyled(cmd, styleInfo, "No workspace.sources configured; skipping checkout and setup on %s\n", name)
		return nil
	}
	primary := sources[0]
	if sb.RepoPath != "" {
		app.Config.RepoRoot = sb.RepoPath
	}
	// Keep managed workspace identity with the sandbox so list/inspect remain
	// meaningful outside the original project directory (primary source only).
	sb.ConfigSource = app.Config.ConfigPath
	sb.WorkspaceRepository = primary.Repository
	sb.WorkspaceBranch = primary.Branch
	sb.WorkspaceDirectory = primary.Directory
	if err := store.UpdateSandbox(*sb); err != nil {
		return err
	}
	if err := ansible.Preflight(false); err != nil {
		return err
	}
	runner := newRunner(app)
	runner.ForwardAgent = app.Config.Workspace.GitAuth == "agent"
	app.Log.Info("running workspace setup", "name", name, "sources", len(sources), "script", "scripts/kaas-setup", "git_auth", app.Config.Workspace.GitAuth)

	checkedOut := 0
	for i, src := range sources {
		if strings.TrimSpace(src.Repository) == "" {
			recordEvent(store, name, "checkout", "skipped", fmt.Sprintf("sources[%d]: no repository", i))
			continue
		}
		if err := checkoutWorkspaceSource(cmd, runner, *sb, src); err != nil {
			sb.SetupStatus = "failed"
			_ = store.UpdateSandbox(*sb)
			recordEvent(store, name, "checkout", "failed", fmt.Sprintf("%s: %v", src.Directory, err))
			recordEvent(store, name, "setup", "failed", "checkout failed")
			return fmt.Errorf("checkout phase (%s): %w", src.Directory, err)
		}
		checkedOut++
		recordEvent(store, name, "checkout", "ok", fmt.Sprintf("%s cloned or updated", src.Directory))
	}
	if checkedOut == 0 {
		recordEvent(store, name, "checkout", "skipped", "no workspace.sources repository configured")
	}

	if schema := app.Config.Workspace.Varlock.Schema; schema != "" {
		validate := app.Config.Workspace.Varlock.ValidateCommand
		if validate == "" {
			validate = "varlock load --agent --summary-stderr"
		}
		command := "test -f " + remotePath(schema) + " && " + validate
		if err := runner.RunShell(cmd.Context(), *sb, "cd "+remotePath(primary.Directory)+" && "+command); err != nil {
			sb.SetupStatus = "failed"
			_ = store.UpdateSandbox(*sb)
			recordEvent(store, name, "environment", "failed", err.Error())
			recordEvent(store, name, "setup", "failed", "Varlock environment validation failed")
			return fmt.Errorf("environment validation phase: %w", err)
		}
		recordEvent(store, name, "environment", "ok", "Varlock schema validated")
	} else {
		recordEvent(store, name, "environment", "skipped", "no workspace.varlock.schema configured")
	}
	setupPath := primary.Directory + "/scripts/kaas-setup"
	if exists, err := workspaceFileExists(cmd, *sb, setupPath); err != nil {
		recordEvent(store, name, "setup", "failed", err.Error())
		return fmt.Errorf("locate standard setup script: %w", err)
	} else if !exists {
		recordEvent(store, name, "setup", "skipped", "scripts/kaas-setup not found")
		return nil
	}
	if err := runner.RunShell(cmd.Context(), *sb, "cd "+remotePath(primary.Directory)+" && ./scripts/kaas-setup"); err != nil {
		sb.SetupStatus = "failed"
		_ = store.UpdateSandbox(*sb)
		recordEvent(store, name, "setup", "failed", err.Error())
		return fmt.Errorf("setup phase: %w", err)
	}

	if sb.WorkspaceDirectory != "" {
		if commit, err := workspaceCommit(cmd, *sb); err == nil {
			sb.WorkspaceCommit = commit
		}
	}
	now := time.Now().UTC()
	sb.SetupStatus, sb.SetupCompletedAt = "ok", &now
	if err := store.UpdateSandbox(*sb); err != nil {
		return err
	}
	recordEvent(store, name, "setup", "ok", "")
	printStyled(cmd, styleSuccess, "Completed workspace setup on %s\n", name)
	return nil
}

func checkoutWorkspaceSource(cmd *cobra.Command, runner *ansible.Runner, sb state.Sandbox, src config.WorkspaceSource) error {
	repository := strings.TrimSpace(src.Repository)
	directory := strings.TrimSpace(src.Directory)
	branch := strings.TrimSpace(src.Branch)
	if repository == "" || directory == "" {
		return fmt.Errorf("repository and directory are required")
	}
	checkout := "if [ -d " + remotePath(directory+"/.git") + " ]; then git -C " + remotePath(directory) + " fetch origin"
	if branch != "" {
		checkout += " " + shellQuote(branch) + " && git -C " + remotePath(directory) + " checkout " + shellQuote(branch) + " && git -C " + remotePath(directory) + " pull --ff-only origin " + shellQuote(branch)
	}
	checkout += "; else mkdir -p " + remotePath(filepath.Dir(directory)) + " && git clone"
	if branch != "" {
		checkout += " --branch " + shellQuote(branch)
	}
	checkout += " " + shellQuote(repository) + " " + remotePath(directory) + "; fi"
	return runner.RunShell(cmd.Context(), sb, checkout)
}

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'" }

// remotePath preserves the conventional ~/ prefix for expansion in the guest.
func remotePath(value string) string {
	if value == "~" {
		return "$HOME"
	}
	if strings.HasPrefix(value, "~/") {
		return "$HOME/" + shellQuote(value[2:])
	}
	return shellQuote(value)
}

func workspaceFileExists(cmd *cobra.Command, sb state.Sandbox, path string) (bool, error) {
	user, host, key := sandboxSSH(sb)
	sshCmd := exec.Command("ssh", "-i", key, "-p", fmt.Sprint(sb.SSHPort), "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new", fmt.Sprintf("%s@%s", user, host), "test -f "+remotePath(path))
	err := sshCmd.Run()
	if err == nil {
		return true, nil
	}
	if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

func workspaceCommit(cmd *cobra.Command, sb state.Sandbox) (string, error) {
	user, host, key := sandboxSSH(sb)
	command := "cd " + remotePath(sb.WorkspaceDirectory) + " && git rev-parse HEAD"
	sshCmd := exec.Command("ssh", "-i", key, "-p", fmt.Sprint(sb.SSHPort), "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new", fmt.Sprintf("%s@%s", user, host), command)
	output, err := sshCmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func newProvisionCmd() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:   "provision [name]",
		Short: "Install packages and language toolchains on a sandbox",
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
			return runProvision(cmd, name, check)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "dry-run (ansible --check)")
	return cmd
}

func runProvision(cmd *cobra.Command, name string, check bool) error {
	store, app, err := openStore(cmd)
	if err != nil {
		return err
	}
	defer store.Close()

	sb, err := loadSandbox(store, name)
	if err != nil {
		return err
	}
	if sb.RepoPath != "" {
		app.Config.RepoRoot = sb.RepoPath
	}

	app.Log.Info("provisioning sandbox", "name", name, "check", check)
	if err := ansible.Preflight(false); err != nil {
		return err
	}
	runner := newRunner(app)
	err = runner.RunPlaybook(cmd.Context(), ansible.PlaybookOpts{
		Playbook: app.Config.Ansible.Playbooks.Setup,
		Sandbox:  *sb,
		Check:    check,
	})
	if err != nil {
		recordEvent(store, name, "provision", "failed", err.Error())
		return err
	}
	now := time.Now().UTC()
	sb.LastProvisionedAt = &now
	sb.DesiredState = state.DesiredRunning
	sb.ObservedState = state.ObservedRunning
	_ = store.UpdateSandbox(*sb)
	recordEvent(store, name, "provision", "ok", "")
	printStyled(cmd, styleSuccess, "Provisioned sandbox %s\n", name)
	return nil
}
