package contract

import (
	"sort"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

// LayerFromConfig maps a config fragment into a resolution layer.
// Secret values and egress runtime (CA, listen addresses) are never included.
func LayerFromConfig(cfg config.Config, source string) Layer {
	return Layer{
		Resources: resourcesPtr(ResourcesFromConfig(cfg)),
		Workspace: workspacePtr(WorkspaceFromConfig(cfg)),
		Secrets:   secretsPtr(SecretsFromConfig(cfg)),
		Egress:    egressPtr(EgressFromConfig(cfg)),
		Source:    source,
	}
}

// FromConfig builds a full Contract from a merged Config (no secret values).
func FromConfig(cfg config.Config) Contract {
	c := Contract{
		SchemaVersion: CurrentSchemaVersion,
		Resources:     ResourcesFromConfig(cfg),
		Workspace:     WorkspaceFromConfig(cfg),
		Secrets:       SecretsFromConfig(cfg),
		Egress:        EgressFromConfig(cfg),
		Provenance: Provenance{
			ConfigSource: firstNonEmpty(cfg.ConfigPath, cfg.UserConfig),
		},
	}
	c.Normalize()
	return c
}

// ResourcesFromConfig maps VM defaults (and optional port base policy).
func ResourcesFromConfig(cfg config.Config) Resources {
	r := Resources{
		CPUs:          cfg.Defaults.CPUs,
		MemoryGiB:     cfg.Defaults.MemoryGiB,
		DiskGiB:       cfg.Defaults.DiskGiB,
		Arch:          cfg.Defaults.Arch,
		SSHPortPolicy: SSHPortPolicyNextFree,
	}
	return r
}

// WorkspaceFromConfig maps workspace intent without credentials.
func WorkspaceFromConfig(cfg config.Config) WorkspaceContract {
	gitAuth := cfg.Workspace.GitAuth
	if gitAuth == "" {
		gitAuth = "agent"
	}
	sources := make([]WorkspaceSourceContract, 0, len(cfg.Workspace.Sources))
	for _, src := range cfg.Workspace.Sources {
		sources = append(sources, WorkspaceSourceContract{
			Repository: src.Repository,
			Branch:     src.Branch,
			Directory:  src.Directory,
		})
	}
	w := WorkspaceContract{
		GitAuth: gitAuth,
		Varlock: VarlockContract{
			Schema:          cfg.Workspace.Varlock.Schema,
			ValidateCommand: cfg.Workspace.Varlock.ValidateCommand,
		},
		Setup:   SetupContract{Script: DefaultSetupScript},
		Sources: sources,
	}
	if len(cfg.Workspace.Services) > 0 {
		names := make([]string, 0, len(cfg.Workspace.Services))
		for name := range cfg.Workspace.Services {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			svc := cfg.Workspace.Services[name]
			scheme := svc.Scheme
			if scheme == "" {
				scheme = "http"
			}
			w.Services = append(w.Services, ServiceContract{
				Name:   name,
				Port:   svc.Port,
				Scheme: scheme,
			})
		}
	}
	w.normalizeSources()
	w.normalizeServices()
	return w
}

// SecretsFromConfig derives secret mode and key references from egress policy.
// Mode is delegated when egress uses a proxy mode; otherwise open.
// Keys are credential key identifiers only.
func SecretsFromConfig(cfg config.Config) SecretContract {
	mode := SecretModeOpen
	switch cfg.Egress.Mode {
	case "proxy_preferred", "proxy_only":
		mode = SecretModeDelegated
	}
	var keys []SecretReference
	seen := map[string]bool{}
	for _, rule := range cfg.Egress.Rules {
		if rule.InjectSecret == nil {
			continue
		}
		k := strings.TrimSpace(rule.InjectSecret.Key)
		if k == "" || seen[k] {
			continue
		}
		seen[k] = true
		keys = append(keys, SecretReference{Key: k})
	}
	return SecretContract{Mode: mode, Keys: keys}
}

// EgressFromConfig maps policy identifiers and credential key refs only.
// Host-owned EgressRuntime (listen host, CA material) is intentionally omitted.
func EgressFromConfig(cfg config.Config) EgressContract {
	if cfg.Egress.SchemaVersion == 0 && cfg.Egress.Mode == "" && cfg.Egress.Default == "" && len(cfg.Egress.Rules) == 0 {
		return EgressContract{}
	}
	out := EgressContract{
		SchemaVersion: cfg.Egress.SchemaVersion,
		Mode:          cfg.Egress.Mode,
		Default:       cfg.Egress.Default,
	}
	for _, rule := range cfg.Egress.Rules {
		hr := EgressHostRule{Host: rule.Host}
		if rule.InjectSecret != nil {
			hr.CredentialKey = rule.InjectSecret.Key
			hr.Injection = rule.InjectSecret.Type
			hr.Header = rule.InjectSecret.Header
		}
		out.Rules = append(out.Rules, hr)
	}
	return out
}

// FlagsLayer builds a layer from CLI resource overrides (0 / "" = unset).
func FlagsLayer(cpus, memory, disk, port int, arch, source string) Layer {
	r := Resources{}
	set := false
	if cpus > 0 {
		r.CPUs = cpus
		set = true
	}
	if memory > 0 {
		r.MemoryGiB = memory
		set = true
	}
	if disk > 0 {
		r.DiskGiB = disk
		set = true
	}
	if arch != "" {
		r.Arch = arch
		set = true
	}
	if port > 0 {
		r.SSHPort = port
		r.SSHPortPolicy = ""
		set = true
	}
	if !set {
		return Layer{Source: source}
	}
	return Layer{Resources: &r, Source: source}
}

func resourcesPtr(r Resources) *Resources { return &r }
func workspacePtr(w WorkspaceContract) *WorkspaceContract {
	return &w
}
func secretsPtr(s SecretContract) *SecretContract { return &s }
func egressPtr(e EgressContract) *EgressContract {
	if e.SchemaVersion == 0 && e.Mode == "" && e.Default == "" && len(e.Rules) == 0 {
		return nil
	}
	return &e
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
