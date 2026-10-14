package ui

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// CommandDTO is one detached guest command as the detail page shows it.
type CommandDTO struct {
	ID        string
	Command   string
	Cwd       string
	PID       int
	Status    string
	Running   bool
	ExitCode  string
	StartedAt time.Time
	Tail      []string
}

// harnesses map the prompt form's selector onto the installed agent CLIs.
// Each is invoked in prompt mode (-p) so a turn runs without a TTY.
var harnesses = map[string]string{
	"pi":          "pi -p %s",
	"omp":         "omp -p %s",
	"prime-agent": "prime-agent -p %s",
}

var harnessOrder = []string{"pi", "omp", "prime-agent"}

func toCommandDTO(c state.Command) CommandDTO {
	dto := CommandDTO{ID: c.ID, Command: c.Command, Cwd: c.Cwd, PID: c.PID, Status: c.Status, Running: c.Running(), StartedAt: c.StartedAt}
	if c.ExitCode != nil {
		dto.ExitCode = fmt.Sprint(*c.ExitCode)
	}
	return dto
}

// commandDTOs lists a sandbox's commands and, for running ones, asks the CLI
// for a fresh status + log tail (one SSH round-trip each; keep the list short).
// r may be nil when no refresh is wanted.
func (s *Server) commandDTOs(r *http.Request, sb state.Sandbox, refresh bool) []CommandDTO {
	list, err := s.store.ListCommands(sb.Name, 12)
	if err != nil {
		return nil
	}
	out := make([]CommandDTO, 0, len(list))
	for _, c := range list {
		dto := toCommandDTO(c)
		if refresh && r != nil && c.Running() && sb.ObservedState == state.ObservedRunning {
			if recs, err := s.runKaasJSONL(r.Context(), sb.RepoPath, "exec", sb.Name, "--status", c.ID, "--tail", "20"); err == nil {
				for _, rec := range recordsOfType(recs, "command") {
					dto.Status = recString(rec, "status")
					dto.Running = dto.Status == state.CmdRunning
					if code, ok := recInt(rec, "exit_code"); ok {
						dto.ExitCode = fmt.Sprint(code)
					}
					dto.Tail = recStrings(rec, "tail")
				}
			}
		}
		out = append(out, dto)
	}
	return out
}

func (s *Server) handlePartialCommands(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dto := toSandboxDTO(*sb, nil, nil)
	s.renderPartial(w, "partial_commands", pageData{Sandbox: &dto, Commands: s.commandDTOs(r, *sb, true)})
}

// handleRunCommand detaches a shell command in the guest via `kaas exec --detach`.
func (s *Server) handleRunCommand(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if sb.ObservedState != state.ObservedRunning {
		http.Error(w, "sandbox is not running", http.StatusConflict)
		return
	}
	command := strings.TrimSpace(r.FormValue("command"))
	if command == "" {
		http.Error(w, "command is required", http.StatusBadRequest)
		return
	}
	s.detachAndRedirect(w, r, *sb, strings.TrimSpace(r.FormValue("cwd")), command)
}

// handlePrompt wraps a prompt in the chosen harness's prompt mode and detaches it.
func (s *Server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if sb.ObservedState != state.ObservedRunning {
		http.Error(w, "sandbox is not running", http.StatusConflict)
		return
	}
	tmpl, ok := harnesses[strings.TrimSpace(r.FormValue("harness"))]
	if !ok {
		http.Error(w, "unknown harness", http.StatusBadRequest)
		return
	}
	prompt := strings.TrimSpace(r.FormValue("prompt"))
	if prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}
	s.detachAndRedirect(w, r, *sb, "", fmt.Sprintf(tmpl, shellQuote(prompt)))
}

func (s *Server) detachAndRedirect(w http.ResponseWriter, r *http.Request, sb state.Sandbox, cwd, command string) {
	args := []string{"exec", sb.Name, "--detach"}
	if cwd != "" {
		args = append(args, "--cwd", cwd)
	}
	args = append(args, "--", command)
	if _, err := s.runKaasJSONL(r.Context(), sb.RepoPath, args...); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, "/sandboxes/"+sb.Name+"#commands", http.StatusSeeOther)
}

func (s *Server) handleKillCommand(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if _, err := s.runKaasJSONL(r.Context(), sb.RepoPath, "exec", sb.Name, "--kill", r.PathValue("id")); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, "/sandboxes/"+sb.Name+"#commands", http.StatusSeeOther)
}

// ProcessDTO is one row of `kaas ps`.
type ProcessDTO struct {
	PID      int
	CPU      string
	MemoryMB int
	User     string
	Elapsed  string
	Command  string
}

func (s *Server) handlePartialProcesses(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data := pageData{Sandbox: &SandboxDTO{Name: sb.Name}}
	if sb.ObservedState != state.ObservedRunning {
		data.Error = "sandbox is not running"
		s.renderPartial(w, "partial_processes", data)
		return
	}
	recs, err := s.runKaasJSONL(r.Context(), sb.RepoPath, "ps", sb.Name)
	if err != nil {
		data.Error = err.Error()
	}
	for _, rec := range recordsOfType(recs, "processes") {
		rows, _ := rec["rows"].([]any)
		for _, raw := range rows {
			row, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			pid, _ := recInt(row, "pid")
			mem, _ := recInt(row, "memory_mb")
			data.Processes = append(data.Processes, ProcessDTO{PID: pid, CPU: recString(row, "cpu_percent"), MemoryMB: mem, User: recString(row, "user"), Elapsed: recString(row, "elapsed"), Command: recString(row, "command")})
		}
	}
	s.renderPartial(w, "partial_processes", data)
}

// Credentials is the "can this sandbox authenticate?" chip (boat fails fast
// with provider_not_configured; we show the fix command instead).
type Credentials struct {
	Label string // deployed, delegated, none, failed
	Class string
	Fix   string // exact command to run, "" when nothing to do
}

func credentialsFor(sb state.Sandbox, events []state.Event) Credentials {
	if sb.EgressMode != "" && sb.EgressMode != "open" {
		if sb.EgressStatus == "running" {
			return Credentials{Label: "delegated (" + sb.EgressMode + ")", Class: "ok"}
		}
		return Credentials{Label: "delegated · proxy down", Class: "warn", Fix: "kaas egress configure " + sb.Name}
	}
	for _, ev := range events { // newest first
		if ev.Action != "secrets" {
			continue
		}
		switch ev.Status {
		case "ok":
			return Credentials{Label: "deployed", Class: "ok"}
		case "skipped":
			return Credentials{Label: "none", Class: "warn", Fix: "kaas secrets set <KEY> && kaas secrets deploy " + sb.Name}
		case "failed":
			return Credentials{Label: "deploy failed", Class: "bad", Fix: "kaas secrets deploy " + sb.Name}
		}
	}
	return Credentials{Label: "none", Class: "warn", Fix: "kaas secrets deploy " + sb.Name}
}

// probeHostPort reports whether anything accepts TCP on the allocated host
// port — the SSH forward is up and the guest listener answers.
func probeHostPort(port int) bool {
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// systemdUnit is the always-on snippet boat recommends: an enabled unit comes
// back after stop/start, a dev server started by hand does not.
func systemdUnit(sb state.Sandbox, svc ServiceDTO) string {
	user := sb.SSHUser
	if user == "" {
		user = "$USER"
	}
	dir := sb.WorkspaceDirectory
	if dir == "" {
		dir = "/home/" + user
	}
	return fmt.Sprintf(`# /etc/systemd/system/%[1]s.service — then: sudo systemctl daemon-reload && sudo systemctl enable --now %[1]s
[Unit]
Description=%[1]s (kaas %[2]s)
After=network.target

[Service]
User=%[3]s
WorkingDirectory=%[4]s
# Replace with the real start command; it must listen on 0.0.0.0:%[5]d (or 127.0.0.1:%[5]d).
ExecStart=/bin/bash -lc 'exec npm run start -- --port %[5]d'
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
`, svc.Service, sb.Name, user, dir, svc.GuestPort)
}
