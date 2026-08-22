package lifecycle

import (
	"errors"
	"fmt"
)

// PartialError is a typed partial-success failure (D4): target is usable but
// source restore did not reach the expected prior observed state.
type PartialError struct {
	Source         string
	Target         string
	Expected       ObservedState
	Cause          error
	RecoveryAction string
}

func (e *PartialError) Error() string {
	if e == nil {
		return "partial lifecycle failure"
	}
	msg := fmt.Sprintf("fork target %q is ready but source %q could not be restored to %s", e.Target, e.Source, e.Expected)
	if e.RecoveryAction != "" {
		msg += "; recovery: " + e.RecoveryAction
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

func (e *PartialError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// IsPartial reports whether err is or wraps a PartialError.
func IsPartial(err error) bool {
	var pe *PartialError
	return errors.As(err, &pe)
}

// EgressError indicates egress coordination failed after a successful VM transition.
type EgressError struct {
	Op      string
	Sandbox string
	Err     error
}

func (e *EgressError) Error() string {
	if e == nil {
		return "egress error"
	}
	return fmt.Sprintf("sandbox %q %s succeeded but egress %s failed: %v", e.Sandbox, e.Op, e.Op, e.Err)
}

func (e *EgressError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
