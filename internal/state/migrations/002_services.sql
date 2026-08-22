-- Host-published sandbox services (guest port → allocated host port + hostname).
CREATE TABLE IF NOT EXISTS sandbox_services (
  sandbox TEXT NOT NULL,
  service TEXT NOT NULL,
  guest_port INTEGER NOT NULL,
  host_port INTEGER NOT NULL UNIQUE,
  scheme TEXT NOT NULL DEFAULT 'http',
  hostname TEXT NOT NULL UNIQUE,
  status TEXT,
  updated_at TEXT NOT NULL,
  PRIMARY KEY (sandbox, service),
  FOREIGN KEY(sandbox) REFERENCES sandboxes(name) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sandbox_services_hostname ON sandbox_services(hostname);
CREATE INDEX IF NOT EXISTS idx_sandbox_services_host_port ON sandbox_services(host_port);
