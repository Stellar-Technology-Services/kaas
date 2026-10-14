package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/spf13/cobra"
)

// Version is set via -ldflags at build time.
var Version = "dev"

type ctxKey struct{}

// GlobalFlags holds root-level flags shared by subcommands.
type GlobalFlags struct {
	ConfigPath string
	StatePath  string
	Verbose    bool
	Quiet      bool
	JSONL      bool
}

// App is the CLI application context loaded for each command run.
type App struct {
	Flags  GlobalFlags
	Config *config.Config
	Log    *slog.Logger
}

// AppFrom returns the App attached to the command context.
func AppFrom(cmd *cobra.Command) *App {
	if v := cmd.Context().Value(ctxKey{}); v != nil {
		if app, ok := v.(*App); ok {
			return app
		}
	}
	return nil
}

func NewRoot() *cobra.Command {
	app := &App{}

	root := &cobra.Command{
		Use:           "kaas",
		Short:         "KaaS — manage Colima coding sandboxes",
		Long:          "kaas orchestrates Colima VMs and Ansible playbooks for isolated coding-agent sandboxes.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			if jsonlEnabled(cmd) {
				emitJSONL("message", map[string]any{"command": cmd.CommandPath(), "message": cmd.Short})
				return
			}
			_ = cmd.Help()
		},
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetContext(context.WithValue(cmd.Context(), ctxKey{}, app))
			app.Log = newLogger(app.Flags.Verbose, app.Flags.Quiet)

			cfg, err := config.Load(config.LoadOptions{
				ConfigPath: app.Flags.ConfigPath,
				StatePath:  app.Flags.StatePath,
			})
			if err != nil {
				return err
			}
			app.Config = cfg
			// kaas.yaml supplies the default protocol; an explicit --jsonl flag
			// always wins over the configured default.
			if !cmd.Root().PersistentFlags().Changed("jsonl") {
				app.Flags.JSONL = cfg.Output.JSONL
			}
			return nil
		},
		PersistentPostRun: func(cmd *cobra.Command, args []string) {
			if app.Flags.JSONL {
				emitJSONL("result", map[string]any{"command": cmd.CommandPath(), "status": "ok"})
			}
		},
	}

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if app.Flags.JSONL {
			emitJSONL("message", map[string]any{"command": cmd.CommandPath(), "message": cmd.Short})
			return
		}
		fmt.Fprint(cmd.OutOrStdout(), cmd.UsageString())
	})

	root.PersistentFlags().StringVar(&app.Flags.ConfigPath, "config", "", "path to kaas.yaml (default: discover via ansible/ansible.cfg)")
	root.PersistentFlags().StringVar(&app.Flags.StatePath, "state", "", "path to SQLite state DB (default: ~/.kaas/state.db)")
	root.PersistentFlags().BoolVarP(&app.Flags.Verbose, "verbose", "v", false, "verbose logging")
	root.PersistentFlags().BoolVarP(&app.Flags.Quiet, "quiet", "q", false, "quiet logging (errors only)")
	root.PersistentFlags().BoolVar(&app.Flags.JSONL, "jsonl", false, "emit machine-readable newline-delimited JSON to stdout")

	// Preserve AddCommand order within help groups (usage journey, not A–Z).
	cobra.EnableCommandSorting = false

	root.AddGroup(
		&cobra.Group{ID: "host", Title: "Host Setup:"},
		&cobra.Group{ID: "lifecycle", Title: "Machine Lifecycle:"},
		&cobra.Group{ID: "guest", Title: "Guest Readiness:"},
		&cobra.Group{ID: "credentials", Title: "Secrets and Egress:"},
		&cobra.Group{ID: "access", Title: "Sandbox Access:"},
		&cobra.Group{ID: "observe", Title: "Inspect and Diagnose:"},
	)

	addGrouped(root, "host",
		newInitCmd(),
		newProjectCmd(),
		newConfigCmd(),
		newDepsCmd(),
		newCompletionCmd(),
	)
	addGrouped(root, "lifecycle",
		newUpCmd(),
		newCreateCmd(),
		newStartCmd(),
		newStopCmd(),
		newExtendCmd(),
		newForkCmd(),
		newTemplateCmd(),
		newDestroyCmd(),
	)
	addGrouped(root, "guest",
		newProvisionCmd(),
		newAgentsCmd(),
		newSetupCmd(),
		newCheckCmd(),
	)
	addGrouped(root, "credentials",
		newSecretsCmd(),
		newEgressCmd(),
		newPubkeyCmd(),
	)
	addGrouped(root, "access",
		newSSHCmd(),
		newExecCmd(),
		newPsCmd(),
		newCPCmd(),
		newPingCmd(),
		newServicesCmd(),
		newDNSCmd(),
		newIngressCmd(),
		newUICmd(),
	)
	addGrouped(root, "observe",
		newListCmd(),
		newInspectCmd(),
		newContractCmd(),
		newDoctorCmd(),
		newEventsCmd(),
	)
	// version stays in Additional Commands (empty GroupID).
	root.AddCommand(newVersionCmd())

	return root
}

// addGrouped assigns a help-section GroupID and registers commands on root.
func addGrouped(root *cobra.Command, groupID string, cmds ...*cobra.Command) {
	for _, c := range cmds {
		c.GroupID = groupID
		root.AddCommand(c)
	}
}

// Execute runs the root command.
func Execute() {
	root := NewRoot()
	if err := root.Execute(); err != nil {
		if root.PersistentFlags().Changed("jsonl") {
			emitJSONL("result", map[string]any{"command": root.CommandPath(), "status": "error", "error": err.Error()})
		} else {
			fmt.Fprintln(os.Stderr, "Error:", err)
		}
		os.Exit(1)
	}
}

// jsonlEnabled reports whether this invocation must keep stdout as JSONL only.
func jsonlEnabled(cmd *cobra.Command) bool {
	app := AppFrom(cmd)
	return app != nil && app.Flags.JSONL
}

// emitJSONL writes one stable machine-readable record to stdout.
func emitJSONL(kind string, fields map[string]any) {
	record := make(map[string]any, len(fields)+2)
	record["schema_version"] = 1
	record["type"] = kind
	for key, value := range fields {
		record[key] = value
	}
	_ = json.NewEncoder(os.Stdout).Encode(record)
}

// commandOutput routes subprocess output away from stdout in JSONL mode.
func commandOutput(cmd *cobra.Command) *os.File {
	if jsonlEnabled(cmd) {
		return os.Stderr
	}
	return os.Stdout
}

// printMessage preserves normal CLI prose while keeping JSONL stdout structured.
func printMessage(cmd *cobra.Command, format string, args ...any) {
	if jsonlEnabled(cmd) {
		emitJSONL("message", map[string]any{"command": cmd.CommandPath(), "message": fmt.Sprintf(format, args...)})
		return
	}
	fmt.Printf(format, args...)
}

func newLogger(verbose, quiet bool) *slog.Logger {
	level := slog.LevelInfo
	switch {
	case quiet:
		level = slog.LevelError
	case verbose:
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print kaas version",
		Run: func(cmd *cobra.Command, args []string) {
			if jsonlEnabled(cmd) {
				emitJSONL("version", map[string]any{"version": Version})
				return
			}
			fmt.Println(Version)
		},
	}
}

func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish|powershell]",
		Short:     "Generate shell completion script",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletion(commandOutput(cmd))
			case "zsh":
				return cmd.Root().GenZshCompletion(commandOutput(cmd))
			case "fish":
				return cmd.Root().GenFishCompletion(commandOutput(cmd), true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(commandOutput(cmd))
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
}
