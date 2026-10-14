package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/env"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/providers/structs"
	"github.com/knadh/koanf/v2"
	yamlv3 "gopkg.in/yaml.v3"
)

const (
	EnvPrefix         = "KAAS_"
	DefaultPortBase   = 6022
	ProjectConfigName = "kaas.yaml"
)

// Config is the merged runtime configuration for kaas.
type Config struct {
	Defaults       Defaults       `koanf:"defaults" yaml:"defaults"`
	Paths          Paths          `koanf:"paths" yaml:"paths"`
	State          State          `koanf:"state" yaml:"state"`
	Ansible        Ansible        `koanf:"ansible" yaml:"ansible"`
	Output         Output         `koanf:"output" yaml:"output"`
	Workspace      Workspace      `koanf:"workspace" yaml:"workspace"`
	Egress         Egress         `koanf:"egress" yaml:"egress"`
	EgressRuntime  EgressRuntime  `koanf:"egress_runtime" yaml:"egress_runtime"`
	IngressRuntime IngressRuntime `koanf:"ingress_runtime" yaml:"ingress_runtime"`
	UIRuntime      UIRuntime      `koanf:"ui_runtime" yaml:"ui_runtime"`

	// Resolved at load time (not from YAML).
	RepoRoot   string `koanf:"-" yaml:"-"` // project root, if a project config is active
	GlobalRoot string `koanf:"-" yaml:"-"` // ~/.kaas; owns canonical Ansible assets
	ConfigPath string `koanf:"-" yaml:"-"` // project config path, if any
	UserConfig string `koanf:"-" yaml:"-"`
}

// Output controls CLI output defaults. Command-line flags take precedence.
type Output struct {
	JSONL bool `koanf:"jsonl" yaml:"jsonl"`
}

// EgressRuntime is host-owned operational configuration. It is allowed only
// in the user config, never in a project policy.
type EgressRuntime struct {
	Root       string `koanf:"root" yaml:"root"`
	ListenHost string `koanf:"listen_host" yaml:"listen_host"`
	GuestHost  string `koanf:"guest_host" yaml:"guest_host"`
	PortBase   int    `koanf:"port_base" yaml:"port_base"`
}

// IngressRuntime is host-owned HTTP ingress and local DNS configuration.
// It is allowed only in the user config, never in a project policy.
type IngressRuntime struct {
	Root         string `koanf:"root" yaml:"root"`
	Listen       string `koanf:"listen" yaml:"listen"`         // HTTP reverse proxy, loopback
	DNSListen    string `koanf:"dns_listen" yaml:"dns_listen"` // UDP DNS for *.domain
	Domain       string `koanf:"domain" yaml:"domain"`         // e.g. kaas.test
	HostPortBase int    `koanf:"host_port_base" yaml:"host_port_base"`
}

// UIRuntime is host-owned local web dashboard configuration.
// It is allowed only in the user config, never in a project policy.
type UIRuntime struct {
	Root   string `koanf:"root" yaml:"root"`     // pid/log directory
	Listen string `koanf:"listen" yaml:"listen"` // loopback HTTP, default 127.0.0.1:18181
}

// Egress is the project-owned, versioned declaration of intended outbound
// HTTP(S) access. It contains identifiers and policy only: it never contains
// secret values or proxy-private configuration.
type Egress struct {
	SchemaVersion int          `koanf:"schema_version" yaml:"schema_version"`
	Mode          string       `koanf:"mode" yaml:"mode"`
	Default       string       `koanf:"default" yaml:"default"`
	Rules         []EgressRule `koanf:"rules" yaml:"rules"`
}

// EgressRule independently describes reachability and optional credential use.
// A rule without InjectSecret permits the host but never attaches a credential.
type EgressRule struct {
	Host         string            `koanf:"host" yaml:"host"`
	InjectSecret *EgressCredential `koanf:"inject_secret" yaml:"inject_secret"`
}

// EgressCredential selects a SecretSpec key for injection at a future trusted
// egress boundary. Key is an identifier, never a credential value.
type EgressCredential struct {
	Key    string `koanf:"key" yaml:"key"`
	Type   string `koanf:"type" yaml:"type"` // bearer, basic, header
	Header string `koanf:"header" yaml:"header"`
}

// Workspace describes optional repository-specific sandbox bootstrap work.
// Managed checkouts live under Sources; the primary is Sources[0].
type Workspace struct {
	GitAuth  string                      `koanf:"git_auth" yaml:"git_auth"` // agent (default), guest_key, https, none
	Varlock  Varlock                     `koanf:"varlock" yaml:"varlock"`
	Sources  []WorkspaceSource           `koanf:"sources" yaml:"sources"`
	Services map[string]WorkspaceService `koanf:"services" yaml:"services"`
}

// WorkspaceService declares a guest HTTP (or similar) listener exposed on the host.
type WorkspaceService struct {
	Port   int    `koanf:"port" yaml:"port"`
	Scheme string `koanf:"scheme" yaml:"scheme"` // http (default), https
}

// WorkspaceSource is one managed git checkout inside the guest.
type WorkspaceSource struct {
	Repository string `koanf:"repository" yaml:"repository"`
	Branch     string `koanf:"branch" yaml:"branch"`
	Directory  string `koanf:"directory" yaml:"directory"`
}

// Primary returns Sources[0] when present.
func (w Workspace) Primary() (WorkspaceSource, bool) {
	if len(w.Sources) == 0 {
		return WorkspaceSource{}, false
	}
	return w.Sources[0], true
}

// Varlock declares an optional repository environment contract. KaaS invokes
// ValidateCommand before project setup without exposing sensitive values.
type Varlock struct {
	Schema          string `koanf:"schema" yaml:"schema"`
	ValidateCommand string `koanf:"validate_command" yaml:"validate_command"`
}

type Defaults struct {
	CPUs        int             `koanf:"cpus" yaml:"cpus"`
	MemoryGiB   int             `koanf:"memory_gib" yaml:"memory_gib"`
	DiskGiB     int             `koanf:"disk_gib" yaml:"disk_gib"`
	Arch        string          `koanf:"arch" yaml:"arch"`
	SSHPortBase int             `koanf:"ssh_port_base" yaml:"ssh_port_base"`
	Sizes       map[string]Size `koanf:"sizes" yaml:"sizes"` // named presets for --size; host config only
	// AutoStop is the default `--auto-stop` for new sandboxes: a Go duration
	// such as "1h" or "never" (the built-in, preserving today's behaviour).
	AutoStop string `koanf:"auto_stop" yaml:"auto_stop"`
}

// Size is a named VM resource preset (`kaas up --size small`). The special
// name "default" always resolves to Defaults.CPUs/MemoryGiB/DiskGiB.
type Size struct {
	CPUs      int `koanf:"cpus" yaml:"cpus"`
	MemoryGiB int `koanf:"memory_gib" yaml:"memory_gib"`
	DiskGiB   int `koanf:"disk_gib" yaml:"disk_gib"`
}

// SizeNames returns preset names in display order (default first).
func (d Defaults) SizeNames() []string {
	names := []string{"default"}
	rest := make([]string, 0, len(d.Sizes))
	for name := range d.Sizes {
		if name != "default" {
			rest = append(rest, name)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		// Order by CPUs so small < default < large reads naturally.
		return d.Sizes[rest[i]].CPUs < d.Sizes[rest[j]].CPUs
	})
	return append(names, rest...)
}

// ResolveSize returns the resources for a preset name.
func (d Defaults) ResolveSize(name string) (Size, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "default" {
		return Size{CPUs: d.CPUs, MemoryGiB: d.MemoryGiB, DiskGiB: d.DiskGiB}, nil
	}
	if s, ok := d.Sizes[name]; ok {
		return s, nil
	}
	return Size{}, fmt.Errorf("unknown size %q (available: %s)", name, strings.Join(d.SizeNames(), ", "))
}

type Paths struct {
	AnsibleDir string `koanf:"ansible_dir" yaml:"ansible_dir"`
	SSHKey     string `koanf:"ssh_key" yaml:"ssh_key"`
	LogsDir    string `koanf:"logs_dir" yaml:"logs_dir"` // per-sandbox phase logs (~/.kaas/logs)
}

type State struct {
	Path string `koanf:"path" yaml:"path"`
}

type Ansible struct {
	Playbooks Playbooks `koanf:"playbooks" yaml:"playbooks"`
}

type Playbooks struct {
	Create  string `koanf:"create" yaml:"create"`
	Setup   string `koanf:"setup" yaml:"setup"`
	Agents  string `koanf:"agents" yaml:"agents"`
	Secrets string `koanf:"secrets" yaml:"secrets"`
	Pubkey  string `koanf:"pubkey" yaml:"pubkey"`
	Egress  string `koanf:"egress" yaml:"egress"`
	All     string `koanf:"all" yaml:"all"`
}

// LoadOptions controls config discovery and overrides.
type LoadOptions struct {
	ConfigPath string // explicit --config
	StatePath  string // explicit --state
	CWD        string // working directory for discovery (defaults to os.Getwd)
}

// Defaults returns built-in configuration values.
func BuiltIn() Config {
	home, _ := os.UserHomeDir()
	statePath := filepath.Join(home, ".kaas", "state.db")
	return Config{
		Defaults: Defaults{
			CPUs:        4,
			MemoryGiB:   8,
			DiskGiB:     64,
			Arch:        "aarch64",
			SSHPortBase: DefaultPortBase,
			Sizes: map[string]Size{
				"small": {CPUs: 2, MemoryGiB: 4, DiskGiB: 32},
				"large": {CPUs: 8, MemoryGiB: 16, DiskGiB: 128},
			},
		},
		Paths: Paths{
			AnsibleDir: "ansible",
			SSHKey:     "~/.colima/_lima/_config/user",
			LogsDir:    filepath.Join(home, ".kaas", "logs"),
		},
		State:         State{Path: statePath},
		Output:        Output{JSONL: false},
		EgressRuntime: EgressRuntime{Root: filepath.Join(home, ".kaas", "egress"), ListenHost: "0.0.0.0", GuestHost: "host.lima.internal", PortBase: 18080},
		IngressRuntime: IngressRuntime{
			Root:         filepath.Join(home, ".kaas", "ingress"),
			Listen:       "127.0.0.1:18000",
			DNSListen:    "127.0.0.1:5353",
			Domain:       "kaas.test",
			HostPortBase: 19000,
		},
		UIRuntime: UIRuntime{
			Root:   filepath.Join(home, ".kaas", "ui"),
			Listen: "127.0.0.1:18181",
		},
		Ansible: Ansible{
			Playbooks: Playbooks{
				Setup:   "setup.yml",
				Agents:  "coding-agents.yml",
				Secrets: "deploy-secrets.yml",
				Pubkey:  "pubkey.yml",
				Egress:  "egress.yml",
			},
		},
	}
}

// Load merges built-in defaults, the per-user workspace, project config, env, and flags.
// Precedence: flags > env (KAAS_*) > project kaas.yaml > ~/.kaas/kaas.yaml > built-in.
func Load(opts LoadOptions) (*Config, error) {
	cwd := opts.CWD
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
	}

	k := koanf.New(".")

	builtIn := BuiltIn()
	if err := k.Load(structs.Provider(builtIn, "koanf"), nil); err != nil {
		return nil, fmt.Errorf("load built-in defaults: %w", err)
	}

	userCfg := userConfigPath()
	globalRoot := ""
	if userCfg != "" {
		globalRoot = filepath.Dir(userCfg)
	}
	if userCfg != "" {
		if _, err := os.Stat(userCfg); err == nil {
			if err := k.Load(file.Provider(userCfg), yaml.Parser()); err != nil {
				return nil, fmt.Errorf("load user config %s: %w", userCfg, err)
			}
		}
	}

	repoRoot, projectCfg, err := discoverProject(cwd, opts.ConfigPath)
	if err != nil {
		return nil, err
	}
	// A repository-local configuration is intentionally loaded after the
	// user workspace, so projects can make their own resource requirements.
	if projectCfg != "" {
		if err := validateProjectConfig(projectCfg); err != nil {
			return nil, err
		}
		if err := k.Load(file.Provider(projectCfg), yaml.Parser()); err != nil {
			return nil, fmt.Errorf("load project config %s: %w", projectCfg, err)
		}
	}

	// KAAS_DEFAULTS_CPUS → defaults.cpus, etc.
	if err := k.Load(env.Provider(EnvPrefix, ".", func(s string) string {
		s = strings.TrimPrefix(s, EnvPrefix)
		s = strings.ToLower(s)
		return strings.ReplaceAll(s, "_", ".")
	}), nil); err != nil {
		return nil, fmt.Errorf("load env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	cfg.RepoRoot = repoRoot
	cfg.GlobalRoot = globalRoot
	cfg.ConfigPath = projectCfg
	cfg.UserConfig = userCfg

	if opts.StatePath != "" {
		cfg.State.Path = opts.StatePath
	}

	cfg.State.Path = expandHome(cfg.State.Path)
	cfg.Paths.SSHKey = expandHome(cfg.Paths.SSHKey)
	cfg.Paths.LogsDir = expandHome(cfg.Paths.LogsDir)
	cfg.EgressRuntime.Root = expandHome(cfg.EgressRuntime.Root)
	cfg.IngressRuntime.Root = expandHome(cfg.IngressRuntime.Root)
	cfg.UIRuntime.Root = expandHome(cfg.UIRuntime.Root)
	// Relative state paths are resolved against the user-global workspace so a
	// mis-set `state.path: state.db` never creates a DB in the caller's CWD.
	if cfg.State.Path != "" && !filepath.IsAbs(cfg.State.Path) && cfg.GlobalRoot != "" {
		cfg.State.Path = filepath.Join(cfg.GlobalRoot, cfg.State.Path)
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.Defaults.SSHPortBase < 1 || c.Defaults.SSHPortBase > 65535 {
		return fmt.Errorf("defaults.ssh_port_base must be a valid TCP port")
	}
	if c.Defaults.CPUs < 1 {
		return fmt.Errorf("defaults.cpus must be >= 1")
	}
	for name, size := range c.Defaults.Sizes {
		if size.CPUs < 1 || size.MemoryGiB < 1 || size.DiskGiB < 1 {
			return fmt.Errorf("defaults.sizes.%s: cpus, memory_gib, and disk_gib must all be >= 1", name)
		}
	}
	if _, err := ParseAutoStop(c.Defaults.AutoStop); err != nil {
		return fmt.Errorf("defaults.auto_stop: %w", err)
	}
	if c.State.Path == "" {
		return fmt.Errorf("state.path is required")
	}
	if c.EgressRuntime.PortBase < 1 || c.EgressRuntime.PortBase > 65500 {
		return fmt.Errorf("egress_runtime.port_base must be a valid TCP port")
	}
	if c.EgressRuntime.Root == "" || c.EgressRuntime.ListenHost == "" || c.EgressRuntime.GuestHost == "" {
		return fmt.Errorf("egress_runtime root, listen_host, and guest_host are required")
	}
	if err := c.IngressRuntime.Validate(); err != nil {
		return err
	}
	if err := c.UIRuntime.Validate(); err != nil {
		return err
	}
	if c.Workspace.GitAuth == "" {
		c.Workspace.GitAuth = "agent"
	}
	switch c.Workspace.GitAuth {
	case "agent", "guest_key", "https", "none":
	default:
		return fmt.Errorf("workspace.git_auth must be agent, guest_key, https, or none (got %q)", c.Workspace.GitAuth)
	}
	if err := c.Workspace.normalizeAndValidate(); err != nil {
		return err
	}
	if err := c.Egress.Validate(); err != nil {
		return err
	}
	return nil
}

// normalizeAndValidate trims the managed checkout list, drops unset entries,
// and validates the rest. A scaffolded-but-unfilled entry means "no source
// configured" and must never stop kaas from running.
func (w *Workspace) normalizeAndValidate() error {
	sources := make([]WorkspaceSource, 0, len(w.Sources))
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

	dirs := make(map[string]int, len(w.Sources))
	for i, src := range w.Sources {
		if src.Directory == "" {
			return fmt.Errorf("workspace.sources[%d]: directory is required", i)
		}
		if prev, ok := dirs[src.Directory]; ok {
			return fmt.Errorf("workspace.sources[%d]: directory %q duplicates sources[%d]", i, src.Directory, prev)
		}
		dirs[src.Directory] = i
		if w.GitAuth == "https" && src.Repository != "" && !strings.HasPrefix(src.Repository, "https://") {
			return fmt.Errorf("workspace.git_auth: https requires an https:// workspace.sources[%d].repository", i)
		}
	}

	if w.Varlock.ValidateCommand != "" && w.Varlock.Schema == "" {
		return fmt.Errorf("workspace.varlock.validate_command requires workspace.varlock.schema")
	}
	if err := validateWorkspaceServices(w.Services); err != nil {
		return err
	}
	return nil
}

var serviceNameRE = regexp.MustCompile(`^[a-z]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validateWorkspaceServices(services map[string]WorkspaceService) error {
	if len(services) == 0 {
		return nil
	}
	ports := make(map[int]string, len(services))
	for name, svc := range services {
		if !serviceNameRE.MatchString(name) {
			return fmt.Errorf("workspace.services[%q]: name must be a lowercase DNS label", name)
		}
		if svc.Port < 1 || svc.Port > 65535 {
			return fmt.Errorf("workspace.services[%s].port must be a valid TCP port", name)
		}
		if prev, ok := ports[svc.Port]; ok {
			return fmt.Errorf("workspace.services[%s].port %d duplicates services[%s]", name, svc.Port, prev)
		}
		ports[svc.Port] = name
		scheme := strings.ToLower(strings.TrimSpace(svc.Scheme))
		if scheme == "" {
			scheme = "http"
		}
		switch scheme {
		case "http", "https":
		default:
			return fmt.Errorf("workspace.services[%s].scheme must be http or https", name)
		}
		svc.Scheme = scheme
		services[name] = svc
	}
	return nil
}

// Validate checks host-owned ingress runtime settings.
func (r IngressRuntime) Validate() error {
	if r.Root == "" {
		return fmt.Errorf("ingress_runtime.root is required")
	}
	if r.Listen == "" {
		return fmt.Errorf("ingress_runtime.listen is required")
	}
	if r.DNSListen == "" {
		return fmt.Errorf("ingress_runtime.dns_listen is required")
	}
	if r.Domain == "" {
		return fmt.Errorf("ingress_runtime.domain is required")
	}
	if strings.EqualFold(r.Domain, "local") || strings.HasSuffix(strings.ToLower(r.Domain), ".local") {
		return fmt.Errorf("ingress_runtime.domain must not use .local (reserved for mDNS); prefer kaas.test")
	}
	if r.HostPortBase < 1 || r.HostPortBase > 65500 {
		return fmt.Errorf("ingress_runtime.host_port_base must be a valid TCP port")
	}
	return nil
}

// Validate checks host-owned UI runtime settings.
func (r UIRuntime) Validate() error {
	if r.Root == "" {
		return fmt.Errorf("ui_runtime.root is required")
	}
	if r.Listen == "" {
		return fmt.Errorf("ui_runtime.listen is required")
	}
	host, _, err := net.SplitHostPort(r.Listen)
	if err != nil {
		return fmt.Errorf("ui_runtime.listen: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("ui_runtime.listen must be a loopback address (got %q)", host)
	}
	return nil
}

var egressHostRE = regexp.MustCompile(`^(?:\*\.)?[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
var secretKeyRE = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
var headerNameRE = regexp.MustCompile(`^[!#$%&'*+.^_` + "`" + `|~0-9A-Za-z-]+$`)

// Validate enforces a deliberately small, reviewable first policy language.
func (e Egress) Validate() error {
	if e.SchemaVersion == 0 && e.Mode == "" && e.Default == "" && len(e.Rules) == 0 {
		return nil // egress is opt-in
	}
	if e.SchemaVersion != 1 {
		return fmt.Errorf("egress.schema_version must be 1")
	}
	switch e.Mode {
	case "open", "proxy_preferred", "proxy_only":
	default:
		return fmt.Errorf("egress.mode must be open, proxy_preferred, or proxy_only (got %q)", e.Mode)
	}
	switch e.Default {
	case "allow", "deny":
	default:
		return fmt.Errorf("egress.default must be allow or deny (got %q)", e.Default)
	}
	if e.Mode == "proxy_only" && e.Default != "deny" {
		return fmt.Errorf("egress.proxy_only requires egress.default: deny")
	}
	seen := make(map[string]bool, len(e.Rules))
	for i, rule := range e.Rules {
		host := strings.ToLower(strings.TrimSpace(rule.Host))
		if host != rule.Host || !egressHostRE.MatchString(host) || host == "*" {
			return fmt.Errorf("egress.rules[%d].host must be a lowercase DNS host or *.subdomain", i)
		}
		if seen[host] {
			return fmt.Errorf("egress.rules[%d].host duplicates %q", i, host)
		}
		seen[host] = true
		if rule.InjectSecret == nil {
			continue
		}
		cred := rule.InjectSecret
		if !secretKeyRE.MatchString(cred.Key) {
			return fmt.Errorf("egress.rules[%d].inject_secret.key must be an uppercase SecretSpec key", i)
		}
		switch cred.Type {
		case "bearer", "basic":
			if cred.Header != "" {
				return fmt.Errorf("egress.rules[%d].inject_secret.header is only valid for type header", i)
			}
		case "header":
			if !headerNameRE.MatchString(cred.Header) {
				return fmt.Errorf("egress.rules[%d].inject_secret.header must be an HTTP header name", i)
			}
		default:
			return fmt.Errorf("egress.rules[%d].inject_secret.type must be bearer, basic, or header", i)
		}
	}
	return nil
}

// AnsibleDir returns the absolute ansible project directory.
func (c *Config) AnsibleDir() string {
	dir := c.Paths.AnsibleDir
	if dir == "" || dir == "." {
		if c.RepoRoot != "" {
			return c.RepoRoot
		}
		return "."
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	if c.GlobalRoot != "" {
		return filepath.Join(c.GlobalRoot, dir)
	}
	if c.RepoRoot != "" {
		return filepath.Join(c.RepoRoot, dir)
	}
	return dir
}

// PlaybookPath returns the absolute path for a named playbook file.
func (c *Config) PlaybookPath(name string) string {
	return resolveAgainst(c.AnsibleDir(), name)
}

func resolveAgainst(base, rel string) string {
	if rel == "" {
		return base
	}
	if filepath.IsAbs(rel) {
		return rel
	}
	return filepath.Join(base, rel)
}

// validateProjectConfig enforces the boundary between application defaults
// owned by ~/.kaas/kaas.yaml and project-specific workspace requirements.
func validateProjectConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read project config %s: %w", path, err)
	}
	var raw map[string]any
	if err := yamlv3.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("parse project config %s: %w", path, err)
	}
	for key, value := range raw {
		switch key {
		case "defaults":
			values, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("project config %s: defaults must be a mapping", path)
			}
			for field := range values {
				if field != "cpus" && field != "memory_gib" && field != "disk_gib" {
					return fmt.Errorf("project config %s: defaults.%s is not allowed; project configs may override only defaults.cpus, defaults.memory_gib, and defaults.disk_gib", path, field)
				}
			}
		case "workspace", "egress":
			// Project-owned workspace (including services) and egress-policy declarations.
		default:
			return fmt.Errorf("project config %s: %q is not allowed; application settings belong in ~/.kaas/kaas.yaml. Project configs may contain only defaults (cpus, memory_gib, disk_gib), workspace, and egress", path, key)
		}
	}
	return nil
}

func userConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kaas", ProjectConfigName)
}

// discoverProject finds the repository root (whose ansible/ contains ansible.cfg) and project config.
func discoverProject(cwd, explicitConfig string) (repoRoot, configPath string, err error) {
	if explicitConfig != "" {
		abs, err := filepath.Abs(explicitConfig)
		if err != nil {
			return "", "", fmt.Errorf("resolve config path: %w", err)
		}
		if _, err := os.Stat(abs); err != nil {
			return "", "", fmt.Errorf("config file: %w", err)
		}
		if user := userConfigPath(); user != "" && abs == user {
			return "", "", nil
		}
		return filepath.Dir(abs), abs, nil
	}
	return findProjectConfig(cwd)
}

// findProjectConfig finds the nearest project-local kaas.yaml without requiring
// project copies of KaaS's global Ansible assets.
func findProjectConfig(start string) (string, string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", "", err
	}
	global := userConfigPath()
	for {
		candidate := filepath.Join(dir, ProjectConfigName)
		if candidate != global {
			if _, err := os.Stat(candidate); err == nil {
				return dir, candidate, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", nil
		}
		dir = parent
	}
}

func findAnsibleRoot(start string) string {
	dir, err := filepath.Abs(start)
	if err != nil {
		return ""
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "ansible", "ansible.cfg")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func expandHome(path string) string {
	if path == "" || path[0] != '~' {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	return path
}
