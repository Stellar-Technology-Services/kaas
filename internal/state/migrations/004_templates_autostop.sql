-- Templates (stopped golden sandboxes used as `--from` sources) and auto-stop.
-- role: NULL/'' = ordinary sandbox, 'template' = deploy source (never auto-stopped).
-- auto_stop_ttl: seconds from each start to the deadline; NULL = never.
-- auto_stop_at: absolute deadline while running; cleared on stop.
ALTER TABLE sandboxes ADD COLUMN role TEXT;
ALTER TABLE sandboxes ADD COLUMN auto_stop_ttl INTEGER;
ALTER TABLE sandboxes ADD COLUMN auto_stop_at TEXT;
