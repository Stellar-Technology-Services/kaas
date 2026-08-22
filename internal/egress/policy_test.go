package egress

import (
	"github.com/Stellar-Technology-Services/kaas/internal/config"
	"testing"
)

func policy() config.Egress {
	return config.Egress{SchemaVersion: 1, Mode: "proxy_preferred", Default: "deny", Rules: []config.EgressRule{{Host: "github.com"}, {Host: "api.github.com", InjectSecret: &config.EgressCredential{Key: "GITHUB_TOKEN", Type: "bearer"}}, {Host: "*.npmjs.org"}}}
}
func TestDecideSeparatesReachabilityFromCredential(t *testing.T) {
	p := policy()
	if d := Decide(p, "github.com"); !d.Allowed || d.Rule.InjectSecret != nil {
		t.Fatalf("uncredentialed allowed rule: %+v", d)
	}
	if d := Decide(p, "api.github.com"); !d.Allowed || d.Rule.InjectSecret.Key != "GITHUB_TOKEN" {
		t.Fatalf("credential rule: %+v", d)
	}
	if !Decide(p, "registry.npmjs.org").Allowed {
		t.Fatal("wildcard not allowed")
	}
	if Decide(p, "evilgithub.com").Allowed {
		t.Fatal("unmatched host allowed")
	}
}
func TestPlanHasNoValues(t *testing.T) {
	p := Plan(policy())
	if p.Rules[1].Credential != "GITHUB_TOKEN" || p.Rules[1].Injection != "bearer" {
		t.Fatalf("unexpected plan: %+v", p)
	}
}
