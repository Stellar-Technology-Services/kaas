-- Background jobs started by the local UI (one `kaas <verb> --jsonl` process each).
-- No FK to sandboxes: a job may create a sandbox that does not exist yet, and
-- job history should survive destroy.
CREATE TABLE IF NOT EXISTS jobs (
  id TEXT PRIMARY KEY,
  sandbox TEXT NOT NULL,
  verb TEXT NOT NULL,
  args_json TEXT,
  status TEXT NOT NULL,
  phase TEXT,
  summary TEXT,
  exit_code INTEGER,
  log_path TEXT,
  created_at TEXT NOT NULL,
  started_at TEXT,
  finished_at TEXT
);

CREATE INDEX IF NOT EXISTS idx_jobs_sandbox ON jobs(sandbox);
CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at);
