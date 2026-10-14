package cli

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/Stellar-Technology-Services/kaas/internal/ui"
	"github.com/spf13/cobra"
)

func newUICmd() *cobra.Command {
	serve := newUIServeCmd()
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Local web dashboard for sandboxes and published services",
		Long:  "Optional localhost UI (default 127.0.0.1:18181). Foreground serve, or start/stop a background daemon. Also usable via brew services on macOS.",
		RunE:  serve.RunE, // kaas ui == kaas ui serve (default listen from config)
	}
	cmd.AddCommand(
		serve,
		newUIStartCmd(),
		newUIStopCmd(),
		newUIStatusCmd(),
		newUIOpenCmd(),
	)
	return cmd
}

func uiOptions(app *App, listen string) (ui.Options, error) {
	if app == nil || app.Config == nil {
		return ui.Options{}, fmt.Errorf("app not initialized")
	}
	if listen == "" {
		listen = app.Config.UIRuntime.Listen
	}
	exe, err := os.Executable()
	if err != nil {
		exe = ""
	}
	return ui.Options{
		Listen:         listen,
		StatePath:      app.Config.State.Path,
		Root:           app.Config.UIRuntime.Root,
		LogsDir:        app.Config.Paths.LogsDir,
		IngressRuntime: app.Config.IngressRuntime,
		Defaults:       app.Config.Defaults,
		Executable:     exe,
	}, nil
}

func newUIServeCmd() *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Run the dashboard in the foreground",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			opts, err := uiOptions(app, listen)
			if err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			printStyled(cmd, styleInfo, "KaaS UI listening on http://%s (Ctrl-C to stop)\n", opts.Listen)
			return ui.ListenAndServe(ctx, opts)
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "loopback listen address (default: ui_runtime.listen)")
	return cmd
}

func newUIStartCmd() *cobra.Command {
	var listen string
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the dashboard in the background",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			opts, err := uiOptions(app, listen)
			if err != nil {
				return err
			}
			if err := ui.StartDaemon(opts); err != nil {
				return err
			}
			st := ui.StatusDaemon(opts)
			if jsonlEnabled(cmd) {
				emitJSONL("ui", map[string]any{
					"action":  "start",
					"running": st.Running,
					"pid":     st.PID,
					"listen":  st.Listen,
				})
				return nil
			}
			printStyled(cmd, styleSuccess, "Started KaaS UI on http://%s (pid %d)\n", opts.Listen, st.PID)
			printMessage(cmd, "  logs: %s\n", ui.LogFile(opts.Root))
			return nil
		},
	}
	cmd.Flags().StringVar(&listen, "listen", "", "loopback listen address (default: ui_runtime.listen)")
	return cmd
}

func newUIStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the background dashboard",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			opts, err := uiOptions(app, "")
			if err != nil {
				return err
			}
			if err := ui.StopDaemon(opts); err != nil {
				return err
			}
			if jsonlEnabled(cmd) {
				emitJSONL("ui", map[string]any{"action": "stop", "ok": true})
				return nil
			}
			printStyled(cmd, styleSuccess, "Stopped KaaS UI\n")
			return nil
		},
	}
}

func newUIStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show dashboard daemon status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			opts, err := uiOptions(app, "")
			if err != nil {
				return err
			}
			st := ui.StatusDaemon(opts)
			if jsonlEnabled(cmd) {
				emitJSONL("ui_status", map[string]any{
					"running": st.Running,
					"pid":     st.PID,
					"listen":  st.Listen,
					"error":   st.Error,
				})
				return nil
			}
			pairs := []kvPair{
				{Key: "RUNNING", Value: fmt.Sprintf("%t", st.Running)},
				{Key: "PID", Value: fmt.Sprintf("%d", st.PID)},
				{Key: "LISTEN", Value: "http://" + st.Listen},
			}
			if st.Error != "" {
				pairs = append(pairs, kvPair{Key: "ERROR", Value: st.Error})
			}
			writeKV(commandOutput(cmd), pairs)
			return nil
		},
	}
}

func newUIOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the dashboard in a browser",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			opts, err := uiOptions(app, "")
			if err != nil {
				return err
			}
			url := "http://" + opts.Listen + "/"
			st := ui.StatusDaemon(opts)
			if !st.Running {
				printStyled(cmd, styleWarn, "UI is not running. Start with: kaas ui start\n  (or kaas ui serve)\n")
			}
			printStyled(cmd, styleInfo, "%s\n", url)
			openBrowser(url)
			return nil
		},
	}
}

func openBrowser(url string) {
	switch runtime.GOOS {
	case "darwin":
		_ = exec.Command("open", url).Start()
	case "linux":
		_ = exec.Command("xdg-open", url).Start()
	}
}
