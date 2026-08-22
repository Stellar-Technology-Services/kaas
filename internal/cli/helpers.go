package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Stellar-Technology-Services/kaas/internal/ansible"
	"github.com/Stellar-Technology-Services/kaas/internal/colima"
	"github.com/Stellar-Technology-Services/kaas/internal/secretspec"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

func openStore(cmd *cobra.Command) (*state.Store, *App, error) {
	app := AppFrom(cmd)
	if app == nil || app.Config == nil {
		return nil, nil, fmt.Errorf("app not initialized")
	}
	store, err := state.Open(app.Config.State.Path)
	if err != nil {
		return nil, nil, err
	}
	return store, app, nil
}

func requireRepoRoot(app *App) (string, error) {
	if app == nil || app.Config == nil {
		return "", fmt.Errorf("app not initialized")
	}
	root := app.Config.GlobalRoot
	if root == "" {
		return "", fmt.Errorf("KaaS global workspace not found; run kaas init")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filepath.Join(abs, "ansible", "ansible.cfg")); err != nil {
		return "", fmt.Errorf("KaaS global workspace is incomplete (no ansible/ansible.cfg under %s); rerun kaas init", abs)
	}
	return abs, nil
}

func resolveSandboxName(cmd *cobra.Command, store *state.Store, args []string) (string, error) {
	if len(args) >= 1 && args[0] != "" {
		return args[0], nil
	}
	list, err := store.ListSandboxes("")
	if err != nil {
		return "", err
	}
	if len(list) == 1 {
		return list[0].Name, nil
	}
	if len(list) == 0 {
		return "", fmt.Errorf("no sandboxes registered; pass a name or run: kaas create <name>")
	}
	return "", fmt.Errorf("multiple sandboxes exist; specify a name (try: kaas list)")
}

func loadSandbox(store *state.Store, name string) (*state.Sandbox, error) {
	sb, err := store.GetSandbox(name)
	if err != nil {
		return nil, err
	}
	return sb, nil
}

func newRunner(app *App) *ansible.Runner {
	return &ansible.Runner{AnsibleDir: app.Config.AnsibleDir(), Stdout: commandOutputFromApp(app), Stderr: os.Stderr}
}

func recordEvent(store *state.Store, name, action, status, detail string) {
	_ = store.AddEvent(name, action, status, detail)
}

func commandOutputFromApp(app *App) *os.File {
	if app != nil && app.Flags.JSONL {
		return os.Stderr
	}
	return os.Stdout
}

func newColimaClient(app *App) *colima.Client {
	client := colima.New()
	if app != nil && app.Flags.JSONL {
		client.Stdout = os.Stderr
		client.Stderr = os.Stderr
	}
	return client
}

func newSecretSpecClient(app *App, repoRoot string) *secretspec.Client {
	client := secretspec.New(repoRoot)
	if app != nil && app.Flags.JSONL {
		client.Stdout = os.Stderr
		client.Stderr = os.Stderr
	}
	return client
}
