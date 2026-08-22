package cli

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/ingress"
	"github.com/Stellar-Technology-Services/kaas/internal/ingress/dnsconfig"
	iruntime "github.com/Stellar-Technology-Services/kaas/internal/ingress/runtime"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

func newServicesCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "services",
		Aliases: []string{"service"},
		Short:   "List and sync host-published sandbox services (*.kaas.test)",
	}
	cmd.AddCommand(
		newServicesListCmd(),
		newServicesSyncCmd(),
		newServicesOpenCmd(),
		newServicesStatusCmd(),
	)
	return cmd
}

func newServicesListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list [name]",
		Short: "List registered services (optional sandbox filter)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			list, err := store.ListServices(filter)
			if err != nil {
				return err
			}
			dnsReady, _, _ := dnsconfig.Status(app.Config.IngressRuntime.Domain)
			if jsonlEnabled(cmd) {
				rows := make([]map[string]any, 0, len(list))
				for _, svc := range list {
					rows = append(rows, serviceRow(svc, app.Config.IngressRuntime, dnsReady))
				}
				emitJSONL("services", map[string]any{"command": cmd.CommandPath(), "services": rows})
				return nil
			}
			if len(list) == 0 {
			printStyled(cmd, styleInfo, "No services registered. Declare workspace.services in kaas.yaml and run kaas services sync.\n")
				return nil
			}
			rows := make([]serviceTableRow, 0, len(list))
			for _, svc := range list {
				u := ingress.URLFor(svc.Scheme, svc.Hostname, app.Config.IngressRuntime.Listen, dnsReady)
				rows = append(rows, serviceTableRow{
					Sandbox:  svc.Sandbox,
					Service:  svc.Service,
					Hostname: svc.Hostname,
					URL:      u,
					Guest:    svc.GuestPort,
					Host:     svc.HostPort,
					Status:   svc.Status,
				})
			}
			writeServicesTable(commandOutput(cmd), rows)
			return nil
		},
	}
}

func serviceRow(svc state.Service, rt config.IngressRuntime, dnsReady bool) map[string]any {
	u := ingress.URLFor(svc.Scheme, svc.Hostname, rt.Listen, dnsReady)
	return map[string]any{
		"sandbox":    svc.Sandbox,
		"service":    svc.Service,
		"hostname":   svc.Hostname,
		"url":        u,
		"guest_port": svc.GuestPort,
		"host_port":  svc.HostPort,
		"scheme":     svc.Scheme,
		"status":     svc.Status,
		"dns_ready":  dnsReady,
	}
}

func newServicesSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync [name]",
		Short: "Allocate host ports, update registry, and reload ingress forwards",
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
			n, err := syncSandboxServices(cmd, store, app, name)
			if err != nil {
				return err
			}
			printStyled(cmd, styleSuccess, "Synced %d service(s) for %s\n", n, name)
			return printPublishedURLs(cmd, store, app, name)
		},
	}
}

func newServicesOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [name] <service>",
		Short: "Print (and try to open) the URL for a published service",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			var name, service string
			if len(args) == 1 {
				service = args[0]
				name, err = resolveSandboxName(cmd, store, nil)
			} else {
				name, service = args[0], args[1]
			}
			if err != nil {
				return err
			}
			svc, err := store.GetService(name, service)
			if err != nil {
				return err
			}
			dnsReady, _, _ := dnsconfig.Status(app.Config.IngressRuntime.Domain)
			u := ingress.URLFor(svc.Scheme, svc.Hostname, app.Config.IngressRuntime.Listen, dnsReady)
			if jsonlEnabled(cmd) {
				emitJSONL("service_url", map[string]any{"sandbox": name, "service": service, "url": u, "hostname": svc.Hostname})
				return nil
			}
			printStyled(cmd, styleInfo, "%s\n", u)
			if !dnsReady {
				printStyled(cmd, styleWarn, "Hint: run kaas dns enable so browsers resolve %s (or curl --resolve %s:%s:127.0.0.1 %s)\n",
					svc.Hostname, svc.Hostname, listenPort(app.Config.IngressRuntime.Listen), u)
			}
			_ = exec.Command("open", u).Start() // macOS; ignore errors on Linux
			return nil
		},
	}
}

func newServicesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show ingress runtime status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			_, app, err := openStore(cmd)
			if err != nil {
				return err
			}
			st, callErr := iruntime.Call(app.Config.IngressRuntime.Root, iruntime.Request{Action: "status"})
			if callErr != nil {
				st.Error = callErr.Error()
			}
			dnsReady, dnsDetail, _ := dnsconfig.Status(app.Config.IngressRuntime.Domain)
			if jsonlEnabled(cmd) {
				emitJSONL("ingress_status", map[string]any{"runtime": st, "dns_enabled": dnsReady, "dns_detail": dnsDetail})
				return nil
			}
			pairs := []kvPair{
				{Key: "RUNNING", Value: fmt.Sprintf("%t", st.Running)},
				{Key: "LISTEN", Value: st.Listen},
				{Key: "DNS", Value: st.DNSListen},
				{Key: "DOMAIN", Value: st.Domain},
				{Key: "ROUTES", Value: fmt.Sprintf("%d", st.Routes)},
				{Key: "DNS_ENABLED", Value: fmt.Sprintf("%t", dnsReady)},
				{Key: "DNS_DETAIL", Value: dnsDetail},
			}
			if st.Error != "" {
				pairs = append(pairs, kvPair{Key: "ERROR", Value: st.Error})
			}
			writeKV(commandOutput(cmd), pairs)
			return nil
		},
	}
}

func listenPort(listen string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" {
		return "18000"
	}
	return port
}

// printPublishedURLs prints a short footer of Host-based service URLs for one sandbox.
// No-op when the sandbox has no registered services.
func printPublishedURLs(cmd *cobra.Command, store *state.Store, app *App, name string) error {
	list, err := store.ListServices(name)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return nil
	}
	rt := app.Config.IngressRuntime
	dnsReady, _, _ := dnsconfig.Status(rt.Domain)
	catalog := ingress.URLFor("http", ingress.CatalogHostname(rt.Domain), rt.Listen, dnsReady)
	if jsonlEnabled(cmd) {
		rows := make([]map[string]any, 0, len(list))
		for _, svc := range list {
			rows = append(rows, serviceRow(svc, rt, dnsReady))
		}
		emitJSONL("published_urls", map[string]any{
			"command":  cmd.CommandPath(),
			"sandbox":  name,
			"services": rows,
			"catalog":  catalog,
		})
		return nil
	}
	lines := make([]publishedURLLine, 0, len(list))
	for _, svc := range list {
		lines = append(lines, publishedURLLine{
			Label: svc.Service,
			URL:   ingress.URLFor(svc.Scheme, svc.Hostname, rt.Listen, dnsReady),
		})
	}
	dnsHint := ""
	if !dnsReady {
		dnsHint = fmt.Sprintf("run kaas dns enable so browsers resolve *.%s without --resolve", rt.Domain)
	}
	writePublishedURLsFooter(commandOutput(cmd), name, lines, catalog, dnsHint)
	return nil
}

func syncSandboxServices(cmd *cobra.Command, store *state.Store, app *App, name string) (int, error) {
	_, err := loadSandbox(store, name)
	if err != nil {
		return 0, err
	}
	c, err := loadPersistedContract(store, name)
	if err != nil {
		return 0, err
	}
	domain := app.Config.IngressRuntime.Domain
	desired := map[string]contract.ServiceContract{}
	for _, svc := range c.Workspace.Services {
		desired[svc.Name] = svc
	}

	existing, err := store.ListServices(name)
	if err != nil {
		return 0, err
	}
	existingByName := map[string]state.Service{}
	for _, svc := range existing {
		existingByName[svc.Service] = svc
	}

	used, err := store.UsedServicePorts()
	if err != nil {
		return 0, err
	}
	used = sandbox.MergeUsed(used)

	for serviceName := range existingByName {
		if _, ok := desired[serviceName]; ok {
			continue
		}
		if err := store.DeleteService(name, serviceName); err != nil {
			return 0, err
		}
		recordEvent(store, name, "service_remove", "ok", serviceName)
	}

	count := 0
	for _, spec := range c.Workspace.Services {
		scheme := spec.Scheme
		if scheme == "" {
			scheme = "http"
		}
		hostName := ingress.Hostname(spec.Name, name, domain)
		row, ok := existingByName[spec.Name]
		hostPort := 0
		if ok && row.GuestPort == spec.Port && row.Hostname == hostName {
			hostPort = row.HostPort
		} else {
			p, err := sandbox.NextPort(app.Config.IngressRuntime.HostPortBase, used)
			if err != nil {
				return 0, err
			}
			hostPort = p
			used = append(used, hostPort)
		}
		st := state.Service{
			Sandbox:   name,
			Service:   spec.Name,
			GuestPort: spec.Port,
			HostPort:  hostPort,
			Scheme:    scheme,
			Hostname:  hostName,
			Status:    "registered",
		}
		if err := store.UpsertService(st); err != nil {
			return 0, err
		}
		count++
	}

	remaining, err := store.ListServices("")
	if err != nil {
		return count, err
	}
	if len(remaining) == 0 {
		reloadIngressBestEffort(store, app)
		stopIngressIfIdle(store, app)
		recordEvent(store, name, "services_sync", "ok", "0 services")
		return 0, nil
	}
	if err := ensureIngress(cmd, store, app); err != nil {
		recordEvent(store, name, "services_sync", "failed", err.Error())
		return count, err
	}
	recordEvent(store, name, "services_sync", "ok", fmt.Sprintf("%d services", count))
	return count, nil
}

func loadPersistedContract(store *state.Store, name string) (contract.Contract, error) {
	snap, err := store.GetContract(name)
	if err != nil {
		return contract.Contract{}, fmt.Errorf("sandbox %s has no contract; run kaas create/up first: %w", name, err)
	}
	return contract.ParseJSON([]byte(snap.ContractJSON))
}

func ensureIngress(cmd *cobra.Command, store *state.Store, app *App) error {
	routes, err := buildIngressRoutes(store)
	if err != nil {
		return err
	}
	root := app.Config.IngressRuntime.Root
	if st, err := iruntime.Call(root, iruntime.Request{Action: "status"}); err == nil && st.Running {
		_, err := iruntime.Call(root, iruntime.Request{Action: "reload", Routes: routes})
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{}
	if app.Config.ConfigPath != "" {
		args = append(args, "--config", app.Config.ConfigPath)
	}
	args = append(args, "--state", app.Config.State.Path, "ingress", "serve", "--runtime-root", root)
	child := exec.Command(exe, args...)
	stdin, err := child.StdinPipe()
	if err != nil {
		return err
	}
	child.Stdout, child.Stderr = openDevNull(), openDevNull()
	if err := child.Start(); err != nil {
		return err
	}
	bootstrap := iruntime.Bootstrap{
		Listen:    app.Config.IngressRuntime.Listen,
		DNSListen: app.Config.IngressRuntime.DNSListen,
		Domain:    app.Config.IngressRuntime.Domain,
		Routes:    routes,
	}
	if err := json.NewEncoder(stdin).Encode(bootstrap); err != nil {
		return err
	}
	_ = stdin.Close()
	go child.Wait()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if st, err := iruntime.Call(root, iruntime.Request{Action: "status"}); err == nil && st.Running {
			_ = cmd
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("ingress runtime failed to become ready")
}

func buildIngressRoutes(store *state.Store) ([]ingress.Route, error) {
	list, err := store.ListServices("")
	if err != nil {
		return nil, err
	}
	out := make([]ingress.Route, 0, len(list))
	for _, svc := range list {
		sb, err := store.GetSandbox(svc.Sandbox)
		if err != nil {
			continue
		}
		user, host, key := sandboxSSH(*sb)
		// Only forward when the sandbox is intended to be up.
		if sb.ObservedState != "" && sb.ObservedState != "running" && sb.DesiredState != "running" {
			continue
		}
		out = append(out, ingress.Route{
			Hostname:  svc.Hostname,
			HostPort:  svc.HostPort,
			Scheme:    svc.Scheme,
			Sandbox:   svc.Sandbox,
			Service:   svc.Service,
			GuestPort: svc.GuestPort,
			SSHHost:   host,
			SSHPort:   sb.SSHPort,
			SSHUser:   user,
			SSHKey:    key,
		})
	}
	return out, nil
}

func newIngressCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "ingress", Short: "Manage host HTTP ingress and local DNS runtime"}
	cmd.AddCommand(newIngressServeCmd())
	return cmd
}

func newIngressServeCmd() *cobra.Command {
	var root string
	c := &cobra.Command{
		Use:   "serve",
		Short: "Run the host ingress proxy/DNS (normally lifecycle-managed)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, cancel := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()
			if root == "" {
				app := AppFrom(cmd)
				if app != nil && app.Config != nil {
					root = app.Config.IngressRuntime.Root
				}
			}
			return iruntime.Serve(ctx, root, os.Stdin)
		},
	}
	c.Flags().StringVar(&root, "runtime-root", "", "host ingress runtime root")
	return c
}

func newDNSCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "dns", Short: "Install or remove local *.kaas.test resolver hooks"}
	cmd.AddCommand(newDNSEnableCmd(), newDNSDisableCmd(), newDNSStatusCmd())
	return cmd
}

func newDNSEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Install split-horizon DNS for ingress_runtime.domain (requires sudo)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			rt := app.Config.IngressRuntime
			if err := dnsconfig.Enable(rt.Domain, rt.DNSListen); err != nil {
				return err
			}
			printStyled(cmd, styleSuccess, "Enabled local DNS for %s → %s\n", rt.Domain, rt.DNSListen)
			printMessage(cmd, "Ensure ingress is running (kaas services sync) so the DNS stub answers queries.\n")
			return nil
		},
	}
}

func newDNSDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Remove local DNS resolver hooks installed by kaas dns enable",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			if err := dnsconfig.Disable(app.Config.IngressRuntime.Domain); err != nil {
				return err
			}
			printStyled(cmd, styleSuccess, "Disabled local DNS for %s\n", app.Config.IngressRuntime.Domain)
			return nil
		},
	}
}

func newDNSStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether local DNS hooks are installed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			ok, detail, err := dnsconfig.Status(app.Config.IngressRuntime.Domain)
			if err != nil {
				return err
			}
			if jsonlEnabled(cmd) {
				emitJSONL("dns_status", map[string]any{"enabled": ok, "detail": detail, "domain": app.Config.IngressRuntime.Domain})
				return nil
			}
			writeKV(commandOutput(cmd), []kvPair{
				{Key: "DOMAIN", Value: app.Config.IngressRuntime.Domain},
				{Key: "ENABLED", Value: fmt.Sprintf("%t", ok)},
				{Key: "DETAIL", Value: detail},
				{Key: "DNS_LISTEN", Value: app.Config.IngressRuntime.DNSListen},
			})
			return nil
		},
	}
}

// reloadIngressBestEffort reloads routes after stop/destroy; ignores missing daemon.
func reloadIngressBestEffort(store *state.Store, app *App) {
	if app == nil || app.Config == nil {
		return
	}
	routes, err := buildIngressRoutes(store)
	if err != nil {
		return
	}
	if iruntime.Running(app.Config.IngressRuntime.Root) {
		_, _ = iruntime.Call(app.Config.IngressRuntime.Root, iruntime.Request{Action: "reload", Routes: routes})
	}
}

// stopIngressIfIdle stops the daemon when no services remain.
func stopIngressIfIdle(store *state.Store, app *App) {
	if app == nil || app.Config == nil {
		return
	}
	list, err := store.ListServices("")
	if err != nil || len(list) > 0 {
		return
	}
	if iruntime.Running(app.Config.IngressRuntime.Root) {
		_, _ = iruntime.Call(app.Config.IngressRuntime.Root, iruntime.Request{Action: "stop"})
	}
}
