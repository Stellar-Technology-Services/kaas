package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/Stellar-Technology-Services/kaas/internal/ansible"
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/egress"
	eruntime "github.com/Stellar-Technology-Services/kaas/internal/egress/runtime"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func newEgressCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "egress", Short: "Manage delegated-credential HTTP(S) egress"}
	cmd.AddCommand(newEgressPlanCmd(), newEgressServeCmd(), newEgressConfigureCmd(), newEgressReloadCmd(), newEgressStatusCmd(), newEgressVerifyCmd())
	return cmd
}
func newEgressPlanCmd() *cobra.Command {
	return &cobra.Command{Use: "plan", Short: "Print the effective redacted egress policy", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		app := AppFrom(cmd)
		if app == nil || app.Config == nil {
			return fmt.Errorf("app not initialized")
		}
		plan := egress.Plan(app.Config.Egress)
		if plan.SchemaVersion == 0 {
			return fmt.Errorf("no egress policy configured")
		}
		if jsonlEnabled(cmd) {
			emitJSONL("egress_plan", map[string]any{"command": cmd.CommandPath(), "plan": plan})
			return nil
		}
		out := commandOutput(cmd)
		writeKV(out, []kvPair{
			{Key: "MODE", Value: plan.Mode},
			{Key: "DEFAULT", Value: plan.Default},
		})
		rows := make([][]string, 0, len(plan.Rules))
		for _, r := range plan.Rules {
			rows = append(rows, []string{r.Host, r.Credential, r.Injection, r.Header})
		}
		writeTable(out, []string{"HOST", "CREDENTIAL", "INJECTION", "HEADER"}, rows, tableOpts{})
		return nil
	}}
}
func newEgressServeCmd() *cobra.Command {
	var root string
	c := &cobra.Command{Use: "serve <name>", Short: "Run the host-owned egress proxy (normally lifecycle-managed)", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		return eruntime.Serve(ctx, root, args[0], os.Stdin)
	}}
	c.Flags().StringVar(&root, "runtime-root", "", "host runtime root")
	return c
}
func referencedCredentials(app *App, name string) (map[string]string, error) {
	keys := map[string]config.EgressCredential{}
	for _, r := range app.Config.Egress.Rules {
		if r.InjectSecret != nil {
			keys[r.InjectSecret.Key] = *r.InjectSecret
		}
	}
	if len(keys) == 0 {
		return map[string]string{}, nil
	}
	root := app.Config.GlobalRoot
	if root == "" {
		return nil, fmt.Errorf("KaaS global workspace not found; run kaas init")
	}
	client := newSecretSpecClient(app, root)
	if err := client.Available(); err != nil {
		return nil, err
	}
	all, err := client.ResolveSandbox(name, "")
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for key, credential := range keys {
		v := all[key]
		if v == "" {
			return nil, fmt.Errorf("delegated SecretSpec key %s is unresolved", key)
		}
		if credential.Type == "basic" && !strings.Contains(v, ":") {
			return nil, fmt.Errorf("delegated basic key %s must have username:password form", key)
		}
		out[key] = v
	}
	return out, nil
}
func ensureEgress(cmd *cobra.Command, sb *state.Sandbox, app *App) (eruntime.Status, error) {
	if app.Config.Egress.Mode == "open" || app.Config.Egress.Mode == "" {
		return eruntime.Status{}, nil
	}
	creds, e := referencedCredentials(app, sb.Name)
	if e != nil {
		return eruntime.Status{}, e
	}
	if st, statusErr := eruntime.Call(app.Config.EgressRuntime.Root, sb.Name, eruntime.Request{Action: "status"}); statusErr == nil && st.Running {
		// Reconfigure is also a rotation point; values remain on the private socket.
		return eruntime.Call(app.Config.EgressRuntime.Root, sb.Name, eruntime.Request{Action: "reload", Credentials: creds})
	}
	port := sb.EgressPort
	if port == 0 {
		port = app.Config.EgressRuntime.PortBase + (sb.SSHPort - app.Config.Defaults.SSHPortBase)
		if port < 1 || port > 65535 {
			return eruntime.Status{}, fmt.Errorf("allocated egress port out of range")
		}
	}
	exe, e := os.Executable()
	if e != nil {
		return eruntime.Status{}, e
	}
	args := []string{}
	if app.Config.ConfigPath != "" {
		args = append(args, "--config", app.Config.ConfigPath)
	}
	args = append(args, "--state", app.Config.State.Path, "egress", "serve", sb.Name, "--runtime-root", app.Config.EgressRuntime.Root)
	child := exec.Command(exe, args...)
	stdin, e := child.StdinPipe()
	if e != nil {
		return eruntime.Status{}, e
	}
	child.Stdout, child.Stderr = openDevNull(), openDevNull()
	if e = child.Start(); e != nil {
		return eruntime.Status{}, e
	}
	bootstrap := eruntime.Bootstrap{Policy: app.Config.Egress, Credentials: creds, Listen: app.Config.EgressRuntime.ListenHost + ":" + strconv.Itoa(port)}
	if e = json.NewEncoder(stdin).Encode(bootstrap); e != nil {
		return eruntime.Status{}, e
	}
	_ = stdin.Close()
	go child.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, e := eruntime.Call(app.Config.EgressRuntime.Root, sb.Name, eruntime.Request{Action: "status"}); e == nil {
			sb.EgressPort = port
			return st, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return eruntime.Status{}, fmt.Errorf("egress proxy failed to become ready")
}
func openDevNull() *os.File { f, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0); return f }
func egressPolicyHash(p any) string {
	b, _ := json.Marshal(p)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func configureEgress(cmd *cobra.Command, name string) error {
	store, app, e := openStore(cmd)
	if e != nil {
		return e
	}
	defer store.Close()
	if c, adopted, err := ensureContract(cmd.Context(), app, name, contract.NewStoreRepository(store)); err != nil {
		return err
	} else if adopted {
		printContractAdoption(cmd, name, c)
	}
	sb, e := loadSandbox(store, name)
	if e != nil {
		return e
	}
	if sb.RepoPath != "" {
		app.Config.RepoRoot = sb.RepoPath
	}
	if app.Config.Egress.Mode == "" || app.Config.Egress.Mode == "open" {
		return nil
	}
	st, e := ensureEgress(cmd, sb, app)
	if e != nil {
		recordEvent(store, name, "egress", "failed", e.Error())
		return e
	}
	host := app.Config.EgressRuntime.GuestHost
	proxyURL := "http://" + host + ":" + strconv.Itoa(sb.EgressPort)
	keys := []string{}
	for _, r := range app.Config.Egress.Rules {
		if r.InjectSecret != nil {
			keys = append(keys, r.InjectSecret.Key)
		}
	}
	vars := map[string]any{"egress_mode": app.Config.Egress.Mode, "egress_proxy_host": host, "egress_proxy_port": sb.EgressPort, "egress_proxy_url": proxyURL, "egress_ca_file": filepath.Join(eruntime.Dir(app.Config.EgressRuntime.Root, name), "ca.pem"), "egress_placeholder_keys": keys}
	tmp, e := writePublicYAML(vars)
	if e != nil {
		return e
	}
	defer os.Remove(tmp)
	runner := newRunner(app)
	if e = runner.RunPlaybook(cmd.Context(), ansible.PlaybookOpts{Playbook: app.Config.Ansible.Playbooks.Egress, Sandbox: *sb, ExtraArgs: []string{"-e", "@" + tmp}}); e != nil {
		recordEvent(store, name, "egress", "failed", e.Error())
		return e
	}
	sb.EgressMode = app.Config.Egress.Mode
	sb.EgressStatus = "running"
	sb.EgressCAFingerprint = st.CAFingerprint
	sb.EgressPolicyHash = egressPolicyHash(app.Config.Egress)
	if e = store.UpdateSandbox(*sb); e != nil {
		return e
	}
	recordEvent(store, name, "egress", "ok", fmt.Sprintf("mode=%s policy=%s", sb.EgressMode, sb.EgressPolicyHash))
	printStyled(cmd, styleSuccess, "Configured %s egress for %s (proxy %s)\n", sb.EgressMode, name, proxyURL)
	return nil
}
func writePublicYAML(v any) (string, error) {
	f, e := os.CreateTemp("", "kaas-egress-public-*.yml")
	if e != nil {
		return "", e
	}
	p := f.Name()
	_ = os.Chmod(p, 0600)
	e = yaml.NewEncoder(f).Encode(v)
	_ = f.Close()
	if e != nil {
		_ = os.Remove(p)
	}
	return p, e
}
func resolveName(cmd *cobra.Command, args []string) (string, error) {
	store, _, e := openStore(cmd)
	if e != nil {
		return "", e
	}
	defer store.Close()
	return resolveSandboxName(cmd, store, args)
}
func newEgressConfigureCmd() *cobra.Command {
	return &cobra.Command{Use: "configure [name]", Short: "Start/reload proxy and configure guest trust and topology", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		n, e := resolveName(cmd, args)
		if e != nil {
			return e
		}
		return configureEgress(cmd, n)
	}}
}
func newEgressReloadCmd() *cobra.Command {
	return &cobra.Command{Use: "reload [name]", Short: "Reload delegated values over the private control socket", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, app, e := openStore(cmd)
		if e != nil {
			return e
		}
		defer store.Close()
		n, e := resolveSandboxName(cmd, store, args)
		if e != nil {
			return e
		}
		sb, e := loadSandbox(store, n)
		if e != nil {
			return e
		}
		app.Config.RepoRoot = sb.RepoPath
		creds, e := referencedCredentials(app, n)
		if e != nil {
			return e
		}
		st, e := eruntime.Call(app.Config.EgressRuntime.Root, n, eruntime.Request{Action: "reload", Credentials: creds})
		if e != nil {
			return e
		}
		recordEvent(store, n, "egress_reload", "ok", "delegated credentials reloaded")
		if jsonlEnabled(cmd) {
			emitJSONL("egress_status", map[string]any{"status": st})
			return nil
		}
		printStyled(cmd, styleSuccess, "Reloaded delegated credentials for %s\n", n)
		return nil
	}}
}
func newEgressStatusCmd() *cobra.Command {
	return &cobra.Command{Use: "status [name]", Short: "Show redacted egress runtime status", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, app, e := openStore(cmd)
		if e != nil {
			return e
		}
		defer store.Close()
		n, e := resolveSandboxName(cmd, store, args)
		if e != nil {
			return e
		}
		sb, e := loadSandbox(store, n)
		if e != nil {
			return e
		}
		st, e := eruntime.Call(app.Config.EgressRuntime.Root, n, eruntime.Request{Action: "status"})
		if e != nil {
			st.Error = e.Error()
		}
		if jsonlEnabled(cmd) {
			emitJSONL("egress_status", map[string]any{"sandbox": n, "configured": sb, "runtime": st})
			return nil
		}
		writeKV(commandOutput(cmd), []kvPair{
			{Key: "SANDBOX", Value: n},
			{Key: "MODE", Value: sb.EgressMode},
			{Key: "RUNNING", Value: fmt.Sprintf("%t", st.Running)},
			{Key: "LISTEN", Value: st.Listen},
			{Key: "CA FINGERPRINT", Value: st.CAFingerprint},
		})
		return nil
	}}
}

func newEgressVerifyCmd() *cobra.Command {
	return &cobra.Command{Use: "verify [name]", Short: "Adversarially verify proxy-only bypass resistance", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		store, app, e := openStore(cmd)
		if e != nil {
			return e
		}
		defer store.Close()
		n, e := resolveSandboxName(cmd, store, args)
		if e != nil {
			return e
		}
		sb, e := loadSandbox(store, n)
		if e != nil {
			return e
		}
		if sb.EgressMode != "proxy_only" {
			return fmt.Errorf("sandbox %s is not configured for proxy_only", n)
		}
		runner := newRunner(app)
		proxyHost := app.Config.EgressRuntime.GuestHost
		proxyPort := strconv.Itoa(sb.EgressPort)
		script := `set -eu
nft list table inet kaas_egress | grep -q 'policy drop'
nft list table inet kaas_egress | grep -q ` + shellQuote(strconv.Itoa(sb.EgressPort)) + `
timeout 3 bash -c '</dev/tcp/` + proxyHost + `/` + proxyPort + `'
! timeout 3 bash -c '</dev/tcp/1.1.1.1/443'
! timeout 3 bash -c '</dev/tcp/8.8.8.8/53'
! timeout 3 bash -c '</dev/tcp/93.184.216.34/443'
! timeout 3 bash -c 'exec 3<>/dev/udp/1.1.1.1/53; printf x >&3'
! timeout 3 bash -c 'exec 3<>/dev/udp/8.8.8.8/53; printf x >&3'
! env -i PATH=/usr/bin:/bin timeout 3 bash -c '</dev/tcp/1.1.1.1/443'
`
		if e = runner.AdHoc(cmd.Context(), *sb, "shell", script, true); e != nil {
			recordEvent(store, n, "egress_verify", "failed", e.Error())
			return fmt.Errorf("proxy_only bypass verification failed: %w", e)
		}
		recordEvent(store, n, "egress_verify", "ok", "direct IP, DNS TCP/UDP, raw TCP, clean environment denied")
		printStyled(cmd, styleSuccess, "Verified unprivileged proxy-only boundary for %s\n", n)
		return nil
	}}
}
