package ui

import (
	"fmt"
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// Readiness collapses desired/observed state, setup status, and the active
// job into one badge, the way boat.dev shows `starting → ready`.
type Readiness struct {
	Label  string // Ready, Starting, Working, Stopped, Setup failed, Failed, Unknown
	Class  string // pill class: ok, warn, bad, or ""
	Detail string // short explanation shown next to the badge
	Busy   bool   // a spinner is appropriate
}

// upPhases is the `kaas up` phase order recorded as events, from the create
// event onward. `dependencies` is recorded before create and is a host
// prerequisite, not guest readiness, so it is not counted.
var upPhases = []string{"create", "provision", "agents", "secrets", "checkout", "setup"}

// phaseAliases map event actions onto the canonical phase list.
var phaseAliases = map[string]string{"egress": "secrets", "environment": "checkout", "service_sync": "setup"}

// readinessFor derives the badge from registry facts only; it never calls the provider.
func readinessFor(sb state.Sandbox, events []state.Event, active *state.Job) Readiness {
	if active != nil {
		detail := active.Verb
		if active.Phase != "" {
			detail += " · " + active.Phase
		}
		return Readiness{Label: "Working", Class: "warn", Detail: detail, Busy: true}
	}
	if sb.IsTemplate() {
		if sb.ObservedState == state.ObservedStopped || sb.ObservedState == "" {
			return Readiness{Label: "Template", Class: "ok", Detail: "stopped golden disk; deploy with --from"}
		}
		return Readiness{Label: "Template", Class: "warn", Detail: "templates should stay stopped (" + sb.ObservedState + ")"}
	}
	switch sb.ObservedState {
	case state.ObservedCreating, state.ObservedStarting:
		return Readiness{Label: "Starting", Class: "warn", Detail: sb.ObservedState, Busy: true}
	case state.ObservedStopping, state.ObservedDestroying:
		return Readiness{Label: "Stopping", Class: "warn", Detail: sb.ObservedState, Busy: true}
	case state.ObservedStopped:
		return Readiness{Label: "Stopped", Class: "", Detail: "disk and registry preserved; start to resume"}
	case state.ObservedFailed:
		return Readiness{Label: "Failed", Class: "bad", Detail: "provider reported a failure; see events"}
	case state.ObservedRunning:
		if sb.SetupStatus == "failed" {
			return Readiness{Label: "Setup failed", Class: "bad", Detail: "re-run setup after fixing the cause"}
		}
		if sb.SetupStatus == "ok" {
			return Readiness{Label: "Ready", Class: "ok", Detail: "commands, ssh, and services work now"}
		}
		done, total := phaseProgress(events)
		if sb.LastProvisionedAt == nil && done == 0 {
			return Readiness{Label: "Running", Class: "warn", Detail: "VM is up; guest not provisioned yet"}
		}
		if done >= total {
			// Every phase reported ok/skipped but setup status was never written
			// (e.g. no workspace configured): treat as ready.
			return Readiness{Label: "Ready", Class: "ok", Detail: "all phases complete"}
		}
		return Readiness{Label: "Starting", Class: "warn", Detail: fmt.Sprintf("phase %d/%d", done, total), Busy: true}
	case state.ObservedAbsent:
		return Readiness{Label: "Absent", Class: "bad", Detail: "registry row exists but the VM is gone"}
	}
	return Readiness{Label: "Unknown", Class: "", Detail: "run kaas list --refresh"}
}

// phaseProgress counts distinct `up` phases that reported ok/skipped since the
// most recent create event. Events are newest first.
func phaseProgress(events []state.Event) (done, total int) {
	total = len(upPhases)
	seen := map[string]bool{}
	for _, ev := range events {
		action := ev.Action
		if alias, ok := phaseAliases[action]; ok {
			action = alias
		}
		if !isUpPhase(action) {
			continue
		}
		if ev.Status == "ok" || ev.Status == "skipped" {
			seen[action] = true
		}
		if action == "create" {
			break
		}
	}
	return len(seen), total
}

func isUpPhase(action string) bool {
	for _, p := range upPhases {
		if p == action {
			return true
		}
	}
	return false
}

// allowedActions lists lifecycle buttons that make sense for the current state.
// Transitional states and an active job disable everything. Templates only
// support removal (and deploy, which lives on the create form).
func allowedActions(sb state.Sandbox, active *state.Job) []string {
	if active != nil {
		return nil
	}
	if sb.IsTemplate() {
		switch sb.ObservedState {
		case state.ObservedRunning, state.ObservedStopped, state.ObservedFailed, state.ObservedUnknown, state.ObservedAbsent, "":
			return []string{"destroy"}
		}
		return nil
	}
	switch sb.ObservedState {
	case state.ObservedRunning:
		return []string{"stop", "extend", "fork", "template", "setup", "sync", "adopt", "destroy"}
	case state.ObservedStopped:
		return []string{"start", "fork", "template", "adopt", "destroy"}
	case state.ObservedFailed, state.ObservedUnknown, state.ObservedAbsent, "":
		return []string{"start", "destroy"}
	}
	return nil
}

func actionAllowed(actions []string, name string) bool {
	for _, a := range actions {
		if a == name {
			return true
		}
	}
	return false
}

// connectCommands returns copyable shell lines for the detail page.
func connectCommands(sb state.Sandbox) []string {
	user := sb.SSHUser
	if user == "" {
		user = "$USER"
	}
	host := sb.SSHHost
	if host == "" {
		host = "127.0.0.1"
	}
	key := sb.SSHKey
	if key == "" {
		key = "~/.colima/_lima/_config/user"
	}
	lines := []string{
		fmt.Sprintf("kaas ssh %s", sb.Name),
		fmt.Sprintf("ssh -i %s -p %d -o IdentitiesOnly=yes %s@%s", key, sb.SSHPort, user, host),
		fmt.Sprintf("kaas cp ./file %s:~/", sb.Name),
	}
	if dir := strings.TrimSpace(sb.WorkspaceDirectory); dir != "" {
		lines = append(lines, fmt.Sprintf("kaas cp %s:%s/report.txt ./", sb.Name, dir))
	}
	return lines
}
