package lifecycle

import (
	"context"
	"fmt"

	"github.com/Stellar-Technology-Services/kaas/internal/colima"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
)

// ColimaProvider adapts *colima.Client to the lifecycle Provider interface.
type ColimaProvider struct {
	Client *colima.Client
}

// NewColimaProvider wraps a Colima client as a Provider.
func NewColimaProvider(client *colima.Client) *ColimaProvider {
	if client == nil {
		client = colima.New()
	}
	return &ColimaProvider{Client: client}
}

func (p *ColimaProvider) Available(context.Context) error {
	return p.Client.Available()
}

func (p *ColimaProvider) List(ctx context.Context) ([]Profile, error) {
	list, err := p.Client.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(list))
	for _, c := range list {
		out = append(out, Profile{
			Name: c.Name, Status: c.Status, Arch: c.Arch,
			CPUs: c.CPUs, Memory: c.Memory, Disk: c.Disk,
		})
	}
	return out, nil
}

func (p *ColimaProvider) Status(ctx context.Context, profile string) (ProviderStatus, error) {
	st, err := p.Client.ProfileStatus(ctx, profile)
	if err != nil {
		return ProviderStatus{}, err
	}
	if st == "" {
		return ProviderStatus{Name: profile, Exists: false}, nil
	}
	return ProviderStatus{Name: profile, Status: st, Exists: true}, nil
}

func (p *ColimaProvider) Create(ctx context.Context, spec CreateSpec) (ProviderResult, error) {
	if err := p.Client.Start(ctx, colima.StartOptions{
		Name: spec.Name, CPUs: spec.CPUs, Memory: spec.Memory,
		Disk: spec.Disk, Arch: spec.Arch, SSHPort: spec.SSHPort,
		Hostname: spec.Name,
	}); err != nil {
		return ProviderResult{}, err
	}
	st, err := p.Status(ctx, spec.Name)
	if err != nil {
		return ProviderResult{Profile: spec.Name}, nil
	}
	return ProviderResult{Profile: spec.Name, Status: st}, nil
}

func (p *ColimaProvider) Start(ctx context.Context, spec StartSpec) error {
	return p.Client.Start(ctx, colima.StartOptions{
		Name: spec.Name, CPUs: spec.CPUs, Memory: spec.Memory,
		Disk: spec.Disk, Arch: spec.Arch, SSHPort: spec.SSHPort,
		Hostname: spec.Name,
	})
}

func (p *ColimaProvider) Stop(ctx context.Context, profile string) error {
	return p.Client.Stop(ctx, profile)
}

func (p *ColimaProvider) Delete(ctx context.Context, profile string, force bool) error {
	return p.Client.Delete(ctx, profile, force)
}

func (p *ColimaProvider) Clone(ctx context.Context, source, target string, sshPort int) error {
	return p.Client.Clone(ctx, source, target, sshPort)
}

func (p *ColimaProvider) SSHConfig(ctx context.Context, profile string) (identity.Endpoint, error) {
	cfg, err := p.Client.SSHConfig(ctx, profile)
	if err != nil {
		return identity.Endpoint{}, err
	}
	if cfg == nil {
		return identity.Endpoint{}, fmt.Errorf("empty ssh-config for %q", profile)
	}
	return identity.Endpoint{
		Host:         cfg.Host,
		Port:         cfg.Port,
		User:         cfg.User,
		IdentityFile: cfg.IdentityFile,
	}, nil
}

func (p *ColimaProvider) UsedPorts(ctx context.Context) ([]int, error) {
	return p.Client.UsedSSHPorts(ctx)
}
