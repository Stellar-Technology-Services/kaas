package contract

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestStoreRepositoryRoundTripAndMissing(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.CreateSandbox(state.Sandbox{
		Name: "demo", RepoPath: "/repo", RepoName: "repo", SSHPort: 6022,
	}); err != nil {
		t.Fatal(err)
	}

	repo := NewStoreRepository(st)
	ctx := context.Background()

	_, err = repo.Get(ctx, "demo")
	if !IsNotFound(err) {
		t.Fatalf("missing row: %v", err)
	}
	missing, err := GetOrNil(ctx, repo, "demo")
	if err != nil || missing != nil {
		t.Fatalf("GetOrNil: %v %#v", err, missing)
	}

	c, err := sampleContract().WithHash()
	if err != nil {
		t.Fatal(err)
	}
	hash1 := c.ContentHash
	if err := repo.Put(ctx, "demo", c); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(ctx, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if got.ContentHash != hash1 {
		t.Fatalf("hash: got %s want %s", got.ContentHash, hash1)
	}
	gotPrimary, _ := got.Workspace.Primary()
	wantPrimary, _ := c.Workspace.Primary()
	if gotPrimary.Repository != wantPrimary.Repository {
		t.Fatalf("workspace: %+v", got.Workspace)
	}
	if got.Secrets.Mode != SecretModeDelegated || len(got.Secrets.Keys) != 2 {
		t.Fatalf("secrets: %+v", got.Secrets)
	}

	// Hash stability across round-trip.
	again, err := got.Hash()
	if err != nil {
		t.Fatal(err)
	}
	if again != hash1 {
		t.Fatalf("hash unstable after round-trip: %s vs %s", again, hash1)
	}

	// Delete sandbox cascades contract.
	if err := st.DeleteSandbox("demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, "demo"); !IsNotFound(err) {
		t.Fatalf("expected cascade delete, got %v", err)
	}
}

func TestStoreRepositoryPutRequiresSandbox(t *testing.T) {
	st, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.CreateSandbox(state.Sandbox{
		Name: "demo", RepoPath: "/repo", RepoName: "repo", SSHPort: 6022,
	}); err != nil {
		t.Fatal(err)
	}
	repo := NewStoreRepository(st)
	c := sampleContract()
	if err := repo.Put(context.Background(), "demo", c); err != nil {
		t.Fatal(err)
	}
	snap, err := st.GetContract("demo")
	if err != nil {
		t.Fatal(err)
	}
	if snap.SchemaVersion != int(CurrentSchemaVersion) {
		t.Fatalf("schema: %d", snap.SchemaVersion)
	}
	if snap.ContractHash == "" || snap.ContractJSON == "" {
		t.Fatalf("snapshot incomplete: %+v", snap)
	}
}
