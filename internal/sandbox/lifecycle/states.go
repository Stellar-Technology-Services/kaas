package lifecycle

import (
	"strings"

	"github.com/Stellar-Technology-Services/kaas/internal/state"
)

// DesiredState is operator intent for a sandbox (D2).
type DesiredState string

const (
	DesiredRegistered DesiredState = DesiredState(state.DesiredRegistered)
	DesiredRunning    DesiredState = DesiredState(state.DesiredRunning)
	DesiredStopped    DesiredState = DesiredState(state.DesiredStopped)
	DesiredDestroyed  DesiredState = DesiredState(state.DesiredDestroyed)
)

// ObservedState is the last reconciled provider/registry fact (D2).
type ObservedState string

const (
	ObservedAbsent     ObservedState = ObservedState(state.ObservedAbsent)
	ObservedCreating   ObservedState = ObservedState(state.ObservedCreating)
	ObservedStarting   ObservedState = ObservedState(state.ObservedStarting)
	ObservedRunning    ObservedState = ObservedState(state.ObservedRunning)
	ObservedStopping   ObservedState = ObservedState(state.ObservedStopping)
	ObservedStopped    ObservedState = ObservedState(state.ObservedStopped)
	ObservedDestroying ObservedState = ObservedState(state.ObservedDestroying)
	ObservedFailed     ObservedState = ObservedState(state.ObservedFailed)
	ObservedUnknown    ObservedState = ObservedState(state.ObservedUnknown)
)

// LifecycleState pairs desired and observed lifecycle state.
type LifecycleState struct {
	Desired  DesiredState
	Observed ObservedState
}

// ObservedFromProvider maps a provider status string into ObservedState.
func ObservedFromProvider(status string, exists bool) ObservedState {
	if !exists || strings.TrimSpace(status) == "" {
		return ObservedAbsent
	}
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return ObservedRunning
	case "stopped":
		return ObservedStopped
	case "starting":
		return ObservedStarting
	case "stopping":
		return ObservedStopping
	default:
		return ObservedUnknown
	}
}

// IsRunning reports whether observed indicates a running VM.
func (o ObservedState) IsRunning() bool {
	return o == ObservedRunning
}

// IsStopped reports whether observed indicates a stopped (but present) VM.
func (o ObservedState) IsStopped() bool {
	return o == ObservedStopped
}

// IsAbsent reports whether the provider profile is missing.
func (o ObservedState) IsAbsent() bool {
	return o == ObservedAbsent
}
