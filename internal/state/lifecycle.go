package state

import (
	"time"
)

// Desired / observed lifecycle values persisted on sandboxes (D2).
const (
	DesiredRegistered = "registered"
	DesiredRunning    = "running"
	DesiredStopped    = "stopped"
	DesiredDestroyed  = "destroyed"

	ObservedAbsent     = "absent"
	ObservedCreating   = "creating"
	ObservedStarting   = "starting"
	ObservedRunning    = "running"
	ObservedStopping   = "stopping"
	ObservedStopped    = "stopped"
	ObservedDestroying = "destroying"
	ObservedFailed     = "failed"
	ObservedUnknown    = "unknown"
)

// Operation statuses for sandbox_operations claims.
const (
	OpClaimed   = "claimed"
	OpRunning   = "running"
	OpSucceeded = "succeeded"
	OpFailed    = "failed"
)

// Operation is a lifecycle operation claim row.
type Operation struct {
	ID        string
	Sandbox   string
	Action    string
	Status    string
	Detail    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// PortReservation holds a short-lived SSH port claim.
type PortReservation struct {
	Port        int
	Sandbox     string
	OperationID string
	ExpiresAt   time.Time
}

