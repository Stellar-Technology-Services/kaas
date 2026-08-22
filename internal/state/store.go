package state

import (
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

var (
	ErrNotFound      = errors.New("sandbox not found")
	ErrAlreadyExists = errors.New("sandbox already exists")
	ErrPortInUse     = errors.New("ssh port already in use")
	ErrPortReserved  = errors.New("ssh port reservation conflict")
)

// Sandbox is a registered Colima coding sandbox.
type Sandbox struct {
	Name                string     `json:"name"`
	RepoPath            string     `json:"repo_path"`
	RepoName            string     `json:"repo_name"`
	SSHHost             string     `json:"ssh_host"`
	SSHPort             int        `json:"ssh_port"`
	SSHUser             string     `json:"ssh_user,omitempty"`
	SSHKey              string     `json:"ssh_key,omitempty"`
	CPUs                int        `json:"cpus,omitempty"`
	MemoryGiB           int        `json:"memory_gib,omitempty"`
	DiskGiB             int        `json:"disk_gib,omitempty"`
	Arch                string     `json:"arch,omitempty"`
	DesiredState        string     `json:"desired_state,omitempty"`
	ObservedState       string     `json:"observed_state,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LastProvisionedAt   *time.Time `json:"last_provisioned_at,omitempty"`
	ConfigSource        string     `json:"config_source,omitempty"`
	WorkspaceRepository string     `json:"workspace_repository,omitempty"`
	WorkspaceBranch     string     `json:"workspace_branch,omitempty"`
	WorkspaceDirectory  string     `json:"workspace_directory,omitempty"`
	WorkspaceCommit     string     `json:"workspace_commit,omitempty"`
	SetupStatus         string     `json:"setup_status,omitempty"`
	SetupCompletedAt    *time.Time `json:"setup_completed_at,omitempty"`
	EgressMode          string     `json:"egress_mode,omitempty"`
	EgressPort          int        `json:"egress_port,omitempty"`
	EgressStatus        string     `json:"egress_status,omitempty"`
	EgressCAFingerprint string     `json:"egress_ca_fingerprint,omitempty"`
	EgressPolicyHash    string     `json:"egress_policy_hash,omitempty"`
}

// Event is an audit log entry for a sandbox action.
type Event struct {
	ID        int64     `json:"id"`
	Sandbox   string    `json:"sandbox"`
	Action    string    `json:"action"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// ContractSnapshot is a redacted persisted sandbox contract.
// A missing row means no contract has been persisted for the sandbox yet.
type ContractSnapshot struct {
	Sandbox       string    `json:"sandbox"`
	SchemaVersion int       `json:"schema_version"`
	ContractJSON  string    `json:"contract_json"`
	ContractHash  string    `json:"contract_hash"`
	SourceConfig  string    `json:"source_config,omitempty"`
	ResolvedAt    time.Time `json:"resolved_at"`
}

// Store is the SQLite-backed sandbox registry.
type Store struct {
	db *sql.DB
}

// Open opens (or creates) the state database at path and applies migrations.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("state path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create state directory: %w", err)
	}

	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at TEXT NOT NULL
	)`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}

	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	for _, name := range names {
		var version int
		if _, err := fmt.Sscanf(name, "%d_", &version); err != nil {
			return fmt.Errorf("parse migration version from %s: %w", name, err)
		}

		var exists int
		if err := s.db.QueryRow(`SELECT COUNT(1) FROM schema_migrations WHERE version = ?`, version).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			continue
		}

		body, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", name, err)
		}
		if _, err := tx.Exec(
			`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`,
			version, time.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// CreateSandbox inserts a new sandbox. repo_path and repo_name are required.
func (s *Store) CreateSandbox(sb Sandbox) error {
	if sb.Name == "" {
		return fmt.Errorf("sandbox name is required")
	}
	if sb.RepoPath == "" || sb.RepoName == "" {
		return fmt.Errorf("repo_path and repo_name are required")
	}
	if sb.SSHPort < 1 || sb.SSHPort > 65535 {
		return fmt.Errorf("invalid ssh_port: %d", sb.SSHPort)
	}
	if sb.SSHHost == "" {
		sb.SSHHost = "127.0.0.1"
	}
	now := time.Now().UTC()
	if sb.CreatedAt.IsZero() {
		sb.CreatedAt = now
	}
	sb.UpdatedAt = now

	_, err := s.db.Exec(`
		INSERT INTO sandboxes (
			name, repo_path, repo_name, ssh_host, ssh_port, ssh_user, ssh_key,
			cpus, memory_gib, disk_gib, arch, desired_state, observed_state,
			created_at, updated_at, last_provisioned_at, config_source, workspace_repository, workspace_branch, workspace_directory, workspace_commit, setup_status, setup_completed_at,
			egress_mode, egress_port, egress_status, egress_ca_fingerprint, egress_policy_hash
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sb.Name, sb.RepoPath, sb.RepoName, sb.SSHHost, sb.SSHPort, nullStr(sb.SSHUser), nullStr(sb.SSHKey),
		nullInt(sb.CPUs), nullInt(sb.MemoryGiB), nullInt(sb.DiskGiB), nullStr(sb.Arch),
		nullStr(sb.DesiredState), nullStr(sb.ObservedState),
		sb.CreatedAt.UTC().Format(time.RFC3339Nano),
		sb.UpdatedAt.UTC().Format(time.RFC3339Nano),
		nullTime(sb.LastProvisionedAt), nullStr(sb.ConfigSource), nullStr(sb.WorkspaceRepository), nullStr(sb.WorkspaceBranch), nullStr(sb.WorkspaceDirectory), nullStr(sb.WorkspaceCommit), nullStr(sb.SetupStatus), nullTime(sb.SetupCompletedAt),
		nullStr(sb.EgressMode), nullInt(sb.EgressPort), nullStr(sb.EgressStatus), nullStr(sb.EgressCAFingerprint), nullStr(sb.EgressPolicyHash),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: sandboxes.name") {
			return fmt.Errorf("%w: %s", ErrAlreadyExists, sb.Name)
		}
		if strings.Contains(err.Error(), "UNIQUE constraint failed: sandboxes.ssh_port") {
			return fmt.Errorf("%w: %d", ErrPortInUse, sb.SSHPort)
		}
		return err
	}
	return nil
}

// GetSandbox returns a sandbox by name.
func (s *Store) GetSandbox(name string) (*Sandbox, error) {
	row := s.db.QueryRow(`
		SELECT name, repo_path, repo_name, ssh_host, ssh_port, ssh_user, ssh_key,
			cpus, memory_gib, disk_gib, arch, desired_state, observed_state,
			created_at, updated_at, last_provisioned_at, config_source, workspace_repository, workspace_branch, workspace_directory, workspace_commit, setup_status, setup_completed_at,
			egress_mode, egress_port, egress_status, egress_ca_fingerprint, egress_policy_hash
		FROM sandboxes WHERE name = ?`, name)
	sb, err := scanSandbox(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return sb, err
}

// ListSandboxes returns all sandboxes, optionally filtered by repo path or name.
func (s *Store) ListSandboxes(repoFilter string) ([]Sandbox, error) {
	q := `
		SELECT name, repo_path, repo_name, ssh_host, ssh_port, ssh_user, ssh_key,
			cpus, memory_gib, disk_gib, arch, desired_state, observed_state,
			created_at, updated_at, last_provisioned_at, config_source, workspace_repository, workspace_branch, workspace_directory, workspace_commit, setup_status, setup_completed_at,
			egress_mode, egress_port, egress_status, egress_ca_fingerprint, egress_policy_hash
		FROM sandboxes`
	var args []any
	if repoFilter != "" {
		q += ` WHERE workspace_repository = ? OR workspace_directory = ?`
		args = append(args, repoFilter, repoFilter)
	}
	q += ` ORDER BY name`

	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Sandbox
	for rows.Next() {
		sb, err := scanSandbox(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *sb)
	}
	return out, rows.Err()
}

// UpdateSandbox updates mutable fields for an existing sandbox.
func (s *Store) UpdateSandbox(sb Sandbox) error {
	if sb.Name == "" {
		return fmt.Errorf("sandbox name is required")
	}
	sb.UpdatedAt = time.Now().UTC()
	res, err := s.db.Exec(`
		UPDATE sandboxes SET
			repo_path = ?, repo_name = ?, ssh_host = ?, ssh_port = ?,
			ssh_user = ?, ssh_key = ?, cpus = ?, memory_gib = ?, disk_gib = ?,
			arch = ?, desired_state = ?, observed_state = ?, updated_at = ?, last_provisioned_at = ?,
			config_source = ?, workspace_repository = ?, workspace_branch = ?, workspace_directory = ?, workspace_commit = ?, setup_status = ?, setup_completed_at = ?,
			egress_mode = ?, egress_port = ?, egress_status = ?, egress_ca_fingerprint = ?, egress_policy_hash = ?
		WHERE name = ?`,
		sb.RepoPath, sb.RepoName, sb.SSHHost, sb.SSHPort,
		nullStr(sb.SSHUser), nullStr(sb.SSHKey), nullInt(sb.CPUs), nullInt(sb.MemoryGiB), nullInt(sb.DiskGiB),
		nullStr(sb.Arch), nullStr(sb.DesiredState), nullStr(sb.ObservedState),
		sb.UpdatedAt.UTC().Format(time.RFC3339Nano), nullTime(sb.LastProvisionedAt),
		nullStr(sb.ConfigSource), nullStr(sb.WorkspaceRepository), nullStr(sb.WorkspaceBranch), nullStr(sb.WorkspaceDirectory), nullStr(sb.WorkspaceCommit), nullStr(sb.SetupStatus), nullTime(sb.SetupCompletedAt),
		nullStr(sb.EgressMode), nullInt(sb.EgressPort), nullStr(sb.EgressStatus), nullStr(sb.EgressCAFingerprint), nullStr(sb.EgressPolicyHash), sb.Name,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed: sandboxes.ssh_port") {
			return fmt.Errorf("%w: %d", ErrPortInUse, sb.SSHPort)
		}
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, sb.Name)
	}
	return nil
}

// DeleteSandbox removes a sandbox by name.
func (s *Store) DeleteSandbox(name string) error {
	res, err := s.db.Exec(`DELETE FROM sandboxes WHERE name = ?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	return nil
}

// UsedPorts returns SSH ports currently registered or reserved (non-expired).
func (s *Store) UsedPorts() ([]int, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	rows, err := s.db.Query(`
		SELECT ssh_port AS port FROM sandboxes
		UNION
		SELECT port FROM port_reservations WHERE expires_at > ?
		ORDER BY port`, now)
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

// GetContract returns the redacted contract snapshot for a sandbox.
// ErrNotFound means no contract row exists for the sandbox.
func (s *Store) GetContract(sandbox string) (*ContractSnapshot, error) {
	row := s.db.QueryRow(`
		SELECT sandbox, schema_version, contract_json, contract_hash, source_config, resolved_at
		FROM sandbox_contracts WHERE sandbox = ?`, sandbox)
	var snap ContractSnapshot
	var source sql.NullString
	var resolved string
	err := row.Scan(&snap.Sandbox, &snap.SchemaVersion, &snap.ContractJSON, &snap.ContractHash, &source, &resolved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: contract for %s", ErrNotFound, sandbox)
	}
	if err != nil {
		return nil, err
	}
	snap.SourceConfig = source.String
	snap.ResolvedAt, _ = parseTime(resolved)
	return &snap, nil
}

// PutContract upserts a redacted contract snapshot.
func (s *Store) PutContract(snap ContractSnapshot) error {
	if snap.Sandbox == "" {
		return fmt.Errorf("sandbox name is required")
	}
	if snap.ContractJSON == "" {
		return fmt.Errorf("contract_json is required")
	}
	if snap.ContractHash == "" {
		return fmt.Errorf("contract_hash is required")
	}
	if snap.SchemaVersion < 1 {
		return fmt.Errorf("schema_version must be >= 1")
	}
	if snap.ResolvedAt.IsZero() {
		snap.ResolvedAt = time.Now().UTC()
	}
	_, err := s.db.Exec(`
		INSERT INTO sandbox_contracts (
			sandbox, schema_version, contract_json, contract_hash, source_config, resolved_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(sandbox) DO UPDATE SET
			schema_version = excluded.schema_version,
			contract_json = excluded.contract_json,
			contract_hash = excluded.contract_hash,
			source_config = excluded.source_config,
			resolved_at = excluded.resolved_at`,
		snap.Sandbox, snap.SchemaVersion, snap.ContractJSON, snap.ContractHash,
		nullStr(snap.SourceConfig), snap.ResolvedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// DeleteContract removes a contract snapshot. Sandbox deletion also cascades.
func (s *Store) DeleteContract(sandbox string) error {
	res, err := s.db.Exec(`DELETE FROM sandbox_contracts WHERE sandbox = ?`, sandbox)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: contract for %s", ErrNotFound, sandbox)
	}
	return nil
}

// AddEvent appends an action event.
func (s *Store) AddEvent(sandbox, action, status, detail string) error {
	_, err := s.db.Exec(
		`INSERT INTO events(sandbox, action, status, detail, created_at) VALUES (?, ?, ?, ?, ?)`,
		sandbox, action, status, nullStr(detail), time.Now().UTC().Format(time.RFC3339Nano),
	)
	return err
}

// ListEvents returns recent events for a sandbox (newest first).
func (s *Store) ListEvents(sandbox string, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
		SELECT id, sandbox, action, status, detail, created_at
		FROM events WHERE sandbox = ?
		ORDER BY id DESC LIMIT ?`, sandbox, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var e Event
		var detail sql.NullString
		var created string
		if err := rows.Scan(&e.ID, &e.Sandbox, &e.Action, &e.Status, &detail, &created); err != nil {
			return nil, err
		}
		e.Detail = detail.String
		t, err := time.Parse(time.RFC3339Nano, created)
		if err != nil {
			t, _ = time.Parse(time.RFC3339, created)
		}
		e.CreatedAt = t
		out = append(out, e)
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...any) error
}

func scanSandbox(row scannable) (*Sandbox, error) {
	var sb Sandbox
	var sshUser, sshKey, arch, desired, observed, lastProv, configSource, workspaceRepo, workspaceBranch, workspaceDirectory, workspaceCommit, setupStatus, setupCompleted, egressMode, egressStatus, egressCA, egressHash sql.NullString
	var cpus, mem, disk, egressPort sql.NullInt64
	var created, updated string

	err := row.Scan(
		&sb.Name, &sb.RepoPath, &sb.RepoName, &sb.SSHHost, &sb.SSHPort,
		&sshUser, &sshKey, &cpus, &mem, &disk, &arch, &desired, &observed,
		&created, &updated, &lastProv, &configSource, &workspaceRepo, &workspaceBranch, &workspaceDirectory, &workspaceCommit, &setupStatus, &setupCompleted,
		&egressMode, &egressPort, &egressStatus, &egressCA, &egressHash,
	)
	if err != nil {
		return nil, err
	}
	sb.SSHUser = sshUser.String
	sb.SSHKey = sshKey.String
	sb.Arch = arch.String
	sb.DesiredState = desired.String
	sb.ObservedState = observed.String
	sb.ConfigSource, sb.WorkspaceRepository, sb.WorkspaceBranch = configSource.String, workspaceRepo.String, workspaceBranch.String
	sb.WorkspaceDirectory, sb.WorkspaceCommit, sb.SetupStatus = workspaceDirectory.String, workspaceCommit.String, setupStatus.String
	sb.EgressMode, sb.EgressStatus, sb.EgressCAFingerprint, sb.EgressPolicyHash = egressMode.String, egressStatus.String, egressCA.String, egressHash.String
	if egressPort.Valid {
		sb.EgressPort = int(egressPort.Int64)
	}
	if cpus.Valid {
		sb.CPUs = int(cpus.Int64)
	}
	if mem.Valid {
		sb.MemoryGiB = int(mem.Int64)
	}
	if disk.Valid {
		sb.DiskGiB = int(disk.Int64)
	}
	sb.CreatedAt, _ = parseTime(created)
	sb.UpdatedAt, _ = parseTime(updated)
	if lastProv.Valid && lastProv.String != "" {
		if t, err := parseTime(lastProv.String); err == nil {
			sb.LastProvisionedAt = &t
		}
	}
	if setupCompleted.Valid && setupCompleted.String != "" {
		if t, err := parseTime(setupCompleted.String); err == nil {
			sb.SetupCompletedAt = &t
		}
	}
	return &sb, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, s)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func nullTime(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return t.UTC().Format(time.RFC3339Nano)
}
