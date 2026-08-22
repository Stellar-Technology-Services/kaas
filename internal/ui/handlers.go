package ui

import (
	"fmt"
	"html/template"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
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
	s.render(w, "sandboxes", pageData{Title: "Sandboxes", Active: "sandboxes", Sandboxes: list})
}

func (s *Server) handleSandboxDetail(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	sb, err := s.store.GetSandbox(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	dto := toSandboxDTO(*sb)
	services, err := s.listServiceDTOs(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	events, err := s.listEventDTOs(name, 40)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	contractHash := ""
	if snap, err := s.store.GetContract(name); err == nil && snap != nil {
		contractHash = snap.ContractHash
	}
	s.render(w, "sandbox_detail", pageData{
		Title:        name,
		Active:       "sandboxes",
		Sandbox:      &dto,
		Services:     services,
		Events:       events,
		ContractHash: contractHash,
	})
}

func (s *Server) handleSandboxSync(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	exe := s.opts.Executable
	if exe == "" {
		var err error
		exe, err = os.Executable()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	cmd := exec.Command(exe, "services", "sync", name)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		http.Error(w, fmt.Sprintf("sync failed: %v\n%s", err, string(out)), http.StatusBadGateway)
		return
	}
	http.Redirect(w, r, "/sandboxes/"+name, http.StatusSeeOther)
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
	s.renderPartial(w, "partial_sandboxes", pageData{Sandboxes: list})
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
	ContractHash string
	Filter       string
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
	}
	return template.New("ui").Funcs(funcs).ParseFS(templateFS, "templates/*.html")
}

func lookPath(name string) error {
	_, err := exec.LookPath(name)
	return err
}
