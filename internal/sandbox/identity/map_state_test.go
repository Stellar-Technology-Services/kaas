package identity

import (
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestFromStateAndApplyRoundTrip(t *testing.T) {
	sb := state.Sandbox{
		Name:                "demo",
		RepoPath:            "/repo",
		RepoName:            "repo",
		SSHHost:             "",
		SSHPort:             6022,
		SSHUser:             "bob",
		SSHKey:              "/key",
		CPUs:                2,
		MemoryGiB:           4,
		DiskGiB:             40,
		Arch:                "x86_64",
		ConfigSource:        "/cfg",
		WorkspaceRepository: "git@example.com:acme/app.git",
		WorkspaceBranch:     "dev",
		WorkspaceDirectory:  "~/workspace/app",
		WorkspaceCommit:     "abc",
		SetupStatus:         "ok",
		EgressMode:          "proxy_only",
		EgressPort:          8080,
		EgressStatus:        "up",
		EgressCAFingerprint: "fp",
		EgressPolicyHash:    "hash",
	}

	id, err := FromState(sb)
	if err != nil {
		t.Fatal(err)
	}
	if id.Endpoint.Host != DefaultSSHHost {
		t.Fatalf("default host: %q", id.Endpoint.Host)
	}
	if id.Workspace.Commit != "abc" || id.ControlRepo.Name != "repo" {
		t.Fatalf("mapped identity: %+v", id)
	}

	out := state.Sandbox{
		SetupStatus:         "ok",
		EgressMode:          "proxy_only",
		EgressPort:          8080,
		EgressStatus:        "up",
		EgressCAFingerprint: "fp",
		EgressPolicyHash:    "hash",
	}
	ApplyToState(&out, id)
	if out.Name != "demo" || out.SSHPort != 6022 || out.RepoPath != "/repo" {
		t.Fatalf("applied identity fields: %+v", out)
	}
	if out.SetupStatus != "ok" || out.EgressMode != "proxy_only" || out.EgressPort != 8080 {
		t.Fatalf("non-identity fields were clobbered: %+v", out)
	}
}

func TestFromStateDoesNotEnforceNameGrammar(t *testing.T) {
	id, err := FromState(state.Sandbox{
		Name:     "Bad_Name",
		RepoPath: "/r",
		RepoName: "r",
		SSHPort:  1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if id.ID.Name != "Bad_Name" {
		t.Fatalf("name: %q", id.ID.Name)
	}
	if err := Validate(id); err == nil {
		t.Fatal("strict Validate should reject a non-grammar name")
	}
}
