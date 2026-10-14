-- Detached guest commands started with `kaas exec --detach` (boat.dev's
-- background commands). Output lives on the guest disk under ~/.kaas-runs/<id>.log
-- so it survives stop/start; this row is the host-side index.
CREATE TABLE IF NOT EXISTS sandbox_commands (
  id TEXT PRIMARY KEY,
  sandbox TEXT NOT NULL,
  command TEXT NOT NULL,
  cwd TEXT,
  pid INTEGER,
  status TEXT NOT NULL,
  exit_code INTEGER,
  started_at TEXT NOT NULL,
  ended_at TEXT,
  FOREIGN KEY(sandbox) REFERENCES sandboxes(name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sandbox_commands_sandbox ON sandbox_commands(sandbox, started_at);
