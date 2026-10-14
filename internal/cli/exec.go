package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

// `kaas exec` is boat.dev's command endpoint on SSH: run something in the
// guest synchronously, or detach it and come back for status, output, or a
// kill. Detached output lives on the guest disk (~/.kaas-runs/<id>.log) so it
// survives stop/start; the host keeps only an index row.

const guestRunsDir = "$HOME/.kaas-runs"

type execOpts struct {
	Cwd     string
	Detach  bool
	Status  string
	Attach  string
	Kill    string
	History bool
	Tail    int
	Timeout time.Duration
}

func newExecCmd() *cobra.Command {
	var opts execOpts
	cmd := &cobra.Command{
		Use:   "exec <name> [flags] -- <command...>",
		Short: "Run a command in a sandbox, or manage detached commands (--detach/--status/--attach/--kill)",
		Long: `Run a shell command inside the guest over SSH.

  kaas exec my-app -- make test                 # synchronous, exit code propagates
  kaas exec my-app --cwd ~/workspace/app --detach -- npm run build
  kaas exec my-app --status <id>                # running / exited / killed / lost, plus the log tail
  kaas exec my-app --attach <id>                # stream the log until the command ends
  kaas exec my-app --kill <id>                  # SIGTERM the process group, SIGKILL after 5s
  kaas exec my-app --history                    # past commands with exit codes

Detached commands keep running when this shell closes. A VM stop ends them
(like a reboot); their logs remain on the guest disk.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, _, err := openStore(cmd)
			if err != nil {
				return err
			}
			defer store.Close()
			sb, err := loadSandbox(store, args[0])
			if err != nil {
				return err
			}
			var command []string
			if dash := cmd.ArgsLenAtDash(); dash >= 0 {
				command = args[dash:]
			}
			switch {
			case opts.History:
				return execHistory(cmd, store, *sb)
			case opts.Status != "":
				return execStatus(cmd, store, *sb, opts.Status, opts.Tail)
			case opts.Attach != "":
				return execAttach(cmd, store, *sb, opts.Attach)
			case opts.Kill != "":
				return execKill(cmd, store, *sb, opts.Kill)
			case len(command) == 0:
				return fmt.Errorf("pass a command after --, or one of --status/--attach/--kill/--history")
			case opts.Detach:
				return execDetach(cmd, store, *sb, opts.Cwd, strings.Join(command, " "))
			default:
				return execSync(cmd, store, *sb, opts.Cwd, strings.Join(command, " "), opts.Timeout)
			}
		},
	}
	cmd.Flags().StringVar(&opts.Cwd, "cwd", "", "guest working directory (default: workspace directory, else $HOME)")
	cmd.Flags().BoolVar(&opts.Detach, "detach", false, "start in the background and print the command id")
	cmd.Flags().StringVar(&opts.Status, "status", "", "show status and log tail for a command id")
	cmd.Flags().StringVar(&opts.Attach, "attach", "", "follow a command's log until it ends; exits with its code")
	cmd.Flags().StringVar(&opts.Kill, "kill", "", "stop a command id (TERM, then KILL after 5s)")
	cmd.Flags().BoolVar(&opts.History, "history", false, "list commands recorded for this sandbox")
	cmd.Flags().IntVar(&opts.Tail, "tail", 40, "log lines to show with --status")
	cmd.Flags().DurationVar(&opts.Timeout, "timeout", 0, "kill a synchronous command after this long (default: none)")
	return cmd
}

func newPsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ps [name]",
		Short: "List the busiest guest processes",
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
			sb, err := loadSandbox(store, name)
			if err != nil {
				return err
			}
			out, err := sshOutput(cmd.Context(), *sb, "ps -eo pid,pcpu,rss,user,etime,args --sort=-pcpu | head -n 16", 20*time.Second)
			if err != nil {
				return err
			}
			if jsonlEnabled(cmd) {
				emitJSONL("processes", map[string]any{"sandbox": name, "rows": parsePs(out)})
				return nil
			}
			fmt.Fprint(commandOutput(cmd), out)
			return nil
		},
	}
}

// --- synchronous ---

func execSync(cmd *cobra.Command, store *state.Store, sb state.Sandbox, cwd, command string, timeout time.Duration) error {
	cwd = defaultCwd(sb, cwd)
	id := newCommandID()
	start := time.Now().UTC()
	_ = store.CreateCommand(state.Command{ID: id, Sandbox: sb.Name, Command: command, Cwd: cwd, StartedAt: start})
	ctx := cmd.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	remote := "cd " + remotePath(cwd) + " && " + command
	sshCmd := sshCommand(ctx, sb, remote)
	sshCmd.Stdin = os.Stdin
	sshCmd.Stdout = commandOutput(cmd)
	sshCmd.Stderr = os.Stderr
	err := sshCmd.Run()
	code := exitCodeOf(err)
	now := time.Now().UTC()
	_ = store.UpdateCommand(state.Command{ID: id, Status: state.CmdExited, ExitCode: &code, EndedAt: &now})
	if jsonlEnabled(cmd) {
		emitJSONL("command", map[string]any{"id": id, "sandbox": sb.Name, "status": state.CmdExited, "exit_code": code, "command": command, "cwd": cwd})
	}
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("command timed out after %s", timeout)
		}
		return &exitError{code: code}
	}
	return nil
}

// exitError propagates the guest exit code without an extra "Error:" line.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("command exited with status %d", e.code) }

// --- detached ---

func execDetach(cmd *cobra.Command, store *state.Store, sb state.Sandbox, cwd, command string) error {
	if sb.ObservedState != state.ObservedRunning {
		return fmt.Errorf("sandbox %s is %s; start it first", sb.Name, sb.ObservedState)
	}
	cwd = defaultCwd(sb, cwd)
	id := newCommandID()
	out, err := sshOutput(cmd.Context(), sb, detachScript(id, cwd, command), 30*time.Second)
	if err != nil {
		return fmt.Errorf("start detached command: %w", err)
	}
	pid, perr := strconv.Atoi(strings.TrimSpace(lastLine(out)))
	if perr != nil {
		return fmt.Errorf("start detached command: could not read pid from %q", strings.TrimSpace(out))
	}
	row := state.Command{ID: id, Sandbox: sb.Name, Command: command, Cwd: cwd, PID: pid, StartedAt: time.Now().UTC()}
	if err := store.CreateCommand(row); err != nil {
		return err
	}
	if jsonlEnabled(cmd) {
		emitJSONL("command", map[string]any{"id": id, "sandbox": sb.Name, "status": state.CmdRunning, "pid": pid, "command": command, "cwd": cwd, "log": guestLogPath(id)})
		return nil
	}
	printStyled(cmd, styleSuccess, "Started %s (pid %d)\n", id, pid)
	printMessage(cmd, "  status: kaas exec %s --status %s\n  attach: kaas exec %s --attach %s\n  kill:   kaas exec %s --kill %s\n", sb.Name, id, sb.Name, id, sb.Name, id)
	return nil
}

// detachScript returns the remote shell that starts command in its own
// session (so the whole group can be killed), logs to ~/.kaas-runs/<id>.log,
// records the exit code in <id>.exit, and prints the pid.
func detachScript(id, cwd, command string) string {
	wrapped := command + "; echo $? > " + guestRunsDir + "/" + id + ".exit"
	return "mkdir -p " + guestRunsDir + " && cd " + remotePath(cwd) +
		" && nohup setsid bash -lc " + shellQuote(wrapped) +
		" > " + guestLogPath(id) + " 2>&1 < /dev/null & echo $!"
}

func guestLogPath(id string) string { return guestRunsDir + "/" + id + ".log" }

// statusScript prints one of: "exit <code>", "running", "lost", then the tail.
func statusScript(id string, pid, tail int) string {
	return "if [ -f " + guestRunsDir + "/" + id + ".exit ]; then echo \"exit $(cat " + guestRunsDir + "/" + id + ".exit)\"; " +
		"elif kill -0 " + strconv.Itoa(pid) + " 2>/dev/null; then echo running; else echo lost; fi; " +
		"echo ---; tail -n " + strconv.Itoa(tail) + " " + guestLogPath(id) + " 2>/dev/null"
}

func execStatus(cmd *cobra.Command, store *state.Store, sb state.Sandbox, id string, tail int) error {
	row, err := store.GetCommand(id)
	if err != nil {
		return err
	}
	if row.Sandbox != sb.Name {
		return fmt.Errorf("command %s belongs to sandbox %s", row.ID, row.Sandbox)
	}
	var lines []string
	if sb.ObservedState == state.ObservedRunning && row.PID > 0 {
		out, err := sshOutput(cmd.Context(), sb, statusScript(row.ID, row.PID, tail), 20*time.Second)
		if err != nil {
			return err
		}
		head, rest, _ := strings.Cut(out, "---\n")
		lines = splitLines(rest)
		applyGuestStatus(store, row, strings.TrimSpace(head))
	} else if row.Running() {
		// The VM is not running, so neither is the command.
		now := time.Now().UTC()
		row.Status, row.EndedAt = state.CmdLost, &now
		_ = store.UpdateCommand(*row)
	}
	if jsonlEnabled(cmd) {
		fields := map[string]any{"id": row.ID, "sandbox": row.Sandbox, "status": row.Status, "pid": row.PID, "command": row.Command, "cwd": row.Cwd, "started_at": row.StartedAt.Format(time.RFC3339), "tail": lines}
		if row.ExitCode != nil {
			fields["exit_code"] = *row.ExitCode
		}
		emitJSONL("command", fields)
		return nil
	}
	status := row.Status
	if row.ExitCode != nil {
		status += " " + strconv.Itoa(*row.ExitCode)
	}
	writeKV(commandOutput(cmd), []kvPair{{Key: "ID", Value: row.ID}, {Key: "STATUS", Value: status}, {Key: "PID", Value: strconv.Itoa(row.PID)}, {Key: "CWD", Value: row.Cwd}, {Key: "COMMAND", Value: row.Command}})
	if len(lines) > 0 {
		fmt.Fprintln(commandOutput(cmd))
		fmt.Fprintln(commandOutput(cmd), strings.Join(lines, "\n"))
	}
	return nil
}

// applyGuestStatus updates the row from the status line the guest printed.
func applyGuestStatus(store *state.Store, row *state.Command, head string) {
	now := time.Now().UTC()
	switch {
	case strings.HasPrefix(head, "exit "):
		code, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(head, "exit ")))
		if row.Status != state.CmdKilled {
			row.Status = state.CmdExited
		}
		row.ExitCode, row.EndedAt = &code, &now
	case head == "running":
		row.Status = state.CmdRunning
	case head == "lost":
		if row.Running() {
			row.Status, row.EndedAt = state.CmdLost, &now
		}
	}
	_ = store.UpdateCommand(*row)
}

func execAttach(cmd *cobra.Command, store *state.Store, sb state.Sandbox, id string) error {
	row, err := store.GetCommand(id)
	if err != nil {
		return err
	}
	if sb.ObservedState != state.ObservedRunning {
		return fmt.Errorf("sandbox %s is %s; use --status to read the last known state", sb.Name, sb.ObservedState)
	}
	// tail --pid exits when the process dies; -n +1 replays from the start.
	remote := "tail -n +1 -f --pid=" + strconv.Itoa(row.PID) + " " + guestLogPath(row.ID) + " 2>/dev/null; true"
	sshCmd := sshCommand(cmd.Context(), sb, remote)
	sshCmd.Stdout = commandOutput(cmd)
	sshCmd.Stderr = os.Stderr
	if err := sshCmd.Run(); err != nil {
		return err
	}
	out, err := sshOutput(cmd.Context(), sb, statusScript(row.ID, row.PID, 0), 20*time.Second)
	if err != nil {
		return err
	}
	head, _, _ := strings.Cut(out, "---\n")
	applyGuestStatus(store, row, strings.TrimSpace(head))
	if row.ExitCode != nil && *row.ExitCode != 0 {
		return &exitError{code: *row.ExitCode}
	}
	return nil
}

func execKill(cmd *cobra.Command, store *state.Store, sb state.Sandbox, id string) error {
	row, err := store.GetCommand(id)
	if err != nil {
		return err
	}
	if !row.Running() {
		printStyled(cmd, styleInfo, "%s is already %s\n", row.ID, row.Status)
		return nil
	}
	if sb.ObservedState != state.ObservedRunning {
		now := time.Now().UTC()
		row.Status, row.EndedAt = state.CmdLost, &now
		return store.UpdateCommand(*row)
	}
	pg := strconv.Itoa(row.PID)
	remote := "kill -TERM -- -" + pg + " 2>/dev/null || kill -TERM " + pg + " 2>/dev/null; " +
		"for i in 1 2 3 4 5; do kill -0 " + pg + " 2>/dev/null || break; sleep 1; done; " +
		"kill -KILL -- -" + pg + " 2>/dev/null; kill -KILL " + pg + " 2>/dev/null; true"
	if _, err := sshOutput(cmd.Context(), sb, remote, 30*time.Second); err != nil {
		return err
	}
	now := time.Now().UTC()
	row.Status, row.EndedAt = state.CmdKilled, &now
	if err := store.UpdateCommand(*row); err != nil {
		return err
	}
	if jsonlEnabled(cmd) {
		emitJSONL("command", map[string]any{"id": row.ID, "sandbox": row.Sandbox, "status": row.Status, "pid": row.PID, "command": row.Command})
		return nil
	}
	printStyled(cmd, styleSuccess, "Killed %s (pid %d)\n", row.ID, row.PID)
	return nil
}

func execHistory(cmd *cobra.Command, store *state.Store, sb state.Sandbox) error {
	list, err := store.ListCommands(sb.Name, 50)
	if err != nil {
		return err
	}
	if jsonlEnabled(cmd) {
		for _, c := range list {
			fields := map[string]any{"id": c.ID, "sandbox": c.Sandbox, "status": c.Status, "pid": c.PID, "command": c.Command, "cwd": c.Cwd, "started_at": c.StartedAt.Format(time.RFC3339)}
			if c.ExitCode != nil {
				fields["exit_code"] = *c.ExitCode
			}
			emitJSONL("command", fields)
		}
		return nil
	}
	if len(list) == 0 {
		printStyled(cmd, styleInfo, "No commands recorded for %s\n", sb.Name)
		return nil
	}
	rows := make([][]string, 0, len(list))
	for _, c := range list {
		status := c.Status
		if c.ExitCode != nil {
			status = fmt.Sprintf("%s %d", c.Status, *c.ExitCode)
		}
		rows = append(rows, []string{c.ID, status, c.StartedAt.Local().Format("2006-01-02 15:04"), c.Command})
	}
	writeTable(commandOutput(cmd), []string{"ID", "STATUS", "STARTED", "COMMAND"}, rows, tableOpts{StatusCols: []int{1}})
	return nil
}

// --- helpers ---

func defaultCwd(sb state.Sandbox, cwd string) string {
	if strings.TrimSpace(cwd) != "" {
		return cwd
	}
	if sb.WorkspaceDirectory != "" {
		return sb.WorkspaceDirectory
	}
	return "~"
}

func newCommandID() string {
	var b [4]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func sshCommand(ctx context.Context, sb state.Sandbox, remote string) *exec.Cmd {
	user, host, key := sandboxSSH(sb)
	return exec.CommandContext(ctx, "ssh", "-i", key, "-p", strconv.Itoa(sb.SSHPort),
		"-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10",
		"-o", "BatchMode=yes", fmt.Sprintf("%s@%s", user, host), remote)
}

// sshOutput runs a remote command and returns its stdout (stderr is surfaced in the error).
func sshOutput(ctx context.Context, sb state.Sandbox, remote string, timeout time.Duration) (string, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	c := sshCommand(ctx, sb, remote)
	var stderr strings.Builder
	c.Stderr = &stderr
	out, err := c.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return string(out), fmt.Errorf("ssh %s: %s", sb.Name, msg)
	}
	return string(out), nil
}

func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return ee.ExitCode()
	}
	return -1
}

func asExitError(err error, target **exec.ExitError) bool {
	for err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// parsePs turns `ps -eo pid,pcpu,rss,user,etime,args` output into rows.
func parsePs(out string) []map[string]any {
	var rows []map[string]any
	sc := bufio.NewScanner(strings.NewReader(out))
	first := true
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if first {
			first = false
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		cpu, _ := strconv.ParseFloat(fields[1], 64)
		rss, _ := strconv.Atoi(fields[2])
		rows = append(rows, map[string]any{"pid": pid, "cpu_percent": cpu, "memory_mb": rss / 1024, "user": fields[3], "elapsed": fields[4], "command": strings.Join(fields[5:], " ")})
	}
	return rows
}
