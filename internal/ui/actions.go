package ui

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// sizeOption is one row of the create form's size selector.
type sizeOption struct {
	Name      string
	CPUs      int
	MemoryGiB int
	DiskGiB   int
	Warn      bool // starting this size now would cross the capacity threshold
}

// createForm carries the New sandbox page state (and validation errors back).
type createForm struct {
	Name       string
	Size       string
	Trust      string
	Repo       string
	Branch     string
	ProjectDir string
	From       string // template to clone instead of provisioning
	AutoStop   string // TTL spec or "never"
	Sizes      []sizeOption
	Templates  []SandboxDTO
	AutoStops  []string
	Capacity   HostCapacity
	Error      string
}

// autoStopChoices are the create form's TTL options; the configured default is
// always included so it can be preselected.
func autoStopChoices(defaultSpec string) []string {
	choices := []string{"never", "1h", "4h", "12h", "1d", "7d"}
	d := strings.TrimSpace(defaultSpec)
	if d == "" {
		d = "never"
	}
	for _, c := range choices {
		if c == d {
			return choices
		}
	}
	return append([]string{d}, choices...)
}

func (s *Server) templateDTOs() []SandboxDTO {
	list, err := s.listSandboxDTOs()
	if err != nil {
		return nil
	}
	var out []SandboxDTO
	for _, sb := range list {
		if sb.IsTemplate {
			out = append(out, sb)
		}
	}
	return out
}

func (s *Server) sizeOptions(cap HostCapacity) []sizeOption {
	d := s.opts.Defaults
	out := make([]sizeOption, 0, len(d.Sizes)+1)
	for _, name := range d.SizeNames() {
		sz, err := d.ResolveSize(name)
		if err != nil {
			continue
		}
		out = append(out, sizeOption{Name: name, CPUs: sz.CPUs, MemoryGiB: sz.MemoryGiB, DiskGiB: sz.DiskGiB, Warn: cap.wouldExceed(sz.MemoryGiB)})
	}
	return out
}

func (s *Server) handleNewSandbox(w http.ResponseWriter, r *http.Request) {
	list, err := s.store.ListSandboxes("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	taken := make(map[string]bool, len(list))
	for _, sb := range list {
		taken[sb.Name] = true
	}
	cap := hostCapacity(list)
	defaultStop := s.opts.Defaults.AutoStop
	if defaultStop == "" {
		defaultStop = "never"
	}
	form := createForm{
		Name: suggestName(taken), Size: "default", Trust: "trusted",
		From: strings.TrimSpace(r.URL.Query().Get("from")), AutoStop: defaultStop,
		Sizes: s.sizeOptions(cap), Templates: s.templateDTOs(), AutoStops: autoStopChoices(defaultStop), Capacity: cap,
	}
	s.render(w, "sandbox_new", pageData{Title: "New sandbox", Active: "sandboxes", Form: &form})
}

// handleCreateSandbox validates the form and starts a `kaas up` job. The CLI
// remains the authority: the UI only translates fields into flags.
func (s *Server) handleCreateSandbox(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	list, _ := s.store.ListSandboxes("")
	cap := hostCapacity(list)
	form := createForm{
		Name:       normalizeName(r.FormValue("name")),
		Size:       strings.TrimSpace(r.FormValue("size")),
		Trust:      strings.TrimSpace(r.FormValue("trust")),
		Repo:       strings.TrimSpace(r.FormValue("repo")),
		Branch:     strings.TrimSpace(r.FormValue("branch")),
		ProjectDir: strings.TrimSpace(r.FormValue("project_dir")),
		From:       strings.TrimSpace(r.FormValue("from")),
		AutoStop:   strings.TrimSpace(r.FormValue("auto_stop")),
		Sizes:      s.sizeOptions(cap),
		Templates:  s.templateDTOs(),
		AutoStops:  autoStopChoices(s.opts.Defaults.AutoStop),
		Capacity:   cap,
	}
	fail := func(msg string) {
		form.Error = msg
		w.WriteHeader(http.StatusBadRequest)
		s.render(w, "sandbox_new", pageData{Title: "New sandbox", Active: "sandboxes", Form: &form})
	}
	if err := identity.ValidateName(form.Name); err != nil {
		fail(err.Error())
		return
	}
	if _, err := s.store.GetSandbox(form.Name); err == nil {
		fail(fmt.Sprintf("sandbox %q already exists", form.Name))
		return
	} else if !errors.Is(err, state.ErrNotFound) {
		fail(err.Error())
		return
	}
	if _, err := s.opts.Defaults.ResolveSize(form.Size); err != nil {
		fail(err.Error())
		return
	}
	switch form.Trust {
	case "", "trusted", "untrusted":
	default:
		fail("trust must be trusted or untrusted")
		return
	}
	if form.Branch != "" && form.Repo == "" {
		fail("branch requires a repository")
		return
	}
	if form.From != "" {
		tpl, err := s.store.GetSandbox(form.From)
		if err != nil {
			fail("template not found: " + form.From)
			return
		}
		if !tpl.IsTemplate() {
			fail(fmt.Sprintf("%q is a sandbox, not a template", form.From))
			return
		}
	}
	if _, err := config.ParseAutoStop(form.AutoStop); err != nil {
		fail("auto-stop: " + err.Error())
		return
	}
	dir := ""
	if form.ProjectDir != "" {
		expanded := expandHome(form.ProjectDir)
		info, err := os.Stat(expanded)
		if err != nil || !info.IsDir() {
			fail("project directory does not exist: " + form.ProjectDir)
			return
		}
		dir = expanded
	}

	args := []string{form.Name}
	if form.Size != "" && form.Size != "default" {
		args = append(args, "--size", form.Size)
	}
	if form.Trust != "" && form.Trust != "trusted" {
		args = append(args, "--trust", form.Trust)
	}
	if form.Repo != "" {
		args = append(args, "--repo", form.Repo)
		if form.Branch != "" {
			args = append(args, "--branch", form.Branch)
		}
	}
	if form.From != "" {
		args = append(args, "--from", form.From)
	}
	if form.AutoStop != "" {
		args = append(args, "--auto-stop", form.AutoStop)
	}
	s.startJobAndRedirect(w, r, startRequest{Sandbox: form.Name, Verb: "up", Args: args, Dir: dir}, "/sandboxes/"+form.Name)
}

// handleSandboxAction maps a lifecycle button onto one CLI invocation.
func (s *Server) handleSandboxAction(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	action := r.PathValue("action")
	sb, err := s.store.GetSandbox(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var active *state.Job
	if job, err := s.store.ActiveJob(name); err == nil {
		active = job
	}
	allowed := allowedActions(*sb, active)
	if !actionAllowed(allowed, action) {
		http.Error(w, fmt.Sprintf("%s is not allowed while %s is %s", action, name, sb.ObservedState), http.StatusConflict)
		return
	}
	back := "/sandboxes/" + name
	switch action {
	case "start", "stop", "setup":
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: action, Args: []string{name}, Dir: sb.RepoPath}, back)
	case "sync":
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "services", Args: []string{"sync", name}, Dir: sb.RepoPath}, back)
	case "fork":
		target := normalizeName(r.FormValue("target"))
		if err := identity.ValidateName(target); err != nil {
			http.Error(w, "fork name: "+err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := s.store.GetSandbox(target); err == nil {
			http.Error(w, fmt.Sprintf("sandbox %q already exists", target), http.StatusConflict)
			return
		}
		// The job is attributed to the source so both rows block further actions;
		// the target does not exist until the clone lands.
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "fork", Args: []string{name, target}, Dir: sb.RepoPath}, "/sandboxes/"+target)
	case "destroy":
		if strings.TrimSpace(r.FormValue("confirm")) != name {
			http.Error(w, "type the sandbox name to confirm destroy", http.StatusBadRequest)
			return
		}
		if sb.IsTemplate() {
			s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "template", Args: []string{"rm", name, "--yes"}}, "/templates")
			return
		}
		args := []string{name, "--yes"}
		if r.FormValue("force") == "on" {
			args = append(args, "--force")
		}
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "destroy", Args: args, Dir: sb.RepoPath}, "/sandboxes")
	case "template":
		target := normalizeName(r.FormValue("target"))
		if err := identity.ValidateName(target); err != nil {
			http.Error(w, "template name: "+err.Error(), http.StatusBadRequest)
			return
		}
		if existing, err := s.store.GetSandbox(target); err == nil && !existing.IsTemplate() {
			http.Error(w, fmt.Sprintf("%q is a sandbox, not a template", target), http.StatusConflict)
			return
		}
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "template", Args: []string{"save", name, target}, Dir: sb.RepoPath}, "/templates")
	case "extend":
		spec := strings.TrimSpace(r.FormValue("for"))
		if spec == "" {
			http.Error(w, "choose a duration", http.StatusBadRequest)
			return
		}
		args := []string{name, "--for", spec}
		if spec == "never" {
			args = []string{name, "--never"}
		}
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "extend", Args: args, Dir: sb.RepoPath}, back)
	case "adopt":
		s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "contract", Args: []string{"adopt", name}, Dir: sb.RepoPath}, back)
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) handleTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := s.listSandboxDTOs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var templates []SandboxDTO
	for _, sb := range list {
		if sb.IsTemplate {
			templates = append(templates, sb)
		}
	}
	s.render(w, "templates", pageData{Title: "Templates", Active: "templates", Sandboxes: templates})
}

func (s *Server) handlePhaseLog(w http.ResponseWriter, r *http.Request) {
	path, ok := phaseLogPath(s.opts.LogsDir, r.PathValue("name"), r.PathValue("phase"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(path)
	if err != nil {
		http.Error(w, "no log for that phase yet", http.StatusNotFound)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, f)
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + strings.TrimPrefix(p, "~")
		}
	}
	return p
}

// defaultsOrBuiltIn guards tests that construct Options without Defaults.
func defaultsOrBuiltIn(d config.Defaults) config.Defaults {
	if d.CPUs == 0 {
		return config.BuiltIn().Defaults
	}
	return d
}
