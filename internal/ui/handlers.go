package ui

import (
	"html/template"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func (s *Server) handleHome(w http.ResponseWriter, r *http.Request) {
	ov, err := s.buildOverview()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "home", pageData{
		Title: "Overview", Active: "home", Overview: &ov,
		Sandboxes: ov.Sandboxes, Services: ov.Services,
	})
}

func (s *Server) handleSandboxes(w http.ResponseWriter, r *http.Request) {
	list, err := s.listSandboxDTOs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "sandboxes", pageData{Title: "Sandboxes", Active: "sandboxes", Sandboxes: withoutTemplates(list)})
}

// withoutTemplates hides template rows from the working-sandbox tables.
func withoutTemplates(list []SandboxDTO) []SandboxDTO {
	out := make([]SandboxDTO, 0, len(list))
	for _, sb := range list {
		if !sb.IsTemplate {
			out = append(out, sb)
		}
	}
	return out
}

func (s *Server) handleSandboxDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	sb, err := s.store.GetSandbox(name)
	if err != nil {
		// A sandbox being created by `kaas up` has a job before it has a row.
		if job, jerr := s.store.ActiveJob(name); jerr == nil {
			dto := toJobDTO(*job, 30)
			s.render(w, "sandbox_pending", pageData{Title: name, Active: "sandboxes", Job: &dto, Filter: name})
			return
		}
		if jobs, jerr := s.listJobDTOs(name, 5); jerr == nil && len(jobs) > 0 {
			// Create failed (or sandbox destroyed): show what happened instead of a bare 404.
			last := toJobDTO(*mustJob(s, jobs[0].ID), 40)
			s.render(w, "sandbox_pending", pageData{Title: name, Active: "sandboxes", Job: &last, Jobs: jobs, Filter: name})
			return
		}
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data, err := s.sandboxPageData(*sb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "sandbox_detail", data)
}

// handlePartialSandboxStatus refreshes the header (readiness + buttons) while
// a job runs, without reloading the whole page.
func (s *Server) handlePartialSandboxStatus(w http.ResponseWriter, r *http.Request) {
	sb, err := s.store.GetSandbox(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	data, err := s.sandboxPageData(*sb)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderPartial(w, "partial_sandbox_status", data)
}

func (s *Server) sandboxPageData(sb state.Sandbox) (pageData, error) {
	name := sb.Name
	services, err := s.listServiceDTOs(name)
	if err != nil {
		return pageData{}, err
	}
	rawEvents, err := s.store.ListEvents(name, 40)
	if err != nil {
		return pageData{}, err
	}
	events := toEventDTOs(rawEvents)
	contractHash := ""
	var env *EnvironmentDTO
	if snap, err := s.store.GetContract(name); err == nil && snap != nil {
		contractHash = snap.ContractHash
		env = environmentFor(snap, s.opts.StatePath, sb.RepoPath)
	}
	jobs, err := s.listJobDTOs(name, 10)
	if err != nil {
		return pageData{}, err
	}
	var activeJob *state.Job
	var active *JobDTO
	if job, err := s.store.ActiveJob(name); err == nil {
		activeJob = job
		j := toJobDTO(*job, 30)
		active = &j
	}
	dto := toSandboxDTO(sb, rawEvents, activeJob)
	units := make(map[string]string, len(services))
	for i := range services {
		services[i].Health = serviceHealth(sb, services[i])
		if services[i].Health == "down" {
			units[services[i].Service] = systemdUnit(sb, services[i])
		}
	}
	return pageData{
		Title:        name,
		Active:       "sandboxes",
		Sandbox:      &dto,
		Services:     services,
		Events:       events,
		ContractHash: contractHash,
		Jobs:         jobs,
		Job:          active,
		Connect:      connectCommands(sb),
		HerdrOnPath:  lookPath("herdr") == nil,
		Environment:  env,
		PhaseLogs:    phaseLogsFor(s.opts.LogsDir, name),
		Templates:    s.templateDTOs(),
		Commands:     s.commandDTOs(nil, sb, false),
		Harnesses:    harnessOrder,
		Units:        units,
	}, nil
}

// serviceHealth probes the host port only for running sandboxes.
func serviceHealth(sb state.Sandbox, svc ServiceDTO) string {
	if sb.ObservedState != state.ObservedRunning {
		return ""
	}
	if probeHostPort(svc.HostPort) {
		return "listening"
	}
	return "down"
}

func mustJob(s *Server, id string) *state.Job {
	job, err := s.store.GetJob(id)
	if err != nil {
		return &state.Job{ID: id}
	}
	return job
}

func (s *Server) handleSandboxSync(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	s.startJobAndRedirect(w, r, startRequest{Sandbox: name, Verb: "services", Args: []string{"sync", name}}, "/sandboxes/"+name)
}

// startJobAndRedirect spawns a background CLI job and sends the browser back
// to the page that will poll it. A refusal (another job active) is reported
// as 409 so the user sees why nothing happened.
func (s *Server) startJobAndRedirect(w http.ResponseWriter, r *http.Request, req startRequest, back string) {
	job, err := s.jobs.Start(req)
	if err != nil {
		status := http.StatusInternalServerError
		if strings.Contains(err.Error(), "already running") {
			status = http.StatusConflict
		}
		http.Error(w, err.Error(), status)
		return
	}
	if back == "" {
		back = "/jobs/" + job.ID
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("sandbox")
	list, err := s.listJobDTOs(filter, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "jobs", pageData{Title: "Jobs", Active: "jobs", Jobs: list, Filter: filter})
}

func (s *Server) handleJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dto := toJobDTO(*job, 200)
	s.render(w, "job", pageData{Title: "Job " + job.Verb, Active: "jobs", Job: &dto})
}

func (s *Server) handleJobLog(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	if job.LogPath == "" {
		_, _ = w.Write([]byte("(no log)\n"))
		return
	}
	f, err := os.Open(job.LogPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	defer f.Close()
	_, _ = io.Copy(w, f)
}

func (s *Server) handlePartialJob(w http.ResponseWriter, r *http.Request) {
	job, err := s.store.GetJob(r.PathValue("id"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dto := toJobDTO(*job, 30)
	s.renderPartial(w, "partial_job", pageData{Job: &dto})
}

func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	list, err := s.listServiceDTOs("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "services", pageData{Title: "Services", Active: "services", Services: list})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("sandbox")
	list, err := s.listEventDTOs(filter, 100)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.render(w, "events", pageData{Title: "Events", Active: "events", Events: list, Filter: filter})
}

func (s *Server) handlePartialOverview(w http.ResponseWriter, r *http.Request) {
	ov, err := s.buildOverview()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderPartial(w, "partial_overview", pageData{
		Overview: &ov, Sandboxes: ov.Sandboxes, Services: ov.Services,
	})
}

func (s *Server) handlePartialSandboxes(w http.ResponseWriter, r *http.Request) {
	list, err := s.listSandboxDTOs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderPartial(w, "partial_sandboxes", pageData{Sandboxes: withoutTemplates(list)})
}

func (s *Server) handlePartialServices(w http.ResponseWriter, r *http.Request) {
	list, err := s.listServiceDTOs("")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.renderPartial(w, "partial_services", pageData{Services: list})
}

type pageData struct {
	Title        string
	Active       string
	Overview     *OverviewDTO
	Sandboxes    []SandboxDTO
	Sandbox      *SandboxDTO
	Services     []ServiceDTO
	Events       []EventDTO
	Jobs         []JobDTO
	Job          *JobDTO
	Form         *createForm
	Connect      []string
	HerdrOnPath  bool
	Environment  *EnvironmentDTO
	PhaseLogs    []PhaseLogDTO
	Templates    []SandboxDTO
	Commands     []CommandDTO
	Processes    []ProcessDTO
	Harnesses    []string
	Units        map[string]string // service name → systemd unit snippet
	ContractHash string
	Filter       string
	Error        string
	Now          time.Time
}

func (s *Server) render(w http.ResponseWriter, name string, data pageData) {
	data.Now = time.Now().UTC()
	tmpl, err := loadTemplates()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) renderPartial(w http.ResponseWriter, name string, data pageData) {
	data.Now = time.Now().UTC()
	tmpl, err := loadTemplates()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func loadTemplates() (*template.Template, error) {
	funcs := template.FuncMap{
		"fmtTime": func(t time.Time) string {
			if t.IsZero() {
				return "—"
			}
			return t.Local().Format("2006-01-02 15:04:05")
		},
		"emptyDash": func(s string) string {
			if strings.TrimSpace(s) == "" {
				return "—"
			}
			return s
		},
		"has": func(list []string, item string) bool { return actionAllowed(list, item) },
		"kib": func(n int64) string {
			if n < 1024 {
				return strconv.FormatInt(n, 10) + " B"
			}
			return strconv.FormatInt(n/1024, 10) + " KiB"
		},
	}
	return template.New("ui").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
}

func lookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}
