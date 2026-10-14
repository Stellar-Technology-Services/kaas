package ui

import (
	"net"
	"strings"
	"testing"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestCredentialsFor(t *testing.T) {
	sb := state.Sandbox{Name: "demo"}
	if c := credentialsFor(sb, []state.Event{ev("secrets", "ok")}); c.Label != "deployed" || c.Fix != "" {
		t.Fatalf("deployed: %+v", c)
	}
	if c := credentialsFor(sb, []state.Event{ev("setup", "ok"), ev("secrets", "skipped")}); c.Label != "none" || !strings.Contains(c.Fix, "secrets deploy demo") {
		t.Fatalf("skipped: %+v", c)
	}
	if c := credentialsFor(sb, nil); c.Label != "none" {
		t.Fatalf("no events: %+v", c)
	}
	delegated := state.Sandbox{Name: "demo", EgressMode: "proxy_only", EgressStatus: "running"}
	if c := credentialsFor(delegated, nil); !strings.HasPrefix(c.Label, "delegated") || c.Class != "ok" {
		t.Fatalf("delegated: %+v", c)
	}
	down := state.Sandbox{Name: "demo", EgressMode: "proxy_only"}
	if c := credentialsFor(down, nil); c.Class != "warn" || !strings.Contains(c.Fix, "egress configure demo") {
		t.Fatalf("proxy down: %+v", c)
	}
}

func TestProbeHostPortAndUnit(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	if !probeHostPort(port) {
		t.Fatal("expected listening port to probe true")
	}
	_ = ln.Close()
	if probeHostPort(port) {
		t.Fatal("expected closed port to probe false")
	}
	if probeHostPort(0) {
		t.Fatal("port 0 must be false")
	}
	unit := systemdUnit(state.Sandbox{Name: "demo", SSHUser: "pradeep", WorkspaceDirectory: "/home/pradeep.guest/workspace/app"}, ServiceDTO{Service: "api", GuestPort: 8080})
	for _, want := range []string{"api.service", "User=pradeep", "WorkingDirectory=/home/pradeep.guest/workspace/app", "8080", "Restart=always"} {
		if !strings.Contains(unit, want) {
			t.Errorf("unit missing %q", want)
		}
	}
}
