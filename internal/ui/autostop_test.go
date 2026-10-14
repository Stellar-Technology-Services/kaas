package ui

import (
	"testing"
	"time"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

func TestAutoStopDue(t *testing.T) {
	now := time.Date(2026, 10, 14, 12, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Hour)
	list := []state.Sandbox{
		{Name: "overdue", ObservedState: state.ObservedRunning, AutoStopAt: &past},
		{Name: "later", ObservedState: state.ObservedRunning, AutoStopAt: &future},
		{Name: "stopped", ObservedState: state.ObservedStopped, AutoStopAt: &past},
		{Name: "never", ObservedState: state.ObservedRunning},
		{Name: "tpl", ObservedState: state.ObservedRunning, AutoStopAt: &past, Role: state.RoleTemplate},
		{Name: "busy", ObservedState: state.ObservedRunning, AutoStopAt: &past},
	}
	due := autoStopDue(list, map[string]bool{"busy": true}, now)
	if len(due) != 1 || due[0].Name != "overdue" {
		t.Fatalf("due = %v", names(due))
	}
}

func names(list []state.Sandbox) []string {
	out := make([]string, 0, len(list))
	for _, sb := range list {
		out = append(out, sb.Name)
	}
	return out
}
