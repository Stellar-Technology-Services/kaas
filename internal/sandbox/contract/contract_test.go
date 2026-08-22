package contract

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/config"
)

func sampleContract() Contract {
	return Contract{
		SchemaVersion: CurrentSchemaVersion,
		Resources: Resources{
			CPUs: 4, MemoryGiB: 8, DiskGiB: 64, Arch: "aarch64",
			SSHPortPolicy: SSHPortPolicyNextFree,
		},
		Workspace: WorkspaceContract{
			GitAuth: "agent",
			Varlock: VarlockContract{Schema: ".env.schema"},
			Setup:   SetupContract{Script: DefaultSetupScript},
			Sources: []WorkspaceSourceContract{{
				Repository: "git@github.com:acme/app.git",
				Branch:     "main",
				Directory:  "~/workspace/app",
			}},
		},
		Secrets: SecretContract{
			Mode: SecretModeDelegated,
			Keys: []SecretReference{{Key: "GITHUB_TOKEN"}, {Key: "NPM_TOKEN"}},
		},
		Egress: EgressContract{
			SchemaVersion: 1,
			Mode:          "proxy_only",
			Default:       "deny",
			Rules: []EgressHostRule{
				{Host: "npmjs.org", CredentialKey: "NPM_TOKEN", Injection: "bearer"},
				{Host: "api.github.com", CredentialKey: "GITHUB_TOKEN", Injection: "bearer"},
			},
		},
		Provenance: Provenance{
			Sources:      []string{"built-in", "/tmp/kaas.yaml"},
			ConfigSource: "/tmp/kaas.yaml",
		},
	}
}

func TestHashStableDespiteRuleOrder(t *testing.T) {
	a := sampleContract()
	b := sampleContract()
	b.Egress.Rules = []EgressHostRule{
		{Host: "api.github.com", CredentialKey: "GITHUB_TOKEN", Injection: "bearer"},
		{Host: "npmjs.org", CredentialKey: "NPM_TOKEN", Injection: "bearer"},
	}
	b.Secrets.Keys = []SecretReference{{Key: "NPM_TOKEN"}, {Key: "GITHUB_TOKEN"}}

	ha, err := a.Hash()
	if err != nil {
		t.Fatal(err)
	}
	hb, err := b.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if ha != hb {
		t.Fatalf("hash unstable: %s vs %s", ha, hb)
	}
}

func TestHashChangesOnMeaningfulEdit(t *testing.T) {
	a := sampleContract()
	b := sampleContract()
	b.Resources.CPUs = 8
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha == hb {
		t.Fatal("expected hash change when cpus change")
	}
}

func TestHashChangesWhenSourcesChange(t *testing.T) {
	a := sampleContract()
	b := sampleContract()
	b.Workspace.Sources = append(b.Workspace.Sources, WorkspaceSourceContract{
		Repository: "git@github.com:acme/other.git",
		Directory:  "~/workspace/other",
	})
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha == hb {
		t.Fatal("expected hash change when sources change")
	}
}

func TestRedactedJSONOmitsCredentialValuesAndPrivateCA(t *testing.T) {
	raw := []byte(`{
		"schema_version": 1,
		"resources": {"cpus": 2, "memory_gib": 4, "disk_gib": 20, "arch": "aarch64"},
		"workspace": {"repository": "https://example.com/r.git", "git_auth": "https"},
		"secrets": {"mode": "delegated", "keys": [{"key": "TOKEN", "value": "super-secret"}]},
		"egress": {
			"schema_version": 1,
			"mode": "proxy_only",
			"default": "deny",
			"rules": [{"host": "api.example.com", "credential_key": "TOKEN", "value": "leak", "ca_private_key": "-----BEGIN"}],
			"ca_private_key": "-----BEGIN PRIVATE KEY-----",
			"credentials": {"TOKEN": "super-secret"}
		},
		"value": "nope",
		"private_key": "-----BEGIN"
	}`)
	c, err := ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.RedactedJSON()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, banned := range []string{
		"super-secret",
		"-----BEGIN",
		`"value"`,
		"ca_private_key",
		"private_key",
		`"credentials"`,
	} {
		if strings.Contains(s, banned) {
			t.Fatalf("redacted JSON must not contain %q; got %s", banned, s)
		}
	}
	if !strings.Contains(s, "TOKEN") {
		t.Fatalf("key reference should remain: %s", s)
	}
}

func TestContractTypesCannotRepresentSecretValues(t *testing.T) {
	banned := map[string]bool{
		"Value": true, "Secret": true, "Password": true, "Token": true,
		"PrivateKey": true, "CAKey": true, "CAPrivateKey": true,
		"CredentialValue": true, "Credentials": true,
	}
	var walk func(t reflect.Type, path string)
	walk = func(typ reflect.Type, path string) {
		if typ == nil {
			return
		}
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if banned[f.Name] {
				t.Fatalf("contract type field %s.%s must not exist (secret-bearing name)", path, f.Name)
			}
			walk(f.Type, path+"."+f.Name)
		}
	}
	walk(reflect.TypeOf(Contract{}), "Contract")
}

func TestFromConfigMapsWithoutSecretValues(t *testing.T) {
	cfg := config.BuiltIn()
	cfg.Workspace = config.Workspace{
		GitAuth: "agent",
		Sources: []config.WorkspaceSource{{
			Repository: "git@github.com:acme/app.git",
			Branch:     "main",
			Directory:  "~/workspace/app",
		}},
	}
	cfg.Egress = config.Egress{
		SchemaVersion: 1,
		Mode:          "proxy_preferred",
		Default:       "deny",
		Rules: []config.EgressRule{
			{Host: "api.github.com", InjectSecret: &config.EgressCredential{Key: "GITHUB_TOKEN", Type: "bearer"}},
		},
	}
	cfg.ConfigPath = "/proj/kaas.yaml"

	c := FromConfig(cfg)
	if c.Secrets.Mode != SecretModeDelegated {
		t.Fatalf("mode: %q", c.Secrets.Mode)
	}
	if len(c.Secrets.Keys) != 1 || c.Secrets.Keys[0].Key != "GITHUB_TOKEN" {
		t.Fatalf("keys: %+v", c.Secrets.Keys)
	}
	if c.Egress.Rules[0].CredentialKey != "GITHUB_TOKEN" {
		t.Fatalf("egress rule: %+v", c.Egress.Rules[0])
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "ListenHost") || strings.Contains(string(raw), "egress_runtime") {
		t.Fatalf("must not embed egress runtime: %s", raw)
	}
}
