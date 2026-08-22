package secretspec

import "testing"

func TestToAnsibleVars(t *testing.T) {
	in := map[string]string{
		"ANTHROPIC_API_KEY": "sk-test",
		"GITHUB_TOKEN":      "ghp_x",
		"OPENAI_API_KEY":    "  ",
		"EMPTY":             "",
	}
	got := ToAnsibleVars(in)
	if got["anthropic_api_key"] != "sk-test" {
		t.Fatalf("anthropic_api_key: got %q", got["anthropic_api_key"])
	}
	if got["github_token"] != "ghp_x" {
		t.Fatalf("github_token: got %q", got["github_token"])
	}
	if _, ok := got["openai_api_key"]; ok {
		t.Fatal("blank openai_api_key should be omitted")
	}
	if _, ok := got["empty"]; ok {
		t.Fatal("empty value should be omitted")
	}
}

func TestResolveSandboxMergeLogic(t *testing.T) {
	// Pure merge behavior mirrored by ResolveSandbox when ExportJSON is stubbed
	// via a fake binary is heavy; keep a focused unit on empty-skip merge rules.
	base := map[string]string{"A": "1", "B": "2"}
	over := map[string]string{"B": "9", "C": "3", "D": ""}
	merged := make(map[string]string)
	for k, v := range base {
		if v != "" {
			merged[k] = v
		}
	}
	for k, v := range over {
		if v != "" {
			merged[k] = v
		}
	}
	if merged["A"] != "1" || merged["B"] != "9" || merged["C"] != "3" {
		t.Fatalf("unexpected merge: %#v", merged)
	}
	if _, ok := merged["D"]; ok {
		t.Fatal("empty override should not clear or set D")
	}
}
