package state

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Service is a registered guest listener published on the host.
type Service struct {
	Sandbox   string    `json:"sandbox"`
	Service   string    `json:"service"`
	GuestPort int       `json:"guest_port"`
	HostPort  int       `json:"host_port"`
	Scheme    string    `json:"scheme"`
	Hostname  string    `json:"hostname"`
	Status    string    `json:"status,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// UpsertService inserts or updates a sandbox service row.
func (s *Store) UpsertService(svc Service) error {
	if svc.Sandbox == "" || svc.Service == "" {
		return fmt.Errorf("sandbox and service are required")
	}
	if svc.GuestPort < 1 || svc.GuestPort > 65535 {
		return fmt.Errorf("invalid guest_port: %d", svc.GuestPort)
	}
	if svc.HostPort < 1 || svc.HostPort > 65535 {
		return fmt.Errorf("invalid host_port: %d", svc.HostPort)
	}
	if svc.Hostname == "" {
		return fmt.Errorf("hostname is required")
	}
	if svc.Scheme == "" {
		svc.Scheme = "http"
	}
	now := time.Now().UTC()
	if svc.UpdatedAt.IsZero() {
		svc.UpdatedAt = now
	}
	_, err := s.db.Exec(`
		INSERT INTO sandbox_services (
			sandbox, service, guest_port, host_port, scheme, hostname, status, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(sandbox, service) DO UPDATE SET
			guest_port = excluded.guest_port,
			host_port = excluded.host_port,
			scheme = excluded.scheme,
			hostname = excluded.hostname,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		svc.Sandbox, svc.Service, svc.GuestPort, svc.HostPort, svc.Scheme, svc.Hostname,
		nullStr(svc.Status), svc.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: sandbox_services.host_port") {
			return fmt.Errorf("%w: %d", ErrPortInUse, svc.HostPort)
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed: sandbox_services.hostname") {
			return fmt.Errorf("hostname already registered: %s", svc.Hostname)
		}
		return err
	}
	return nil
}

// GetService returns one service for a sandbox.
func (s *Store) GetService(sandbox, service string) (*Service, error) {
	row := s.db.QueryRow(`
		SELECT sandbox, service, guest_port, host_port, scheme, hostname, status, updated_at
		FROM sandbox_services WHERE sandbox = ? AND service = ?`, sandbox, service)
	svc, err := scanService(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: service %s/%s", ErrNotFound, sandbox, service)
	}
	return svc, err
}

// ListServices returns services, optionally filtered by sandbox name.
func (s *Store) ListServices(sandbox string) ([]Service, error) {
	q := `
		SELECT sandbox, service, guest_port, host_port, scheme, hostname, status, updated_at
		FROM sandbox_services`
	var args []any
	if sandbox != "" {
		q += ` WHERE sandbox = ?`
		args = append(args, sandbox)
	}
	q += ` ORDER BY sandbox, service`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Service
	for rows.Next() {
		svc, err := scanService(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *svc)
	}
	return out, rows.Err()
}

// DeleteService removes one service row.
func (s *Store) DeleteService(sandbox, service string) error {
	res, err := s.db.Exec(`DELETE FROM sandbox_services WHERE sandbox = ? AND service = ?`, sandbox, service)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: service %s/%s", ErrNotFound, sandbox, service)
	}
	return nil
}

// DeleteServicesForSandbox removes all services for a sandbox.
func (s *Store) DeleteServicesForSandbox(sandbox string) error {
	_, err := s.db.Exec(`DELETE FROM sandbox_services WHERE sandbox = ?`, sandbox)
	return err
}

// UsedServicePorts returns host ports allocated to published services.
func (s *Store) UsedServicePorts() ([]int, error) {
	rows, err := s.db.Query(`SELECT host_port FROM sandbox_services ORDER BY host_port`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ports []int
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		ports = append(ports, p)
	}
	return ports, rows.Err()
}

func scanService(row scannable) (*Service, error) {
	var svc Service
	var status sql.NullString
	var updated string
	if err := row.Scan(
		&svc.Sandbox, &svc.Service, &svc.GuestPort, &svc.HostPort, &svc.Scheme, &svc.Hostname, &status, &updated,
	); err != nil {
		return nil, err
	}
	svc.Status = status.String
	svc.UpdatedAt, _ = parseTime(updated)
	return &svc, nil
}
