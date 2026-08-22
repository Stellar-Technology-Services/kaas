CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sandboxes (
  name TEXT PRIMARY KEY,
  repo_path TEXT NOT NULL,
  repo_name TEXT NOT NULL,
  ssh_host TEXT NOT NULL DEFAULT '127.0.0.1',
  ssh_port INTEGER NOT NULL UNIQUE,
  ssh_user TEXT,
  ssh_key TEXT,
  cpus INTEGER,
  memory_gib INTEGER,
  disk_gib INTEGER,
  arch TEXT,
  desired_state TEXT,
  observed_state TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  last_provisioned_at TEXT,
  config_source TEXT,
  workspace_repository TEXT,
  workspace_branch TEXT,
  workspace_directory TEXT,
  workspace_commit TEXT,
  setup_status TEXT,
  setup_completed_at TEXT,
  egress_mode TEXT,
  egress_port INTEGER,
  egress_status TEXT,
  egress_ca_fingerprint TEXT,
  egress_policy_hash TEXT
);

CREATE INDEX IF NOT EXISTS idx_sandboxes_repo ON sandboxes(repo_path);
CREATE INDEX IF NOT EXISTS idx_sandboxes_workspace_repository ON sandboxes(workspace_repository);

CREATE TABLE IF NOT EXISTS events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  sandbox TEXT NOT NULL,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  detail TEXT,
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_sandbox ON events(sandbox);

-- Redacted sandbox contract snapshots (schema_version >= 1).
-- Absence of a row means no contract has been persisted for the sandbox yet.
CREATE TABLE IF NOT EXISTS sandbox_contracts (
  sandbox TEXT PRIMARY KEY,
  schema_version INTEGER NOT NULL,
  contract_json TEXT NOT NULL,
  contract_hash TEXT NOT NULL,
  source_config TEXT,
  resolved_at TEXT NOT NULL,
  FOREIGN KEY(sandbox) REFERENCES sandboxes(name) ON DELETE CASCADE
);

-- Lifecycle operation claims for serializing sandbox mutations.
CREATE TABLE IF NOT EXISTS sandbox_operations (
  id TEXT PRIMARY KEY,
  sandbox TEXT NOT NULL,
  action TEXT NOT NULL,
  status TEXT NOT NULL,
  detail TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sandbox_operations_sandbox ON sandbox_operations(sandbox);

-- Short-lived SSH port reservations.
CREATE TABLE IF NOT EXISTS port_reservations (
  port INTEGER PRIMARY KEY,
  sandbox TEXT NOT NULL,
  operation_id TEXT NOT NULL,
  expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_port_reservations_expires ON port_reservations(expires_at);
