package lifecycle_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/identity"
	"github.com/Stellar-Technology-Services/kaas/internal/sandbox/lifecycle"
	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func testIdentity(t *testing.T, name string, port int) identity.Identity {
	t.Helper()
	id := identity.Identity{
		ID:       identity.SandboxID{Name: name},
		Provider: identity.ProviderIdentity{ProfileName: name},
		ControlRepo: identity.ControlRepository{
			Path: "/repo",
			Name: "repo",
		},
		Endpoint: identity.Endpoint{Host: identity.DefaultSSHHost, Port: port},
		Resources: identity.Resources{
			CPUs: 2, MemoryGiB: 4, DiskGiB: 40, Arch: "aarch64",
		},
	}
	if port > 0 {
		if err := identity.Validate(id); err != nil {
			t.Fatal(err)
		}
	} else if err := identity.ValidateName(name); err != nil {
		t.Fatal(err)
	}
	return id
}

func newTestService(t *testing.T) (*lifecycle.Service, *lifecycle.FakeProvider, *state.Store) {
	t.Helper()
	store, err := state.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	fake := lifecycle.NewFakeProvider()
	svc := lifecycle.NewService(lifecycle.Deps{
		Provider: fake,
		Store:    store,
		Events:   lifecycle.StoreEvents{Store: store},
		Egress:   lifecycle.NoopEgress{},
		Ports:    lifecycle.NewStorePortAllocator(store),
		PortBase: 6022,
		Clock:    func() time.Time { return time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC) },
	})
	return svc, fake, store
}

func TestCreateSuccess(t *testing.T) {
	svc, _, store := newTestService(t)
	ctx := context.Background()
	res, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "demo", 0)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Created || res.State.Observed != lifecycle.ObservedRunning {
		t.Fatalf("result: %+v", res)
	}
	sb, err := store.GetSandbox("demo")
	if err != nil {
		t.Fatal(err)
	}
	if sb.DesiredState != state.DesiredRunning || sb.ObservedState != state.ObservedRunning {
		t.Fatalf("lifecycle: %+v", sb)
	}
	if sb.SSHPort < 6022 {
		t.Fatalf("port: %d", sb.SSHPort)
	}
}

func TestCreateFailureKeepsRegistry(t *testing.T) {
	svc, fake, store := newTestService(t)
	fake.Failures["Create"] = errors.New("boom")
	_, err := svc.Create(context.Background(), lifecycle.CreateInput{Identity: testIdentity(t, "fail-me", 6101)})
	if err == nil {
		t.Fatal("expected error")
	}
	sb, getErr := store.GetSandbox("fail-me")
	if getErr != nil {
		t.Fatalf("registry should be kept: %v", getErr)
	}
	if sb.ObservedState != state.ObservedFailed {
		t.Fatalf("observed: %q", sb.ObservedState)
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	svc, fake, store := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "up", 6102)}); err != nil {
		t.Fatal(err)
	}
	fake.Failures["Start"] = errors.New("should not start")
	if err := svc.Start(ctx, "up"); err != nil {
		t.Fatal(err)
	}
	sb, _ := store.GetSandbox("up")
	if sb.ObservedState != state.ObservedRunning {
		t.Fatalf("observed: %q", sb.ObservedState)
	}
}

func TestStopAlreadyStopped(t *testing.T) {
	svc, fake, _ := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "down", 6103)}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop(ctx, "down"); err != nil {
		t.Fatal(err)
	}
	fake.Failures["Stop"] = errors.New("should not stop")
	if err := svc.Stop(ctx, "down"); err != nil {
		t.Fatal(err)
	}
}

func TestDestroyFailureRetainsRegistry(t *testing.T) {
	svc, fake, store := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "keep", 6104)}); err != nil {
		t.Fatal(err)
	}
	fake.Failures["Delete"] = errors.New("delete failed")
	err := svc.Destroy(ctx, lifecycle.DestroyInput{Name: "keep"})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, getErr := store.GetSandbox("keep"); getErr != nil {
		t.Fatalf("registry should remain: %v", getErr)
	}
}

func TestDestroyForceRemovesRegistry(t *testing.T) {
	svc, fake, store := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "force-me", 6105)}); err != nil {
		t.Fatal(err)
	}
	fake.Failures["Delete"] = errors.New("delete failed")
	if err := svc.Destroy(ctx, lifecycle.DestroyInput{Name: "force-me", Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetSandbox("force-me"); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("expected registry removed, got %v", err)
	}
}

func TestDestroyIdempotentWhenAbsent(t *testing.T) {
	svc, _, _ := newTestService(t)
	err := svc.Destroy(context.Background(), lifecycle.DestroyInput{Name: "ghost"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestForkFromRunningAndStopped(t *testing.T) {
	svc, _, store := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "src-run", 6110)}); err != nil {
		t.Fatal(err)
	}
	res, err := svc.Fork(ctx, lifecycle.ForkInput{Source: "src-run", Target: "dst-run"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Target.ID.Name != "dst-run" {
		t.Fatalf("target: %+v", res.Target)
	}
	src, _ := store.GetSandbox("src-run")
	if src.ObservedState != state.ObservedRunning {
		t.Fatalf("source should be restored to running: %q", src.ObservedState)
	}
	dst, _ := store.GetSandbox("dst-run")
	if dst.ObservedState != state.ObservedRunning || dst.SSHPort == src.SSHPort {
		t.Fatalf("dst: %+v src port %d", dst, src.SSHPort)
	}

	if err := svc.Stop(ctx, "src-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fork(ctx, lifecycle.ForkInput{Source: "src-run", Target: "dst-stop"}); err != nil {
		t.Fatal(err)
	}
	src2, _ := store.GetSandbox("src-run")
	if src2.ObservedState != state.ObservedStopped {
		t.Fatalf("stopped source should remain stopped: %q", src2.ObservedState)
	}
}

func TestForkSourceRestoreFailurePartialError(t *testing.T) {
	svc, fake, store := newTestService(t)
	ctx := context.Background()
	if _, err := svc.Create(ctx, lifecycle.CreateInput{Identity: testIdentity(t, "src-partial", 6120)}); err != nil {
		t.Fatal(err)
	}
	fake.FailOnce["Start"] = errors.New("cannot restart source")
	_, err := svc.Fork(ctx, lifecycle.ForkInput{Source: "src-partial", Target: "dst-partial"})
	if !lifecycle.IsPartial(err) {
		t.Fatalf("expected PartialError, got %v", err)
	}
	if _, getErr := store.GetSandbox("dst-partial"); getErr != nil {
		t.Fatalf("target should be kept: %v", getErr)
	}
}

func TestReconcileDiscrepancies(t *testing.T) {
	svc, fake, store := newTestService(t)
	ctx := context.Background()

	// Registry without provider.
	if err := store.CreateSandbox(state.Sandbox{
		Name: "orphan-reg", RepoPath: "/r", RepoName: "r", SSHPort: 6130,
		DesiredState: state.DesiredRunning, ObservedState: state.ObservedRunning,
	}); err != nil {
		t.Fatal(err)
	}
	// Provider without registry.
	fake.Seed("orphan-prov", "Running", 6131)
	// Status mismatch.
	if err := store.CreateSandbox(state.Sandbox{
		Name: "mismatch", RepoPath: "/r", RepoName: "r", SSHPort: 6132,
		DesiredState: state.DesiredRunning, ObservedState: state.ObservedRunning,
	}); err != nil {
		t.Fatal(err)
	}
	fake.Seed("mismatch", "Stopped", 6132)
	// Port mismatch.
	if err := store.CreateSandbox(state.Sandbox{
		Name: "porty", RepoPath: "/r", RepoName: "r", SSHPort: 6133,
		DesiredState: state.DesiredRunning, ObservedState: state.ObservedRunning,
	}); err != nil {
		t.Fatal(err)
	}
	fake.Seed("porty", "Running", 6199)

	ds, err := svc.Reconcile(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[lifecycle.DiscrepancyKind]bool{}
	for _, d := range ds {
		kinds[d.Kind] = true
	}
	for _, want := range []lifecycle.DiscrepancyKind{
		lifecycle.DiscrepancyRegistryWithoutProvider,
		lifecycle.DiscrepancyProviderWithoutRegistry,
		lifecycle.DiscrepancyStatusMismatch,
		lifecycle.DiscrepancyPortMismatch,
	} {
		if !kinds[want] {
			t.Fatalf("missing discrepancy %s in %+v", want, ds)
		}
	}
}

func TestPortReservationConflict(t *testing.T) {
	_, _, store := newTestService(t)
	ctx := context.Background()
	op, err := store.ClaimOperation("a", "create")
	if err != nil {
		t.Fatal(err)
	}
	expires := time.Now().UTC().Add(time.Hour)
	if err := store.ReservePort(7001, "a", op.ID, expires); err != nil {
		t.Fatal(err)
	}
	op2, err := store.ClaimOperation("b", "create")
	if err != nil {
		t.Fatal(err)
	}
	err = store.ReservePort(7001, "b", op2.ID, expires)
	if !errors.Is(err, state.ErrPortReserved) {
		t.Fatalf("expected ErrPortReserved, got %v", err)
	}
	_ = ctx
}
