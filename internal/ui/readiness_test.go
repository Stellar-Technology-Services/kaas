package ui

import (
	"testing"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func ev(action, status string) state.Event { return state.Event{Action: action, Status: status} }

func TestReadinessFor(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		sb     state.Sandbox
		events []state.Event
		job    *state.Job
		want   string
		busy   bool
	}{
		{"active job wins", state.Sandbox{ObservedState: state.ObservedRunning, SetupStatus: "ok"}, nil, &state.Job{Verb: "stop"}, "Working", true},
		{"stopped", state.Sandbox{ObservedState: state.ObservedStopped}, nil, nil, "Stopped", false},
		{"failed", state.Sandbox{ObservedState: state.ObservedFailed}, nil, nil, "Failed", false},
		{"ready via setup ok", state.Sandbox{ObservedState: state.ObservedRunning, SetupStatus: "ok"}, nil, nil, "Ready", false},
		{"setup failed", state.Sandbox{ObservedState: state.ObservedRunning, SetupStatus: "failed"}, nil, nil, "Setup failed", false},
		{"running unprovisioned", state.Sandbox{ObservedState: state.ObservedRunning}, nil, nil, "Running", false},
		{"mid up", state.Sandbox{ObservedState: state.ObservedRunning, LastProvisionedAt: &now},
			[]state.Event{ev("provision", "ok"), ev("create", "ok"), ev("dependencies", "ok")}, nil, "Starting", true},
		{"all phases skipped/ok", state.Sandbox{ObservedState: state.ObservedRunning, LastProvisionedAt: &now},
			[]state.Event{ev("setup", "skipped"), ev("checkout", "skipped"), ev("egress", "ok"), ev("agents", "ok"), ev("provision", "ok"), ev("create", "ok"), ev("dependencies", "ok")}, nil, "Ready", false},
		{"transitional", state.Sandbox{ObservedState: state.ObservedStarting}, nil, nil, "Starting", true},
		{"unknown", state.Sandbox{ObservedState: ""}, nil, nil, "Unknown", false},
	}
	for _, tc := range cases {
		got := readinessFor(tc.sb, tc.events, tc.job)
		if got.Label != tc.want || got.Busy != tc.busy {
			t.Errorf("%s: got %+v, want label %q busy %v", tc.name, got, tc.want, tc.busy)
		}
	}
}

func TestPhaseProgressStopsAtCreate(t *testing.T) {
	// A previous run's phases (older than the latest create) must not count.
	events := []state.Event{ev("create", "ok"), ev("setup", "ok"), ev("provision", "ok"), ev("create", "ok")}
	done, total := phaseProgress(events)
	if done != 1 || total != 6 {
		t.Fatalf("done=%d total=%d", done, total)
	}
}

func TestAllowedActions(t *testing.T) {
	if got := allowedActions(state.Sandbox{ObservedState: state.ObservedRunning}, &state.Job{}); got != nil {
		t.Fatalf("active job must disable actions, got %v", got)
	}
	running := allowedActions(state.Sandbox{ObservedState: state.ObservedRunning}, nil)
	if !actionAllowed(running, "stop") || actionAllowed(running, "start") {
		t.Fatalf("running: %v", running)
	}
	stopped := allowedActions(state.Sandbox{ObservedState: state.ObservedStopped}, nil)
	if !actionAllowed(stopped, "start") || actionAllowed(stopped, "stop") || !actionAllowed(stopped, "fork") {
		t.Fatalf("stopped: %v", stopped)
	}
	if got := allowedActions(state.Sandbox{ObservedState: state.ObservedStarting}, nil); got != nil {
		t.Fatalf("transitional: %v", got)
	}
}
