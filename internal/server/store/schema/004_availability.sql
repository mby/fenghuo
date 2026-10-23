CREATE TABLE IF NOT EXISTS availability_intervals (
  node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  start INTEGER NOT NULL,
  end INTEGER NOT NULL CHECK (end > start),
  state TEXT NOT NULL CHECK (state IN ('online', 'offline')),
  PRIMARY KEY (node_id, start)
);
CREATE INDEX IF NOT EXISTS availability_expiry ON availability_intervals(end);
