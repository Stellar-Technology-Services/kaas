package cli

import (
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

func TestApplySize(t *testing.T) {
	cfg := config.BuiltIn()
	var opts createOpts
	if err := applySize(&cfg, "small", &opts); err != nil {
		t.Fatal(err)
	}
	if opts.CPUs != 2 || opts.Memory != 4 || opts.Disk != 32 {
		t.Fatalf("small preset: %+v", opts)
	}
	opts = createOpts{CPUs: 6}
	if err := applySize(&cfg, "default", &opts); err != nil {
		t.Fatal(err)
	}
	if opts.CPUs != 6 || opts.Memory != 8 || opts.Disk != 64 {
		t.Fatalf("explicit cpus must win over preset: %+v", opts)
	}
	if err := applySize(&cfg, "huge", &opts); err == nil {
		t.Fatal("expected unknown size error")
	}
}

func TestApplyRepoAndBaseName(t *testing.T) {
	cases := map[string]string{
		"git@github.com:acme/project.git":  "project",
		"https://github.com/acme/web-app":  "web-app",
		"https://github.com/acme/web-app/": "web-app",
		"/Users/me/src/thing":              "thing",
		"":                                 "project",
	}
	for in, want := range cases {
		if got := repoBaseName(in); got != want {
			t.Errorf("repoBaseName(%q) = %q, want %q", in, got, want)
		}
	}
	cfg := config.BuiltIn()
	if err := applyRepo(&cfg, "git@github.com:acme/project.git", "main"); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Workspace.Sources) != 1 || cfg.Workspace.Sources[0].Directory != "~/workspace/project" || cfg.Workspace.Sources[0].Branch != "main" {
		t.Fatalf("sources: %+v", cfg.Workspace.Sources)
	}
	if err := applyRepo(&cfg, "", "main"); err == nil {
		t.Fatal("branch without repo must fail")
	}
}

func TestApplyTrust(t *testing.T) {
	cfg := config.BuiltIn()
	if err := applyTrust(&cfg, "untrusted"); err != nil {
		t.Fatal(err)
	}
	if cfg.Egress.Mode != "proxy_only" || cfg.Egress.Default != "deny" || len(cfg.Egress.Rules) == 0 {
		t.Fatalf("untrusted preset: %+v", cfg.Egress)
	}
	for _, rule := range cfg.Egress.Rules {
		if rule.InjectSecret != nil {
			t.Fatalf("synthesized rules must not inject credentials: %+v", rule)
		}
	}
	cfg = config.BuiltIn()
	cfg.Egress = config.Egress{SchemaVersion: 1, Mode: "proxy_preferred", Default: "deny", Rules: []config.EgressRule{{Host: "example.com"}}}
	if err := applyTrust(&cfg, "untrusted"); err != nil {
		t.Fatal(err)
	}
	if cfg.Egress.Mode != "proxy_only" || len(cfg.Egress.Rules) != 1 {
		t.Fatalf("existing policy must be kept, mode forced: %+v", cfg.Egress)
	}
	if err := applyTrust(&cfg, "bogus"); err == nil {
		t.Fatal("expected error for unknown trust preset")
	}
	cfg = config.BuiltIn()
	if err := applyTrust(&cfg, "trusted"); err != nil || cfg.Egress.Mode != "" {
		t.Fatalf("trusted must leave egress alone: %v %+v", err, cfg.Egress)
	}
}
