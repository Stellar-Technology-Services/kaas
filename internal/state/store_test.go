package state

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenMigrateAndCRUD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	err = s.CreateSandbox(Sandbox{
		Name:                "demo",
		RepoPath:            "/Users/me/projects/kaas",
		RepoName:            "kaas",
		WorkspaceRepository: "git@github.com:acme/payments.git",
		WorkspaceDirectory:  "~/workspace/payments",
		WorkspaceCommit:     "abc123",
		SetupStatus:         "ok",
		ConfigSource:        "/Users/me/projects/payments/kaas.yaml",
		SSHPort:             6022,
		CPUs:                4,
	})
	if err != nil {
		t.Fatal(err)
	}

	got, err := s.GetSandbox("demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.RepoPath != "/Users/me/projects/kaas" || got.RepoName != "kaas" {
		t.Fatalf("repo fields: %+v", got)
	}
	if got.SSHPort != 6022 {
		t.Fatalf("port: %d", got.SSHPort)
	}
	if got.WorkspaceRepository != "git@github.com:acme/payments.git" || got.WorkspaceCommit != "abc123" || got.SetupStatus != "ok" {
		t.Fatalf("workspace identity: %+v", got)
	}

	if err := s.CreateSandbox(Sandbox{
		Name: "demo", RepoPath: "/x", RepoName: "x", SSHPort: 6023,
	}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists, got %v", err)
	}

	if err := s.CreateSandbox(Sandbox{
		Name: "other", RepoPath: "/x", RepoName: "x", SSHPort: 6022,
	}); !errors.Is(err, ErrPortInUse) {
		t.Fatalf("expected ErrPortInUse, got %v", err)
	}

	if err := s.CreateSandbox(Sandbox{Name: "bad", SSHPort: 1}); err == nil {
		t.Fatal("expected error for missing repo fields")
	}

	list, err := s.ListSandboxes("")
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	filtered, err := s.ListSandboxes("git@github.com:acme/payments.git")
	if err != nil || len(filtered) != 1 {
		t.Fatalf("filter by workspace repository: %v len=%d", err, len(filtered))
	}

	ports, err := s.UsedPorts()
	if err != nil || len(ports) != 1 || ports[0] != 6022 {
		t.Fatalf("ports: %v %v", ports, err)
	}

	got.DesiredState = DesiredRunning
	got.ObservedState = ObservedRunning
	if err := s.UpdateSandbox(*got); err != nil {
		t.Fatal(err)
	}
	got2, _ := s.GetSandbox("demo")
	if got2.DesiredState != DesiredRunning || got2.ObservedState != ObservedRunning {
		t.Fatalf("lifecycle: desired=%q observed=%q", got2.DesiredState, got2.ObservedState)
	}

	if err := s.AddEvent("demo", "create", "ok", "started"); err != nil {
		t.Fatal(err)
	}
	events, err := s.ListEvents("demo", 10)
	if err != nil || len(events) != 1 || events[0].Action != "create" {
		t.Fatalf("events: %+v err=%v", events, err)
	}

	if err := s.DeleteSandbox("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSandbox("demo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestOpenAppliesMigrationsIdempotently(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
}

func TestSandboxContractRoundTripAndCascade(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.GetContract("missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing contract: %v", err)
	}

	if err := s.CreateSandbox(Sandbox{Name: "c1", RepoPath: "/r", RepoName: "r", SSHPort: 6100}); err != nil {
		t.Fatal(err)
	}
	if err := s.PutContract(ContractSnapshot{
		Sandbox:       "c1",
		SchemaVersion: 1,
		ContractJSON:  `{"schema_version":1,"resources":{"cpus":4,"memory_gib":8,"disk_gib":64,"arch":"aarch64"}}`,
		ContractHash:  "abc123",
		SourceConfig:  "/proj/kaas.yaml",
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetContract("c1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContractHash != "abc123" || got.SourceConfig != "/proj/kaas.yaml" {
		t.Fatalf("snapshot: %+v", got)
	}

	if err := s.DeleteSandbox("c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetContract("c1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected FK cascade, got %v", err)
	}
}

func TestLifecycleStateMigrationAndPortReservation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.CreateSandbox(Sandbox{
		Name: "life", RepoPath: "/r", RepoName: "r", SSHPort: 6300,
		DesiredState: DesiredRunning, ObservedState: ObservedRunning,
	}); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSandbox("life")
	if err != nil {
		t.Fatal(err)
	}
	if got.DesiredState != DesiredRunning || got.ObservedState != ObservedRunning {
		t.Fatalf("lifecycle fields: desired=%q observed=%q", got.DesiredState, got.ObservedState)
	}

	op, err := s.ClaimOperation("life", "create")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ReservePort(6301, "life", op.ID, got.CreatedAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	ports, err := s.UsedPorts()
	if err != nil {
		t.Fatal(err)
	}
	foundReserved := false
	for _, p := range ports {
		if p == 6301 {
			foundReserved = true
		}
	}
	if !foundReserved {
		t.Fatalf("expected reserved port in UsedPorts: %v", ports)
	}
	if err := s.ReleasePort(6301); err != nil {
		t.Fatal(err)
	}
}

func TestEgressMetadataRoundTripContainsNoCredentialValue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	sb := Sandbox{Name: "egress", RepoPath: "/repo", RepoName: "repo", SSHHost: "127.0.0.1", SSHPort: 6200, EgressMode: "proxy_only", EgressPort: 18080, EgressStatus: "running", EgressCAFingerprint: "abc", EgressPolicyHash: "def"}
	if err = s.CreateSandbox(sb); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetSandbox("egress")
	if err != nil {
		t.Fatal(err)
	}
	if got.EgressMode != "proxy_only" || got.EgressPort != 18080 || got.EgressCAFingerprint != "abc" {
		t.Fatalf("metadata: %+v", got)
	}
}
