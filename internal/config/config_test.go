package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuiltInDefaults(t *testing.T) {
	c := BuiltIn()
	if c.Defaults.SSHPortBase != DefaultPortBase {
		t.Fatalf("ssh_port_base: got %d want %d", c.Defaults.SSHPortBase, DefaultPortBase)
	}
	if c.Output.JSONL {
		t.Fatal("jsonl should default to false")
	}
	if c.Paths.AnsibleDir != "ansible" {
		t.Fatalf("ansible dir: %q", c.Paths.AnsibleDir)
	}
}

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ansible", "ansible.cfg"), []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	yaml := `
defaults:
  cpus: 6
`
	if err := os.WriteFile(filepath.Join(dir, ProjectConfigName), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoRoot != dir {
		t.Fatalf("RepoRoot: got %q want %q", cfg.RepoRoot, dir)
	}
	if got, want := cfg.AnsibleDir(), filepath.Join(cfg.GlobalRoot, "ansible"); got != want {
		t.Fatalf("AnsibleDir: got %q want %q", got, want)
	}
	if cfg.Defaults.CPUs != 6 {
		t.Fatalf("cpus: got %d want 6", cfg.Defaults.CPUs)
	}
	if cfg.Defaults.MemoryGiB != 8 {
		t.Fatalf("memory default should remain 8, got %d", cfg.Defaults.MemoryGiB)
	}
}

func TestLoadExplicitConfigAndStateOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "custom.yaml")
	if err := os.WriteFile(cfgPath, []byte("defaults:\n  cpus: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(dir, "state.db")

	cfg, err := Load(LoadOptions{
		CWD:        dir,
		ConfigPath: cfgPath,
		StatePath:  statePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.CPUs != 2 {
		t.Fatalf("cpus: got %d", cfg.Defaults.CPUs)
	}
	if cfg.State.Path != statePath {
		t.Fatalf("state path: got %q want %q", cfg.State.Path, statePath)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ansible", "ansible.cfg"), []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KAAS_DEFAULTS_CPUS", "12")

	cfg, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.CPUs != 12 {
		t.Fatalf("cpus from env: got %d want 12", cfg.Defaults.CPUs)
	}
}

func TestValidateRejectsBadPort(t *testing.T) {
	c := BuiltIn()
	c.Defaults.SSHPortBase = 0
	if err := c.Validate(); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestValidateWorkspaceServicesAndIngressDomain(t *testing.T) {
	c := BuiltIn()
	c.Workspace.Services = map[string]WorkspaceService{
		"api": {Port: 8080, Scheme: "http"},
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	c.Workspace.Services["bad"] = WorkspaceService{Port: 8080}
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate guest port error")
	}
	c = BuiltIn()
	c.IngressRuntime.Domain = "kaas.local"
	if err := c.Validate(); err == nil {
		t.Fatal("expected .local domain rejection")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := expandHome("~/foo/bar")
	want := filepath.Join(home, "foo", "bar")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestLoadDefaultsToSASWorkspace(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := filepath.Join(home, ".kaas")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "kaas.yaml"), []byte("defaults:\n  cpus: 9\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.CPUs != 9 || cfg.GlobalRoot != workspace {
		t.Fatalf("workspace not selected: %+v", cfg)
	}
}

func TestLoadResolvesRelativeStatePathAgainstGlobalRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := filepath.Join(home, ".kaas")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "kaas.yaml"), []byte("state:\n  path: state.db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{CWD: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(workspace, "state.db")
	if cfg.State.Path != want {
		t.Fatalf("state path: got %q want %q", cfg.State.Path, want)
	}
}

func TestLoadDiscoversProjectFromNestedDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ansible", "ansible.cfg"), []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kaas.yaml"), []byte("defaults:\n  cpus: 6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(dir, "nested", "working")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{CWD: nested})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoRoot != dir {
		t.Fatalf("RepoRoot: got %q want %q", cfg.RepoRoot, dir)
	}
	if got, want := cfg.AnsibleDir(), filepath.Join(cfg.GlobalRoot, "ansible"); got != want {
		t.Fatalf("AnsibleDir: got %q want %q", got, want)
	}
}

func TestProjectConfigOverridesUserDefaults(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	workspace := filepath.Join(home, ".kaas")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "kaas.yaml"), []byte("defaults:\n  cpus: 4\n  memory_gib: 8\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "ansible", "ansible.cfg"), []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "kaas.yaml"), []byte("defaults:\n  cpus: 8\n  disk_gib: 120\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{CWD: project})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.CPUs != 8 || cfg.Defaults.MemoryGiB != 8 || cfg.Defaults.DiskGiB != 120 {
		t.Fatalf("unexpected merged defaults: %+v", cfg.Defaults)
	}
	if cfg.RepoRoot != project {
		t.Fatalf("RepoRoot: got %q want %q", cfg.RepoRoot, project)
	}
}

func TestLoadRejectsApplicationSettingsInProjectConfig(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ansible", "ansible.cfg"), []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kaas.yaml"), []byte("state:\n  path: other.db\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(LoadOptions{CWD: dir})
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected project scope error, got %v", err)
	}
}

func TestLoadRejectsNonResourceProjectDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ansible", "ansible.cfg"), []byte("[defaults]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "kaas.yaml"), []byte("defaults:\n  arch: x86_64\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(LoadOptions{CWD: dir})
	if err == nil || !strings.Contains(err.Error(), "defaults.arch") {
		t.Fatalf("expected project default scope error, got %v", err)
	}
}

func TestValidateWorkspaceGitAuth(t *testing.T) {
	c := BuiltIn()
	c.Workspace.GitAuth = "invalid"
	if err := c.Validate(); err == nil {
		t.Fatal("expected git auth validation error")
	}
	c.Workspace.GitAuth = "https"
	c.Workspace.Sources = []WorkspaceSource{{
		Repository: "git@github.com:acme/app.git",
		Directory:  "~/workspace/app",
	}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected https repository validation error")
	}
	c.Workspace.Sources[0].Repository = "https://github.com/acme/app.git"
	if err := c.Validate(); err != nil {
		t.Fatalf("valid https auth: %v", err)
	}
}

func TestValidateWorkspaceVarlock(t *testing.T) {
	c := BuiltIn()
	c.Workspace.Varlock.ValidateCommand = "varlock load --agent"
	if err := c.Validate(); err == nil {
		t.Fatal("expected schema requirement")
	}
	c.Workspace.Varlock.Schema = ".env.schema"
	if err := c.Validate(); err == nil {
		t.Fatal("expected directory requirement")
	}
	c.Workspace.Sources = []WorkspaceSource{{
		Repository: "git@github.com:acme/app.git",
		Directory:  "~/workspace/app",
	}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid Varlock configuration: %v", err)
	}
}

func TestValidateWorkspaceSourcesUniqueDirs(t *testing.T) {
	c := BuiltIn()
	c.Workspace.Sources = []WorkspaceSource{
		{Repository: "git@github.com:acme/a.git", Directory: "~/workspace/a"},
		{Repository: "git@github.com:acme/b.git", Directory: "~/workspace/a"},
	}
	if err := c.Validate(); err == nil {
		t.Fatal("expected duplicate directory error")
	}
}

func TestLoadDiscoversProjectWithoutAnsibleAssets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	global := filepath.Join(home, ".kaas")
	if err := os.MkdirAll(filepath.Join(global, "ansible"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(global, "kaas.yaml"), []byte("paths:\n  ansible_dir: ansible\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "kaas.yaml"), []byte("defaults:\n  cpus: 10\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{CWD: project})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoRoot != project || cfg.ConfigPath != filepath.Join(project, "kaas.yaml") {
		t.Fatalf("project discovery: %+v", cfg)
	}
	if got, want := cfg.AnsibleDir(), filepath.Join(global, "ansible"); got != want {
		t.Fatalf("AnsibleDir got %q want %q", got, want)
	}
}

func TestValidateEgressPolicy(t *testing.T) {
	c := BuiltIn()
	c.Egress = Egress{SchemaVersion: 1, Mode: "proxy_only", Default: "deny", Rules: []EgressRule{{Host: "github.com"}, {Host: "api.github.com", InjectSecret: &EgressCredential{Key: "GITHUB_TOKEN", Type: "bearer"}}, {Host: "registry.npmjs.org", InjectSecret: &EgressCredential{Key: "NPM_TOKEN", Type: "header", Header: "X-Npm-Token"}}}}
	if err := c.Validate(); err != nil {
		t.Fatalf("valid egress policy: %v", err)
	}
	c.Egress.Default = "allow"
	if err := c.Validate(); err == nil {
		t.Fatal("proxy_only allow default accepted")
	}
	c.Egress.Default = "deny"
	c.Egress.Rules[0].Host = "https://github.com"
	if err := c.Validate(); err == nil {
		t.Fatal("URL host accepted")
	}
}
func TestProjectConfigAllowsEgressOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kaas.yaml"), []byte("egress:\n  schema_version: 1\n  mode: proxy_preferred\n  default: deny\n  rules:\n    - host: github.com\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(LoadOptions{CWD: dir})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Egress.Mode != "proxy_preferred" {
		t.Fatalf("egress: %+v", cfg.Egress)
	}
}
