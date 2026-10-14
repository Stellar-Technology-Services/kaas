package state

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Command statuses mirror boat.dev's history records.
const (
	CmdRunning = "running"
	CmdExited  = "exited"
	CmdKilled  = "killed"
	CmdLost    = "lost" // the guest no longer knows the pid (reboot, stop)
)

// Command is one detached guest process started through `kaas exec`.
type Command struct {
	ID        string     `json:"id"`
	Sandbox   string     `json:"sandbox"`
	Command   string     `json:"command"`
	Cwd       string     `json:"cwd,omitempty"`
	PID       int        `json:"pid,omitempty"`
	Status    string     `json:"status"`
	ExitCode  *int       `json:"exit_code,omitempty"`
	StartedAt time.Time  `json:"started_at"`
	EndedAt   *time.Time `json:"ended_at,omitempty"`
}

// Running reports whether the host believes the command is still alive.
func (c Command) Running() bool { return c.Status == CmdRunning }

// CreateCommand inserts a running command row.
func (s *Store) CreateCommand(c Command) error {
	if c.ID == "" || c.Sandbox == "" || c.Command == "" {
		return fmt.Errorf("id, sandbox, and command are required")
	}
	if c.Status == "" {
		c.Status = CmdRunning
	}
	if c.StartedAt.IsZero() {
		c.StartedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT INTO sandbox_commands(id, sandbox, command, cwd, pid, status, exit_code, started_at, ended_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Sandbox, c.Command, nullStr(c.Cwd), nullInt(c.PID), c.Status, nullIntPtr(c.ExitCode),
		c.StartedAt.UTC().Format(time.RFC3339Nano), nullTime(c.EndedAt),
	)
	return err
}

// UpdateCommand persists status, exit code, and end time.
func (s *Store) UpdateCommand(c Command) error {
	res, err := s.db.Exec(`UPDATE sandbox_commands SET status = ?, exit_code = ?, ended_at = ?, pid = ? WHERE id = ?`,
		c.Status, nullIntPtr(c.ExitCode), nullTime(c.EndedAt), nullInt(c.PID), c.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: command %s", ErrNotFound, c.ID)
	}
	return nil
}

// GetCommand returns one command by id (full id or unique prefix).
func (s *Store) GetCommand(id string) (*Command, error) {
	row := s.db.QueryRow(`SELECT id, sandbox, command, cwd, pid, status, exit_code, started_at, ended_at
		FROM sandbox_commands WHERE id = ? OR id LIKE ? ORDER BY started_at DESC LIMIT 1`, id, id+"%")
	c, err := scanCommand(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: command %s", ErrNotFound, id)
	}
	return c, err
}

// ListCommands returns a sandbox's commands, newest first.
func (s *Store) ListCommands(sandbox string, limit int) ([]Command, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`SELECT id, sandbox, command, cwd, pid, status, exit_code, started_at, ended_at
		FROM sandbox_commands WHERE sandbox = ? ORDER BY started_at DESC LIMIT ?`, sandbox, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Command
	for rows.Next() {
		c, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// MarkCommandsLost flags every running command of a sandbox as lost — called
// when the VM stops, since detached processes do not survive a reboot.
func (s *Store) MarkCommandsLost(sandbox string) error {
	_, err := s.db.Exec(`UPDATE sandbox_commands SET status = ?, ended_at = ? WHERE sandbox = ? AND status = ?`,
		CmdLost, time.Now().UTC().Format(time.RFC3339Nano), sandbox, CmdRunning)
	return err
}

func scanCommand(row scannable) (*Command, error) {
	var c Command
	var cwd, ended sql.NullString
	var pid, exitCode sql.NullInt64
	var started string
	if err := row.Scan(&c.ID, &c.Sandbox, &c.Command, &cwd, &pid, &c.Status, &exitCode, &started, &ended); err != nil {
		return nil, err
	}
	c.Cwd = cwd.String
	if pid.Valid {
		c.PID = int(pid.Int64)
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		c.ExitCode = &code
	}
	c.StartedAt, _ = parseTime(started)
	if ended.Valid && ended.String != "" {
		if t, err := parseTime(ended.String); err == nil {
			c.EndedAt = &t
		}
	}
	return &c, nil
}

func nullIntPtr(n *int) any {
	if n == nil {
		return nil
	}
	return *n
}
