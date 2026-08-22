package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
)

// FakeProvider is an in-memory Provider for lifecycle tests.
type FakeProvider struct {
	mu       sync.Mutex
	profiles map[string]*fakeProfile

	// Failures inject errors for named methods (e.g. "Create", "Delete", "Clone", "Start").
	Failures map[string]error

	// FailOnce injects a one-shot error then clears itself.
	FailOnce map[string]error

	AvailableErr error
}

type fakeProfile struct {
	Name    string
	Status  string
	CPUs    int
	Memory  int
	Disk    int
	Arch    string
	SSHPort int
	Host    string
	User    string
	Key     string
}

// NewFakeProvider returns an empty FakeProvider.
func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		profiles: make(map[string]*fakeProfile),
		Failures: make(map[string]error),
		FailOnce: make(map[string]error),
	}
}

func (f *FakeProvider) fail(method string) error {
	if f.FailOnce != nil {
		if err, ok := f.FailOnce[method]; ok {
			delete(f.FailOnce, method)
			return err
		}
	}
	if f.Failures != nil {
		return f.Failures[method]
	}
	return nil
}

func (f *FakeProvider) Available(context.Context) error {
	if f.AvailableErr != nil {
		return f.AvailableErr
	}
	return f.fail("Available")
}

func (f *FakeProvider) List(context.Context) ([]Profile, error) {
	if err := f.fail("List"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Profile, 0, len(f.profiles))
	for _, p := range f.profiles {
		out = append(out, Profile{
			Name: p.Name, Status: p.Status, Arch: p.Arch,
			CPUs: p.CPUs, Memory: p.Memory, Disk: p.Disk,
		})
	}
	return out, nil
}

func (f *FakeProvider) Status(_ context.Context, profile string) (ProviderStatus, error) {
	if err := f.fail("Status"); err != nil {
		return ProviderStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[profile]
	if !ok {
		return ProviderStatus{Name: profile, Exists: false}, nil
	}
	return ProviderStatus{Name: profile, Status: p.Status, Exists: true}, nil
}

func (f *FakeProvider) Create(_ context.Context, spec CreateSpec) (ProviderResult, error) {
	if err := f.fail("Create"); err != nil {
		return ProviderResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.profiles[spec.Name]; ok {
		return ProviderResult{}, fmt.Errorf("profile %q already exists", spec.Name)
	}
	p := &fakeProfile{
		Name:    spec.Name,
		Status:  "Running",
		CPUs:    spec.CPUs,
		Memory:  spec.Memory,
		Disk:    spec.Disk,
		Arch:    spec.Arch,
		SSHPort: spec.SSHPort,
		Host:    identity.DefaultSSHHost,
		User:    "user",
		Key:     "/tmp/" + spec.Name + ".pem",
	}
	f.profiles[spec.Name] = p
	return ProviderResult{
		Profile: spec.Name,
		Status:  ProviderStatus{Name: spec.Name, Status: p.Status, Exists: true},
	}, nil
}

func (f *FakeProvider) Start(_ context.Context, spec StartSpec) error {
	if err := f.fail("Start"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[spec.Name]
	if !ok {
		return fmt.Errorf("profile %q not found", spec.Name)
	}
	if spec.SSHPort > 0 {
		p.SSHPort = spec.SSHPort
	}
	if spec.CPUs > 0 {
		p.CPUs = spec.CPUs
	}
	if spec.Memory > 0 {
		p.Memory = spec.Memory
	}
	if spec.Disk > 0 {
		p.Disk = spec.Disk
	}
	if spec.Arch != "" {
		p.Arch = spec.Arch
	}
	p.Status = "Running"
	return nil
}

func (f *FakeProvider) Stop(_ context.Context, profile string) error {
	if err := f.fail("Stop"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[profile]
	if !ok {
		return fmt.Errorf("profile %q not found", profile)
	}
	p.Status = "Stopped"
	return nil
}

func (f *FakeProvider) Delete(_ context.Context, profile string, _ bool) error {
	if err := f.fail("Delete"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.profiles[profile]; !ok {
		return fmt.Errorf("profile %q not found", profile)
	}
	delete(f.profiles, profile)
	return nil
}

func (f *FakeProvider) Clone(_ context.Context, source, target string, sshPort int) error {
	if err := f.fail("Clone"); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	src, ok := f.profiles[source]
	if !ok {
		return fmt.Errorf("source profile %q not found", source)
	}
	if _, exists := f.profiles[target]; exists {
		return fmt.Errorf("target profile %q already exists", target)
	}
	if !strings.EqualFold(src.Status, "Stopped") {
		return fmt.Errorf("source profile %q must be stopped to clone", source)
	}
	f.profiles[target] = &fakeProfile{
		Name:    target,
		Status:  "Running",
		CPUs:    src.CPUs,
		Memory:  src.Memory,
		Disk:    src.Disk,
		Arch:    src.Arch,
		SSHPort: sshPort,
		Host:    identity.DefaultSSHHost,
		User:    src.User,
		Key:     "/tmp/" + target + ".pem",
	}
	return nil
}

func (f *FakeProvider) SSHConfig(_ context.Context, profile string) (identity.Endpoint, error) {
	if err := f.fail("SSHConfig"); err != nil {
		return identity.Endpoint{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	p, ok := f.profiles[profile]
	if !ok {
		return identity.Endpoint{}, fmt.Errorf("profile %q not found", profile)
	}
	return identity.Endpoint{
		Host:         p.Host,
		Port:         p.SSHPort,
		User:         p.User,
		IdentityFile: p.Key,
	}, nil
}

func (f *FakeProvider) UsedPorts(context.Context) ([]int, error) {
	if err := f.fail("UsedPorts"); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	ports := make([]int, 0, len(f.profiles))
	for _, p := range f.profiles {
		if p.SSHPort > 0 {
			ports = append(ports, p.SSHPort)
		}
	}
	return ports, nil
}

// Seed adds or replaces a profile for test setup.
func (f *FakeProvider) Seed(name, status string, sshPort int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.profiles[name] = &fakeProfile{
		Name:    name,
		Status:  status,
		SSHPort: sshPort,
		Host:    identity.DefaultSSHHost,
		User:    "user",
		Key:     "/tmp/" + name + ".pem",
		CPUs:    2,
		Memory:  4,
		Disk:    40,
		Arch:    "aarch64",
	}
}
