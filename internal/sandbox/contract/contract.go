// Package contract owns sandbox contract types, deterministic hashing, and
// resolution from configuration layers. Contracts describe desired behavior and
// never contain credential values or private egress runtime material.
package contract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Schema versions for persisted contracts.
const (
	// CurrentSchemaVersion is the schema written by Resolve / Put.
	CurrentSchemaVersion ContractVersion = 1

	// DefaultSetupScript is the repository-owned bootstrap entrypoint.
	DefaultSetupScript = "scripts/kaas-setup"
	// SSHPortPolicyNextFree allocates the next free port from ssh_port_base.
	SSHPortPolicyNextFree = "next_free_from_base"

	// SecretModeOpen deploys secrets into the guest (traditional path).
	SecretModeOpen = "open"
	// SecretModeDelegated keeps values on the host egress proxy.
	SecretModeDelegated = "delegated"
)

// ContractVersion is the persisted contract schema version.
type ContractVersion int

// Contract is the secret-free desired behavior of a sandbox.
type Contract struct {
	SchemaVersion ContractVersion   `json:"schema_version"`
	Resources     Resources         `json:"resources"`
	Workspace     WorkspaceContract `json:"workspace"`
	Secrets       SecretContract    `json:"secrets"`
	Egress        EgressContract    `json:"egress"`
	Provenance    Provenance        `json:"provenance"`
	// ContentHash is the stable digest of the redacted contract body.
	ContentHash string `json:"hash,omitempty"`
}

// Resources are desired VM resource requirements and SSH port intent.
// Mirrors identity.Resources plus port allocation policy for the resource contract.
type Resources struct {
	CPUs      int    `json:"cpus"`
	MemoryGiB int    `json:"memory_gib"`
	DiskGiB   int    `json:"disk_gib"`
	Arch      string `json:"arch"`
	// SSHPort is an explicit requested port. Zero means use SSHPortPolicy.
	SSHPort int `json:"ssh_port,omitempty"`
	// SSHPortPolicy is used when SSHPort is zero (default: next_free_from_base).
	SSHPortPolicy string `json:"ssh_port_policy,omitempty"`
}

// WorkspaceContract is guest workspace intent (no credentials).
// Sources holds every managed checkout; the primary is Sources[0].
type WorkspaceContract struct {
	GitAuth  string                    `json:"git_auth,omitempty"`
	Varlock  VarlockContract           `json:"varlock"`
	Setup    SetupContract             `json:"setup"`
	Sources  []WorkspaceSourceContract `json:"sources,omitempty"`
	Services []ServiceContract         `json:"services,omitempty"`
}

// ServiceContract declares a named guest listener to publish on the host.
type ServiceContract struct {
	Name   string `json:"name"`
	Port   int    `json:"port"`
	Scheme string `json:"scheme,omitempty"`
}

// WorkspaceSourceContract is one managed git checkout inside the guest.
type WorkspaceSourceContract struct {
	Repository string `json:"repository,omitempty"`
	Branch     string `json:"branch,omitempty"`
	Directory  string `json:"directory,omitempty"`
}

// Primary returns Sources[0] when present.
func (w WorkspaceContract) Primary() (WorkspaceSourceContract, bool) {
	if len(w.Sources) == 0 {
		return WorkspaceSourceContract{}, false
	}
	return w.Sources[0], true
}

// VarlockContract declares an optional environment schema/validation command.
type VarlockContract struct {
	Schema          string `json:"schema,omitempty"`
	ValidateCommand string `json:"validate_command,omitempty"`
}

// SetupContract names the repository setup script to run after checkout.
type SetupContract struct {
	Script string `json:"script,omitempty"`
}

// SecretContract holds secret mode and key references only — never values.
type SecretContract struct {
	Mode string            `json:"mode,omitempty"`
	Keys []SecretReference `json:"keys,omitempty"`
}

// SecretReference identifies a SecretSpec key by name.
type SecretReference struct {
	Key string `json:"key"`
}

// EgressContract is a redacted egress policy: hosts and credential key refs only.
type EgressContract struct {
	SchemaVersion int              `json:"schema_version,omitempty"`
	Mode          string           `json:"mode,omitempty"`
	Default       string           `json:"default,omitempty"`
	Rules         []EgressHostRule `json:"rules,omitempty"`
	// PolicyHash is a redacted hash of the policy body (no values).
	PolicyHash string `json:"policy_hash,omitempty"`
}

// EgressHostRule allows a host and optionally names a credential key to inject.
type EgressHostRule struct {
	Host          string `json:"host"`
	CredentialKey string `json:"credential_key,omitempty"`
	Injection     string `json:"injection,omitempty"`
	Header        string `json:"header,omitempty"`
}

// Provenance records which config layers contributed to the contract.
type Provenance struct {
	Sources      []string `json:"sources,omitempty"`
	ConfigSource string   `json:"config_source,omitempty"`
}

// Normalize sorts slices for stable hashing and JSON and fills defaults.
func (c *Contract) Normalize() {
	if c == nil {
		return
	}
	if c.SchemaVersion == 0 {
		c.SchemaVersion = CurrentSchemaVersion
	}
	if c.Resources.SSHPort == 0 && c.Resources.SSHPortPolicy == "" {
		c.Resources.SSHPortPolicy = SSHPortPolicyNextFree
	}
	if c.Workspace.Setup.Script == "" {
		c.Workspace.Setup.Script = DefaultSetupScript
	}
	if c.Workspace.GitAuth == "" {
		c.Workspace.GitAuth = "agent"
	}
	c.Workspace.normalizeSources()
	c.Workspace.normalizeServices()
	sort.SliceStable(c.Secrets.Keys, func(i, j int) bool {
		return c.Secrets.Keys[i].Key < c.Secrets.Keys[j].Key
	})
	sort.SliceStable(c.Egress.Rules, func(i, j int) bool {
		return c.Egress.Rules[i].Host < c.Egress.Rules[j].Host
	})
	if len(c.Provenance.Sources) > 1 {
		// Keep insertion order of unique sources; do not alphabetize —
		// order reflects authority stack.
		c.Provenance.Sources = uniquePreserve(c.Provenance.Sources)
	}
}

// normalizeSources trims whitespace on every managed checkout entry and drops
// entries that name neither a repository nor a directory, so an unfilled
// placeholder hashes the same as no source at all.
func (w *WorkspaceContract) normalizeSources() {
	if w == nil || len(w.Sources) == 0 {
		return
	}
	sources := make([]WorkspaceSourceContract, 0, len(w.Sources))
	for _, src := range w.Sources {
		src.Repository = strings.TrimSpace(src.Repository)
		src.Branch = strings.TrimSpace(src.Branch)
		src.Directory = strings.TrimSpace(src.Directory)
		if src.Repository == "" && src.Directory == "" {
			continue
		}
		sources = append(sources, src)
	}
	w.Sources = sources
}

// normalizeServices trims and sorts declared services for stable hashing.
func (w *WorkspaceContract) normalizeServices() {
	if w == nil {
		return
	}
	for i := range w.Services {
		w.Services[i].Name = strings.TrimSpace(w.Services[i].Name)
		scheme := strings.ToLower(strings.TrimSpace(w.Services[i].Scheme))
		if scheme == "" {
			scheme = "http"
		}
		w.Services[i].Scheme = scheme
	}
	sort.SliceStable(w.Services, func(i, j int) bool {
		return w.Services[i].Name < w.Services[j].Name
	})
}

func uniquePreserve(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// Hash returns a deterministic SHA-256 hex digest of the redacted contract
// body. ContentHash itself is excluded. Maps/slices are normalized first.
func (c Contract) Hash() (string, error) {
	c.Normalize()
	c.ContentHash = ""
	c.Egress.PolicyHash = ""
	if c.Egress.Mode != "" || len(c.Egress.Rules) > 0 || c.Egress.SchemaVersion != 0 {
		ph, err := hashEgressPolicy(c.Egress)
		if err != nil {
			return "", err
		}
		c.Egress.PolicyHash = ph
	}
	body, err := canonicalJSON(c)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// WithHash returns a copy with ContentHash and Egress.PolicyHash populated.
func (c Contract) WithHash() (Contract, error) {
	c.Normalize()
	if c.Egress.Mode != "" || len(c.Egress.Rules) > 0 || c.Egress.SchemaVersion != 0 {
		ph, err := hashEgressPolicy(c.Egress)
		if err != nil {
			return Contract{}, err
		}
		c.Egress.PolicyHash = ph
	}
	h, err := c.Hash()
	if err != nil {
		return Contract{}, err
	}
	c.ContentHash = h
	return c, nil
}

// RedactedJSON returns stable JSON for persistence. Secret values cannot appear
// because the type system has no value fields.
func (c Contract) RedactedJSON() ([]byte, error) {
	out, err := c.WithHash()
	if err != nil {
		return nil, err
	}
	return canonicalJSON(out)
}

func hashEgressPolicy(e EgressContract) (string, error) {
	// Hash policy without the PolicyHash field to avoid self-reference.
	body := struct {
		SchemaVersion int              `json:"schema_version"`
		Mode          string           `json:"mode"`
		Default       string           `json:"default"`
		Rules         []EgressHostRule `json:"rules"`
	}{
		SchemaVersion: e.SchemaVersion,
		Mode:          e.Mode,
		Default:       e.Default,
		Rules:         append([]EgressHostRule(nil), e.Rules...),
	}
	sort.SliceStable(body.Rules, func(i, j int) bool {
		return body.Rules[i].Host < body.Rules[j].Host
	})
	b, err := canonicalJSON(body)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalJSON marshals with sorted object keys via encoding/json (Go map
// iteration is randomized, but our structs use fixed field order). We re-encode
// through a round-trip map for nested maps if present; for struct-only values
// json.Marshal field order is declaration order and stable.
func canonicalJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var normalized any
	if err := json.Unmarshal(b, &normalized); err != nil {
		return nil, err
	}
	return json.Marshal(sortJSON(normalized))
}

func sortJSON(v any) any {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		// encoding/json map marshal sorts keys in recent Go; we still rebuild
		// deterministically. Use json.Marshal on a sorted key list via ordered encoding.
		ordered := make(map[string]any, len(t))
		for _, k := range keys {
			ordered[k] = sortJSON(t[k])
		}
		return ordered
	case []any:
		out := make([]any, len(t))
		for i := range t {
			out[i] = sortJSON(t[i])
		}
		return out
	default:
		return v
	}
}

// ParseJSON unmarshals redacted contract JSON. Unknown fields (including any
// attempted credential "value" keys) are ignored by encoding/json.
func ParseJSON(data []byte) (Contract, error) {
	var c Contract
	if err := json.Unmarshal(data, &c); err != nil {
		return Contract{}, fmt.Errorf("parse contract: %w", err)
	}
	c.Normalize()
	return c, nil
}
