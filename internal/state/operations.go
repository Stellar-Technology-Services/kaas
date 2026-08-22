package state

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ClaimOperation inserts a new operation claim in claimed status.
func (s *Store) ClaimOperation(sandbox, action string) (*Operation, error) {
	if sandbox == "" {
		return nil, fmt.Errorf("sandbox is required")
	}
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	now := time.Now().UTC()
	op := &Operation{
		ID:        uuid.NewString(),
		Sandbox:   sandbox,
		Action:    action,
		Status:    OpClaimed,
		CreatedAt: now,
		UpdatedAt: now,
	}
	_, err := s.db.Exec(`
		INSERT INTO sandbox_operations(id, sandbox, action, status, detail, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		op.ID, op.Sandbox, op.Action, op.Status, nil,
		op.CreatedAt.Format(time.RFC3339Nano),
		op.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	return op, nil
}

// UpdateOperation updates status and optional detail for an operation claim.
func (s *Store) UpdateOperation(id, status, detail string) error {
	if id == "" {
		return fmt.Errorf("operation id is required")
	}
	if status == "" {
		return fmt.Errorf("operation status is required")
	}
	res, err := s.db.Exec(`
		UPDATE sandbox_operations SET status = ?, detail = ?, updated_at = ?
		WHERE id = ?`,
		status, nullStr(detail), time.Now().UTC().Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: operation %s", ErrNotFound, id)
	}
	return nil
}

// GetOperation returns an operation by id.
func (s *Store) GetOperation(id string) (*Operation, error) {
	row := s.db.QueryRow(`
		SELECT id, sandbox, action, status, detail, created_at, updated_at
		FROM sandbox_operations WHERE id = ?`, id)
	var op Operation
	var detail sql.NullString
	var created, updated string
	err := row.Scan(&op.ID, &op.Sandbox, &op.Action, &op.Status, &detail, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: operation %s", ErrNotFound, id)
	}
	if err != nil {
		return nil, err
	}
	op.Detail = detail.String
	op.CreatedAt, _ = parseTime(created)
	op.UpdatedAt, _ = parseTime(updated)
	return &op, nil
}

// ActiveOperation returns the latest claimed/running operation for a sandbox, if any.
func (s *Store) ActiveOperation(sandbox string) (*Operation, error) {
	row := s.db.QueryRow(`
		SELECT id, sandbox, action, status, detail, created_at, updated_at
		FROM sandbox_operations
		WHERE sandbox = ? AND status IN (?, ?)
		ORDER BY created_at DESC LIMIT 1`, sandbox, OpClaimed, OpRunning)
	var op Operation
	var detail sql.NullString
	var created, updated string
	err := row.Scan(&op.ID, &op.Sandbox, &op.Action, &op.Status, &detail, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: active operation for %s", ErrNotFound, sandbox)
	}
	if err != nil {
		return nil, err
	}
	op.Detail = detail.String
	op.CreatedAt, _ = parseTime(created)
	op.UpdatedAt, _ = parseTime(updated)
	return &op, nil
}

// ReservePort claims a port for a sandbox operation until expiresAt.
func (s *Store) ReservePort(port int, sandbox, operationID string, expiresAt time.Time) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if sandbox == "" || operationID == "" {
		return fmt.Errorf("sandbox and operation_id are required")
	}
	if expiresAt.IsZero() {
		return fmt.Errorf("expires_at is required")
	}
	// Drop expired reservations first so ports can be reused.
	if _, err := s.db.Exec(`DELETE FROM port_reservations WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	_, err := s.db.Exec(`
		INSERT INTO port_reservations(port, sandbox, operation_id, expires_at)
		VALUES (?, ?, ?, ?)`,
		port, sandbox, operationID, expiresAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return fmt.Errorf("%w: %d", ErrPortReserved, port)
		}
		return err
	}
	return nil
}

// ReleasePort removes a port reservation.
func (s *Store) ReleasePort(port int) error {
	_, err := s.db.Exec(`DELETE FROM port_reservations WHERE port = ?`, port)
	return err
}

// ReleasePortsForOperation removes all reservations for an operation.
func (s *Store) ReleasePortsForOperation(operationID string) error {
	_, err := s.db.Exec(`DELETE FROM port_reservations WHERE operation_id = ?`, operationID)
	return err
}

// CleanupExpiredReservations deletes reservations at or before now.
func (s *Store) CleanupExpiredReservations(now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	_, err := s.db.Exec(`DELETE FROM port_reservations WHERE expires_at <= ?`, now.UTC().Format(time.RFC3339Nano))
	return err
}
