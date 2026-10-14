package state

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Job statuses for background CLI invocations started by the UI.
const (
	JobQueued  = "queued"
	JobRunning = "running"
	JobOK      = "ok"
	JobFailed  = "failed"
)

// Job is one background `kaas <verb> ... --jsonl` invocation.
type Job struct {
	ID         string     `json:"id"`
	Sandbox    string     `json:"sandbox"`
	Verb       string     `json:"verb"`
	Args       []string   `json:"args,omitempty"`
	Status     string     `json:"status"`
	Phase      string     `json:"phase,omitempty"`
	Summary    string     `json:"summary,omitempty"`
	ExitCode   *int       `json:"exit_code,omitempty"`
	LogPath    string     `json:"log_path,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// Active reports whether the job is still queued or running.
func (j Job) Active() bool { return j.Status == JobQueued || j.Status == JobRunning }

// CreateJob inserts a queued job and returns it with a generated id.
func (s *Store) CreateJob(sandbox, verb string, args []string) (*Job, error) {
	if sandbox == "" {
		return nil, fmt.Errorf("sandbox is required")
	}
	if verb == "" {
		return nil, fmt.Errorf("verb is required")
	}
	argsJSON, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	job := &Job{
		ID:        uuid.NewString(),
		Sandbox:   sandbox,
		Verb:      verb,
		Args:      args,
		Status:    JobQueued,
		CreatedAt: time.Now().UTC(),
	}
	_, err = s.db.Exec(`
		INSERT INTO jobs(id, sandbox, verb, args_json, status, created_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		job.ID, job.Sandbox, job.Verb, string(argsJSON), job.Status,
		job.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return nil, err
	}
	return job, nil
}

// UpdateJob persists mutable job fields (status, phase, summary, exit code, log path, timestamps).
func (s *Store) UpdateJob(job Job) error {
	if job.ID == "" {
		return fmt.Errorf("job id is required")
	}
	var exitCode any
	if job.ExitCode != nil {
		exitCode = *job.ExitCode
	}
	res, err := s.db.Exec(`
		UPDATE jobs SET status = ?, phase = ?, summary = ?, exit_code = ?, log_path = ?,
			started_at = ?, finished_at = ?
		WHERE id = ?`,
		job.Status, nullStr(job.Phase), nullStr(job.Summary), exitCode, nullStr(job.LogPath),
		nullTime(job.StartedAt), nullTime(job.FinishedAt), job.ID,
	)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: job %s", ErrNotFound, job.ID)
	}
	return nil
}

// GetJob returns a job by id.
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`
		SELECT id, sandbox, verb, args_json, status, phase, summary, exit_code, log_path, created_at, started_at, finished_at
		FROM jobs WHERE id = ?`, id)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: job %s", ErrNotFound, id)
	}
	return job, err
}

// ListJobs returns recent jobs (newest first), optionally filtered by sandbox.
func (s *Store) ListJobs(sandbox string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 50
	}
	q := `SELECT id, sandbox, verb, args_json, status, phase, summary, exit_code, log_path, created_at, started_at, finished_at FROM jobs`
	var args []any
	if sandbox != "" {
		q += ` WHERE sandbox = ?`
		args = append(args, sandbox)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *job)
	}
	return out, rows.Err()
}

// ActiveJob returns the queued/running job for a sandbox, if any.
func (s *Store) ActiveJob(sandbox string) (*Job, error) {
	row := s.db.QueryRow(`
		SELECT id, sandbox, verb, args_json, status, phase, summary, exit_code, log_path, created_at, started_at, finished_at
		FROM jobs WHERE sandbox = ? AND status IN (?, ?)
		ORDER BY created_at DESC LIMIT 1`, sandbox, JobQueued, JobRunning)
	job, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: active job for %s", ErrNotFound, sandbox)
	}
	return job, err
}

// ActiveJobs returns every queued/running job.
func (s *Store) ActiveJobs() ([]Job, error) {
	rows, err := s.db.Query(`
		SELECT id, sandbox, verb, args_json, status, phase, summary, exit_code, log_path, created_at, started_at, finished_at
		FROM jobs WHERE status IN (?, ?) ORDER BY created_at DESC`, JobQueued, JobRunning)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *job)
	}
	return out, rows.Err()
}

func scanJob(row scannable) (*Job, error) {
	var job Job
	var argsJSON, phase, summary, logPath, started, finished sql.NullString
	var exitCode sql.NullInt64
	var created string
	if err := row.Scan(&job.ID, &job.Sandbox, &job.Verb, &argsJSON, &job.Status, &phase, &summary, &exitCode, &logPath, &created, &started, &finished); err != nil {
		return nil, err
	}
	if argsJSON.Valid && argsJSON.String != "" {
		_ = json.Unmarshal([]byte(argsJSON.String), &job.Args)
	}
	job.Phase, job.Summary, job.LogPath = phase.String, summary.String, logPath.String
	if exitCode.Valid {
		code := int(exitCode.Int64)
		job.ExitCode = &code
	}
	job.CreatedAt, _ = parseTime(created)
	if started.Valid && started.String != "" {
		if t, err := parseTime(started.String); err == nil {
			job.StartedAt = &t
		}
	}
	if finished.Valid && finished.String != "" {
		if t, err := parseTime(finished.String); err == nil {
			job.FinishedAt = &t
		}
	}
	return &job, nil
}
