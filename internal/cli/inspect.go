package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/contract"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect [name]",
		Short: "Show stored and current facts for one sandbox",
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
			svc := newLifecycleService(store, app)
			discrepancies, _ := svc.Reconcile(cmd.Context(), name)
			contractHash := ""
			var workspaceSources []contract.WorkspaceSourceContract
			if c, err := contract.NewStoreRepository(store).Get(cmd.Context(), name); err == nil {
				contractHash = c.ContentHash
				c.Normalize()
				workspaceSources = c.Workspace.Sources
			} else if contract.IsNotFound(err) {
				contractHash = "(none — will be created on next resume)"
			}
			if jsonlEnabled(cmd) {
				emitJSONL("sandbox", map[string]any{
					"command": cmd.CommandPath(), "sandbox": sb,
					"desired_state": sb.DesiredState, "observed_state": sb.ObservedState,
					"contract_hash": contractHash, "workspace_sources": workspaceSources,
					"discrepancies": discrepancies,
				})
				return nil
			}
			out := commandOutput(cmd)
			writeKV(out, []kvPair{
				{Key: "NAME", Value: sb.Name},
				{Key: "DESIRED", Value: emptyDash(sb.DesiredState)},
				{Key: "OBSERVED", Value: emptyDash(sb.ObservedState)},
				{Key: "SSH", Value: fmt.Sprintf("%s@%s:%d", sb.SSHUser, sb.SSHHost, sb.SSHPort)},
				{Key: "CONTRACT", Value: contractHash},
				{Key: "WORKSPACE REPOSITORY", Value: emptyDash(sb.WorkspaceRepository)},
				{Key: "WORKSPACE BRANCH", Value: emptyDash(sb.WorkspaceBranch)},
				{Key: "WORKSPACE DIRECTORY", Value: emptyDash(sb.WorkspaceDirectory)},
				{Key: "WORKSPACE COMMIT", Value: emptyDash(sb.WorkspaceCommit)},
				{Key: "SETUP", Value: emptyDash(sb.SetupStatus)},
				{Key: "CONFIG SOURCE", Value: emptyDash(sb.ConfigSource)},
				{Key: "CONTROL REPO", Value: emptyDash(sb.RepoPath)},
				{Key: "RESOURCES", Value: fmt.Sprintf("%d CPU / %d GiB RAM / %d GiB disk", sb.CPUs, sb.MemoryGiB, sb.DiskGiB)},
				{Key: "CREATED", Value: sb.CreatedAt.Format("2006-01-02 15:04:05Z07:00")},
				{Key: "UPDATED", Value: sb.UpdatedAt.Format("2006-01-02 15:04:05Z07:00")},
			})
			if len(workspaceSources) > 1 {
				writeTitle(out, "Workspace sources")
				srcRows := make([][]string, 0, len(workspaceSources))
				for i, src := range workspaceSources {
					label := "primary"
					if i > 0 {
						label = fmt.Sprintf("source[%d]", i)
					}
					srcRows = append(srcRows, []string{label, emptyDash(src.Repository), emptyDash(src.Branch), emptyDash(src.Directory)})
				}
				writeTable(out, []string{"SOURCE", "REPOSITORY", "BRANCH", "DIRECTORY"}, srcRows, tableOpts{})
			}
			if len(discrepancies) > 0 {
				printStyled(cmd, styleWarn, "Discrepancies:\n")
				for _, d := range discrepancies {
					printMessage(cmd, "  - %s (%s)\n", d.Detail, d.Kind)
				}
			}
			return nil
		},
	}
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func newEventsCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use: "events [name]", Short: "Show recorded lifecycle events for a sandbox", Args: cobra.MaximumNArgs(1),
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
			if _, err := loadSandbox(store, name); err != nil {
				return err
			}
			events, err := store.ListEvents(name, limit)
			if err != nil {
				return err
			}
			if jsonlEnabled(cmd) {
				for _, event := range events {
					emitJSONL("event", map[string]any{"command": cmd.CommandPath(), "event": event})
				}
				return nil
			}
			rows := make([][]string, 0, len(events))
			for _, event := range events {
				rows = append(rows, []string{
					event.CreatedAt.Format("2006-01-02 15:04:05Z07:00"),
					event.Action,
					event.Status,
					event.Detail,
				})
			}
			writeTable(commandOutput(cmd), []string{"TIME", "ACTION", "STATUS", "DETAIL"}, rows, tableOpts{StatusCols: []int{2}})
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 50, "maximum events to show")
	return cmd
}

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Advice string `json:"advice,omitempty"`
}

func doctorAdvice(tool string) string {
	linux := runtime.GOOS == "linux"
	switch tool {
	case "colima", "limactl":
		if linux {
			return "install Colima and Lima from their GitHub releases (see docs: Install on Linux), then run kaas doctor again"
		}
		return "install Colima and Lima (brew install colima lima), then run kaas doctor again"
	case "ansible", "ansible-playbook", "ansible-galaxy":
		if linux {
			return "install Ansible (sudo apt install ansible or kaas deps --install), then run kaas up <name> to install Galaxy collections"
		}
		return "install Ansible (brew install ansible or kaas deps --install), then run kaas up <name> to install Galaxy collections"
	case "secretspec":
		if linux {
			return "install SecretSpec (curl -sSL https://install.secretspec.dev | sh); optional secrets can be added later"
		}
		return "install SecretSpec (brew install secretspec); optional secrets can be added later"
	case "kvm":
		return "enable KVM (sudo apt install qemu-kvm) and ensure your user can access /dev/kvm"
	default:
		return "install the missing prerequisite and rerun kaas doctor"
	}
}

func doctorKVMCheck() doctorCheck {
	const path = "/dev/kvm"
	info, err := os.Stat(path)
	if err != nil {
		return doctorCheck{Name: "kvm", Status: "missing", Detail: err.Error(), Advice: doctorAdvice("kvm")}
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return doctorCheck{Name: "kvm", Status: "failed", Detail: err.Error(), Advice: doctorAdvice("kvm")}
	}
	_ = f.Close()
	return doctorCheck{Name: "kvm", Status: "ok", Detail: info.Mode().String()}
}

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use: "doctor [name]", Short: "Check host prerequisites and optional sandbox connectivity", Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			app := AppFrom(cmd)
			if app == nil || app.Config == nil {
				return fmt.Errorf("app not initialized")
			}
			checks := []doctorCheck{}
			for _, tool := range []string{"colima", "limactl", "ansible", "ansible-playbook", "ansible-galaxy", "secretspec"} {
				if _, err := exec.LookPath(tool); err != nil {
					checks = append(checks, doctorCheck{Name: tool, Status: "missing", Detail: err.Error(), Advice: doctorAdvice(tool)})
				} else {
					checks = append(checks, doctorCheck{Name: tool, Status: "ok"})
				}
			}
			if runtime.GOOS == "linux" {
				checks = append(checks, doctorKVMCheck())
			}
			ansibleCfg := filepath.Join(app.Config.AnsibleDir(), "ansible.cfg")
			if _, err := os.Stat(ansibleCfg); err != nil {
				checks = append(checks, doctorCheck{Name: "ansible assets", Status: "missing", Detail: ansibleCfg, Advice: "run kaas init, or pass a configuration with a valid paths.ansible_dir"})
			} else {
				checks = append(checks, doctorCheck{Name: "ansible assets", Status: "ok", Detail: ansibleCfg})
			}
			if len(args) == 1 {
				store, err := state.Open(app.Config.State.Path)
				if err != nil {
					return err
				}
				defer store.Close()
				sb, err := loadSandbox(store, args[0])
				if err != nil {
					return err
				}
				runner := newRunner(app)
				if err := runner.Ping(cmd.Context(), *sb); err != nil {
					checks = append(checks, doctorCheck{Name: "sandbox SSH/Ansible", Status: "failed", Detail: err.Error(), Advice: "run kaas start <name>; if it is running, inspect `kaas events <name>` and rerun kaas provision <name>"})
				} else {
					checks = append(checks, doctorCheck{Name: "sandbox SSH/Ansible", Status: "ok"})
				}
				svc := newLifecycleService(store, app)
				if discrepancies, err := svc.Reconcile(cmd.Context(), args[0]); err != nil {
					checks = append(checks, doctorCheck{Name: "reconcile", Status: "failed", Detail: err.Error(), Advice: "ensure Colima is available, then rerun kaas doctor <name>"})
				} else if len(discrepancies) == 0 {
					checks = append(checks, doctorCheck{Name: "reconcile", Status: "ok"})
				} else {
					for _, d := range discrepancies {
						checks = append(checks, doctorCheck{Name: "reconcile:" + string(d.Kind), Status: "warn", Detail: d.Detail, Advice: "inspect with kaas inspect " + args[0]})
					}
				}
			}
			if jsonlEnabled(cmd) {
				for _, check := range checks {
					emitJSONL("check", map[string]any{"command": cmd.CommandPath(), "check": check})
				}
				return nil
			}
			rows := make([][]string, 0, len(checks))
			for _, check := range checks {
				rows = append(rows, []string{check.Name, strings.ToUpper(check.Status), check.Detail, check.Advice})
			}
			writeTable(commandOutput(cmd), []string{"CHECK", "STATUS", "DETAIL", "ADVICE"}, rows, tableOpts{StatusCols: []int{1}})
			return nil
		},
	}
}
