package ui

import (
	"fmt"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/colima"
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
	RepoPath      string
	UpdatedAt     time.Time
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
}

// EventDTO is a UI-facing lifecycle event.
type EventDTO struct {
	Sandbox   string
	Action    string
	Status    string
	Detail    string
	CreatedAt time.Time
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

func toSandboxDTO(sb state.Sandbox) SandboxDTO {
	return SandboxDTO{
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
		RepoPath:      sb.RepoPath,
		UpdatedAt:     sb.UpdatedAt,
	}
}

func (s *Server) listSandboxDTOs() ([]SandboxDTO, error) {
	list, err := s.store.ListSandboxes("")
	if err != nil {
		return nil, err
	}
	out := make([]SandboxDTO, 0, len(list))
	for _, sb := range list {
		out = append(out, toSandboxDTO(sb))
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
	return out, nil
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
		if sb.ObservedState == state.ObservedRunning || sb.ObservedState == "running" {
			running++
		}
	}
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
