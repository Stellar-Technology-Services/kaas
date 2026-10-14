package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/colima"
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"github.com/Stellar-Technology-Services/kaas/internal/ingress"
	"github.com/Stellar-Technology-Services/kaas/internal/ingress/dnsconfig"
	iruntime "github.com/Stellar-Technology-Services/kaas/internal/ingress/runtime"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// SandboxDTO is a UI-facing sandbox row.
type SandboxDTO struct {
	Name          string
	DesiredState  string
	ObservedState string
	SSHPort       int
	CPUs          int
	MemoryGiB     int
	DiskGiB       int
	SetupStatus   string
	EgressMode    string
	EgressStatus  string
	Workspace     string
	WorkspaceDir  string
	RepoPath      string
	UpdatedAt     time.Time
	Readiness     Readiness
	Actions       []string
	Credentials   Credentials
	IsTemplate    bool
	AutoStopTTL   string // "4h", "never"
	AutoStopAt    time.Time
	AutoStopIn    string // humanized remaining time, "" when not armed
}

// ServiceDTO is a UI-facing published service.
type ServiceDTO struct {
	Sandbox   string
	Service   string
	Hostname  string
	URL       string
	GuestPort int
	HostPort  int
	Scheme    string
	Status    string
	Health    string // listening / down / "" (not probed)
}

// EventDTO is a UI-facing lifecycle event.
type EventDTO struct {
	Sandbox   string
	Action    string
	Status    string
	Detail    string
	CreatedAt time.Time
}

// JobDTO is a UI-facing background job.
type JobDTO struct {
	ID         string
	Sandbox    string
	Verb       string
	Command    string // "kaas <verb> <args>" for display
	Status     string
	Active     bool
	Phase      string
	Summary    string
	ExitCode   int
	LogTail    []string
	CreatedAt  time.Time
	FinishedAt time.Time
}

func toJobDTO(job state.Job, tailLines int) JobDTO {
	dto := JobDTO{
		ID:        job.ID,
		Sandbox:   job.Sandbox,
		Verb:      job.Verb,
		Command:   "kaas " + job.Verb + " " + strings.Join(job.Args, " "),
		Status:    job.Status,
		Active:    job.Active(),
		Phase:     job.Phase,
		Summary:   job.Summary,
		CreatedAt: job.CreatedAt,
	}
	if job.ExitCode != nil {
		dto.ExitCode = *job.ExitCode
	}
	if job.FinishedAt != nil {
		dto.FinishedAt = *job.FinishedAt
	}
	if tailLines > 0 && job.LogPath != "" {
		if lines, err := tailFile(job.LogPath, tailLines); err == nil {
			dto.LogTail = lines
		}
	}
	return dto
}

func (s *Server) listJobDTOs(sandbox string, limit int) ([]JobDTO, error) {
	list, err := s.store.ListJobs(sandbox, limit)
	if err != nil {
		return nil, err
	}
	out := make([]JobDTO, 0, len(list))
	for _, job := range list {
		out = append(out, toJobDTO(job, 0))
	}
	return out, nil
}

// OverviewDTO powers the home dashboard.
type OverviewDTO struct {
	SandboxCount   int
	RunningCount   int
	ServiceCount   int
	IngressRunning bool
	IngressListen  string
	IngressRoutes  int
	DNSEnabled     bool
	DNSDetail      string
	Domain         string
	CatalogURL     string
	Listen         string
	Tools          []ToolStatus
	Capacity       HostCapacity
	ActiveJobs     int
	TemplateCount  int
	Sandboxes      []SandboxDTO
	Services       []ServiceDTO
}

// ToolStatus is a host prerequisite check.
type ToolStatus struct {
	Name   string
	OK     bool
	Detail string
}

type storeAdapter struct {
	*state.Store
}

func openStore(path string) (*storeAdapter, error) {
	st, err := state.Open(path)
	if err != nil {
		return nil, err
	}
	return &storeAdapter{Store: st}, nil
}

func (s *storeAdapter) Close() error { return s.Store.Close() }

func toSandboxDTO(sb state.Sandbox, events []state.Event, active *state.Job) SandboxDTO {
	dto := SandboxDTO{
		Name:          sb.Name,
		DesiredState:  sb.DesiredState,
		ObservedState: sb.ObservedState,
		SSHPort:       sb.SSHPort,
		CPUs:          sb.CPUs,
		MemoryGiB:     sb.MemoryGiB,
		DiskGiB:       sb.DiskGiB,
		SetupStatus:   sb.SetupStatus,
		EgressMode:    sb.EgressMode,
		EgressStatus:  sb.EgressStatus,
		Workspace:     sb.WorkspaceRepository,
		WorkspaceDir:  sb.WorkspaceDirectory,
		RepoPath:      sb.RepoPath,
		UpdatedAt:     sb.UpdatedAt,
		Readiness:     readinessFor(sb, events, active),
		Actions:       allowedActions(sb, active),
		Credentials:   credentialsFor(sb, events),
		IsTemplate:    sb.IsTemplate(),
		AutoStopTTL:   config.FormatAutoStop(time.Duration(sb.AutoStopTTL) * time.Second),
	}
	if sb.AutoStopAt != nil {
		dto.AutoStopAt = *sb.AutoStopAt
		dto.AutoStopIn = humanDuration(time.Until(*sb.AutoStopAt))
	}
	return dto
}

// humanDuration renders "2h 05m" / "12m" / "overdue".
func humanDuration(d time.Duration) string {
	if d <= 0 {
		return "overdue"
	}
	d = d.Round(time.Minute)
	h := int(d / time.Hour)
	m := int(d % time.Hour / time.Minute)
	switch {
	case h >= 48:
		return fmt.Sprintf("%dd %dh", h/24, h%24)
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	default:
		return fmt.Sprintf("%dm", m)
	}
}

func (s *Server) listSandboxDTOs() ([]SandboxDTO, error) {
	list, err := s.store.ListSandboxes("")
	if err != nil {
		return nil, err
	}
	activeJobs, _ := s.store.ActiveJobs()
	activeBySandbox := make(map[string]*state.Job, len(activeJobs))
	for i := range activeJobs {
		activeBySandbox[activeJobs[i].Sandbox] = &activeJobs[i]
	}
	out := make([]SandboxDTO, 0, len(list))
	for _, sb := range list {
		events, _ := s.store.ListEvents(sb.Name, 20)
		out = append(out, toSandboxDTO(sb, events, activeBySandbox[sb.Name]))
	}
	return out, nil
}

func (s *Server) listServiceDTOs(sandbox string) ([]ServiceDTO, error) {
	list, err := s.store.ListServices(sandbox)
	if err != nil {
		return nil, err
	}
	dnsReady, _, _ := dnsconfig.Status(s.opts.IngressRuntime.Domain)
	out := make([]ServiceDTO, 0, len(list))
	for _, svc := range list {
		out = append(out, ServiceDTO{
			Sandbox:   svc.Sandbox,
			Service:   svc.Service,
			Hostname:  svc.Hostname,
			URL:       ingress.URLFor(svc.Scheme, svc.Hostname, s.opts.IngressRuntime.Listen, dnsReady),
			GuestPort: svc.GuestPort,
			HostPort:  svc.HostPort,
			Scheme:    svc.Scheme,
			Status:    svc.Status,
		})
	}
	return out, nil
}

func (s *Server) listEventDTOs(sandbox string, limit int) ([]EventDTO, error) {
	list, err := s.store.ListEvents(sandbox, limit)
	if err != nil {
		return nil, err
	}
	return toEventDTOs(list), nil
}

func toEventDTOs(list []state.Event) []EventDTO {
	out := make([]EventDTO, 0, len(list))
	for _, ev := range list {
		out = append(out, EventDTO{
			Sandbox:   ev.Sandbox,
			Action:    ev.Action,
			Status:    ev.Status,
			Detail:    ev.Detail,
			CreatedAt: ev.CreatedAt,
		})
	}
	return out
}

func (s *Server) buildOverview() (OverviewDTO, error) {
	sandboxes, err := s.listSandboxDTOs()
	if err != nil {
		return OverviewDTO{}, err
	}
	services, err := s.listServiceDTOs("")
	if err != nil {
		return OverviewDTO{}, err
	}
	running := 0
	for _, sb := range sandboxes {
		if sb.IsTemplate {
			continue
		}
		if sb.ObservedState == state.ObservedRunning || sb.ObservedState == "running" {
			running++
		}
	}
	// Overview and list pages show working sandboxes; templates have their own page.
	working := make([]SandboxDTO, 0, len(sandboxes))
	templates := 0
	for _, sb := range sandboxes {
		if sb.IsTemplate {
			templates++
			continue
		}
		working = append(working, sb)
	}
	sandboxes = working
	raw, _ := s.store.ListSandboxes("")
	activeJobs, _ := s.store.ActiveJobs()
	dnsReady, dnsDetail, _ := dnsconfig.Status(s.opts.IngressRuntime.Domain)
	st, _ := iruntime.Call(s.opts.IngressRuntime.Root, iruntime.Request{Action: "status"})
	ov := OverviewDTO{
		SandboxCount:   len(sandboxes),
		RunningCount:   running,
		ServiceCount:   len(services),
		IngressRunning: st.Running,
		IngressListen:  st.Listen,
		IngressRoutes:  st.Routes,
		DNSEnabled:     dnsReady,
		DNSDetail:      dnsDetail,
		Domain:         s.opts.IngressRuntime.Domain,
		CatalogURL:     ingress.URLFor("http", ingress.CatalogHostname(s.opts.IngressRuntime.Domain), s.opts.IngressRuntime.Listen, dnsReady),
		Listen:         s.opts.Listen,
		Tools:          hostToolStatus(),
		Capacity:       hostCapacity(raw),
		ActiveJobs:     len(activeJobs),
		TemplateCount:  templates,
		Sandboxes:      sandboxes,
		Services:       services,
	}
	if ov.IngressListen == "" {
		ov.IngressListen = s.opts.IngressRuntime.Listen
	}
	return ov, nil
}

func hostToolStatus() []ToolStatus {
	tools := []string{"colima", "limactl", "ansible", "ansible-playbook", "secretspec"}
	out := make([]ToolStatus, 0, len(tools)+1)
	client := colima.New()
	if err := client.Available(); err != nil {
		out = append(out, ToolStatus{Name: "colima", OK: false, Detail: err.Error()})
	} else {
		out = append(out, ToolStatus{Name: "colima", OK: true})
	}
	for _, name := range tools[1:] {
		ok := lookPath(name) == nil
		detail := ""
		if !ok {
			detail = fmt.Sprintf("%s not found on PATH", name)
		}
		out = append(out, ToolStatus{Name: name, OK: ok, Detail: detail})
	}
	return out
}
