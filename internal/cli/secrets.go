package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/ansible"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/secretspec"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage API keys via SecretSpec (profile = sandbox name)",
		Long: `Declare secrets in secretspec.toml; store values in a provider (keyring).

Profile convention:
  default     — shared keys for every sandbox
  <sandbox>   — per-instance overrides (same name as kaas create <name>)

Examples:
  kaas secrets init
  kaas secrets set ANTHROPIC_API_KEY
  kaas secrets set GITHUB_TOKEN --sandbox work
  kaas secrets deploy work`,
	}

	cmd.AddCommand(newSecretsInitCmd())
	cmd.AddCommand(newSecretsSetCmd())
	cmd.AddCommand(newSecretsDeployCmd())
	cmd.AddCommand(newSecretsCheckCmd())
	return cmd
}

func newSecretsInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Verify secretspec.toml and print setup hints",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			root, err := requireRepoRoot(app)
			if err != nil {
				return err
			}
			client := newSecretSpecClient(app, root)
			if err := client.Available(); err != nil {
				return err
			}
			if err := client.EnsureManifest(); err != nil {
				return err
			}
			printMessage(cmd, "SecretSpec contract: %s\n", client.File)
			printMessage(cmd, "Next steps:\n")
			printMessage(cmd, "  1. secretspec config global init   # pick keyring (recommended)\n")
			printMessage(cmd, "  2. kaas secrets set ANTHROPIC_API_KEY\n")
			printMessage(cmd, "  3. kaas secrets set GITHUB_TOKEN --sandbox <name>   # optional per-sandbox\n")
			printMessage(cmd, "  4. kaas secrets deploy <name>\n")
			printMessage(cmd, "\n")
			return nil
		},
	}
}

func newSecretsSetCmd() *cobra.Command {
	var sandbox string
	cmd := &cobra.Command{
		Use:   "set <KEY>",
		Short: "Store a secret value (interactive) for default or a sandbox profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			root, err := requireRepoRoot(app)
			if err != nil {
				return err
			}
			client := newSecretSpecClient(app, root)
			if err := client.Available(); err != nil {
				return err
			}
			if err := client.EnsureManifest(); err != nil {
				return err
			}
			profile := "default"
			if sandbox != "" {
				profile = sandbox
			}
			key := strings.ToUpper(args[0])
			printMessage(cmd, "Setting %s for profile %q (SecretSpec)\n", key, profile)
			return client.SetPrompt(key, profile)
		},
	}
	cmd.Flags().StringVar(&sandbox, "sandbox", "", "sandbox name (= SecretSpec profile); default profile if omitted")
	return cmd
}

func newSecretsDeployCmd() *cobra.Command {
	var scope string
	cmd := &cobra.Command{
		Use:   "deploy [name]",
		Short: "Resolve SecretSpec (default ∪ sandbox profile) and deploy onto the VM",
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
			if c, adopted, err := ensureContract(cmd.Context(), app, name, contract.NewStoreRepository(store)); err != nil {
				return err
			} else if adopted {
				printContractAdoption(cmd, name, c)
			}
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			// SecretSpec lives in the user-global workspace (~/.kaas), not the
			// control repo that registered the sandbox (sb.RepoPath).
			root, err := requireRepoRoot(app)
			if err != nil {
				return err
			}
			if sb.RepoPath != "" {
				app.Config.RepoRoot = sb.RepoPath
			}

			client := newSecretSpecClient(app, root)
			if err := client.Available(); err != nil {
				return err
			}
			if err := client.EnsureManifest(); err != nil {
				return err
			}

			resolved, err := client.ResolveSandbox(name, scope)
			if err != nil {
				return err
			}
			if len(resolved) == 0 {
				// SecretSpec declarations may all be optional. An empty resolution is
				// a valid first-run state, not a failed sandbox lifecycle phase.
				recordEvent(store, name, "secrets", "skipped", "no secrets resolved")
				printStyled(cmd, styleWarn, "No secrets resolved for %s; skipping deployment\n", name)
				return nil
			}

			ansibleVars := secretspec.ToAnsibleVars(resolved)
			tmp, err := writeTempSecretsYAML(ansibleVars)
			if err != nil {
				return err
			}
			defer os.Remove(tmp)

			app.Log.Info("deploying secrets", "sandbox", name, "count", len(ansibleVars), "scope", scope)
			err = runNamedPlaybook(cmd, []string{name}, "secrets", func(app *App) string {
				return app.Config.Ansible.Playbooks.Secrets
			}, ansible.PlaybookOpts{
				ExtraArgs: []string{"-e", "@" + tmp},
			})
			if err != nil {
				return err
			}
			printStyled(cmd, styleSuccess, "Deployed %d secret(s) to %s via SecretSpec\n", len(ansibleVars), name)
			return nil
		},
	}
	cmd.Flags().StringVar(&scope, "scope", "", "SecretSpec scope (e.g. agents, github)")
	return cmd
}

func newSecretsCheckCmd() *cobra.Command {
	var sandbox string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run secretspec check for default or a sandbox profile",
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			root, err := requireRepoRoot(app)
			if err != nil {
				return err
			}
			client := newSecretSpecClient(app, root)
			if err := client.Available(); err != nil {
				return err
			}
			profile := "default"
			if sandbox != "" {
				profile = sandbox
			}
			return client.Check(profile)
		},
	}
	cmd.Flags().StringVar(&sandbox, "sandbox", "", "sandbox profile to check (default: default)")
	return cmd
}

func writeTempSecretsYAML(vars map[string]string) (string, error) {
	f, err := os.CreateTemp("", "kaas-secrets-*.yml")
	if err != nil {
		return "", err
	}
	path := f.Name()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(vars); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	_ = enc.Close()
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	_ = os.Chmod(path, 0o600)
	return path, nil
}
