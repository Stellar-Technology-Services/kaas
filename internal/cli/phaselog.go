package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/ansible"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
	"github.com/spf13/cobra"
)

// phaseLog tees a readiness phase's Ansible/SSH output into
// <paths.logs_dir>/<sandbox>/<phase>.log so the UI (and a later operator) can
// read why a phase failed without re-running it. Output still reaches the
// terminal as before.
type phaseLog struct {
	file *os.File
}

// attachPhaseLog points the runner's streams at the terminal and the log file.
// A missing or unwritable logs dir degrades to terminal-only output.
func attachPhaseLog(app *App, runner *ansible.Runner, sandbox, phase string) *phaseLog {
	if app == nil || app.Config == nil || app.Config.Paths.LogsDir == "" {
		return &phaseLog{}
	}
	path := PhaseLogPath(app.Config.Paths.LogsDir, sandbox, phase)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return &phaseLog{}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return &phaseLog{}
	}
	fmt.Fprintf(f, "# kaas %s %s · %s\n", phase, sandbox, time.Now().UTC().Format(time.RFC3339))
	runner.Stdout = io.MultiWriter(runner.Stdout, f)
	runner.Stderr = io.MultiWriter(runner.Stderr, f)
	return &phaseLog{file: f}
}

func (p *phaseLog) Close() {
	if p != nil && p.file != nil {
		_ = p.file.Close()
	}
}

// PhaseLogPath is shared with the UI so both sides agree on the layout.
func PhaseLogPath(logsDir, sandbox, phase string) string {
	return filepath.Join(logsDir, sandbox, phase+".log")
}

// runPreStopHook executes scripts/kaas-prestop in the primary workspace
// directory when it exists. Boat calls this the pre-stop command: a chance to
// flush app state before the VM goes away. Failure is recorded, never fatal.
func runPreStopHook(cmd *cobra.Command, store *state.Store, sb state.Sandbox) {
	dir := strings.TrimSpace(sb.WorkspaceDirectory)
	if dir == "" || sb.ObservedState != state.ObservedRunning {
		return
	}
	script := dir + "/scripts/kaas-prestop"
	exists, err := workspaceFileExists(cmd, sb, script)
	if err != nil || !exists {
		return
	}
	user, host, key := sandboxSSH(sb)
	remote := "cd " + remotePath(dir) + " && ./scripts/kaas-prestop"
	sshCmd := exec.CommandContext(cmd.Context(), "ssh", "-i", key, "-p", fmt.Sprint(sb.SSHPort), "-o", "IdentitiesOnly=yes", "-o", "StrictHostKeyChecking=accept-new", "-o", "ConnectTimeout=10", fmt.Sprintf("%s@%s", user, host), remote)
	sshCmd.Stdout = commandOutput(cmd)
	var tail strings.Builder
	sshCmd.Stderr = io.MultiWriter(os.Stderr, &tail)
	printStyled(cmd, styleInfo, "Running scripts/kaas-prestop on %s\n", sb.Name)
	if err := sshCmd.Run(); err != nil {
		detail := strings.TrimSpace(lastLine(tail.String()))
		if detail == "" {
			detail = err.Error()
		}
		recordEvent(store, sb.Name, "prestop", "failed", detail)
		printStyled(cmd, styleWarn, "pre-stop hook failed (%s); stopping anyway\n", detail)
		return
	}
	recordEvent(store, sb.Name, "prestop", "ok", "")
}

func lastLine(s string) string {
	sc := bufio.NewScanner(strings.NewReader(s))
	last := ""
	for sc.Scan() {
		if t := strings.TrimSpace(sc.Text()); t != "" {
			last = t
		}
	}
	return last
}

// confirmPrompt asks a y/N question on the terminal.
func confirmPrompt(cmd *cobra.Command, question string) bool {
	fmt.Fprint(commandOutput(cmd), question)
	answer, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false
	}
	answer = strings.ToLower(strings.TrimSpace(answer))
	return answer == "y" || answer == "yes"
}
