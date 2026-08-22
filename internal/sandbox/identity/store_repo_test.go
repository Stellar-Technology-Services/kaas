package identity

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestStoreRepositoryRoundTrip(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	repo := NewStoreRepository(store)
	ctx := context.Background()

	in, err := New(validInput())
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(ctx, in); err != nil {
		t.Fatal(err)
	}

	got, err := repo.Get(ctx, in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != in.ID || got.Endpoint.Port != in.Endpoint.Port || got.ControlRepo != in.ControlRepo {
		t.Fatalf("got %+v want %+v", got, in)
	}

	// Non-identity columns must survive identity updates.
	sb, err := store.GetSandbox(in.ID.Name)
	if err != nil {
		t.Fatal(err)
	}
	sb.SetupStatus = "ok"
	sb.EgressMode = "proxy_only"
	sb.EgressPort = 18080
	if err := store.UpdateSandbox(*sb); err != nil {
		t.Fatal(err)
	}

	got.Endpoint.Port = 6101
	got.Resources.CPUs = 6
	if err := repo.Update(ctx, got); err != nil {
		t.Fatal(err)
	}
	sb, err = store.GetSandbox(in.ID.Name)
	if err != nil {
		t.Fatal(err)
	}
	if sb.SSHPort != 6101 || sb.CPUs != 6 {
		t.Fatalf("identity fields not updated: %+v", sb)
	}
	if sb.SetupStatus != "ok" || sb.EgressMode != "proxy_only" || sb.EgressPort != 18080 {
		t.Fatalf("non-identity fields clobbered: %+v", sb)
	}

	list, err := repo.List(ctx, Filter{})
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if err := repo.Delete(ctx, in.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, in.ID); !IsNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestStoreRepositoryRejectsInvalidCreate(t *testing.T) {
	store, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	repo := NewStoreRepository(store)
	err = repo.Create(context.Background(), Identity{
		ID:       SandboxID{Name: "bad_name"},
		Provider: ProviderIdentity{ProfileName: "bad_name"},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}
