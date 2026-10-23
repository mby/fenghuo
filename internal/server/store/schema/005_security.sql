-- Management IDs are independent of authentication token hashes, including old rows.
ALTER TABLE sessions ADD COLUMN id TEXT NOT NULL DEFAULT '';
UPDATE sessions SET id = lower(hex(randomblob(16)));
CREATE UNIQUE INDEX sessions_id ON sessions(id);

CREATE TABLE security_audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  created_at INTEGER NOT NULL,
  action TEXT NOT NULL CHECK(length(action) BETWEEN 1 AND 40),
  target TEXT NOT NULL DEFAULT '' CHECK(length(target) <= 32),
  status INTEGER NOT NULL CHECK(status BETWEEN 200 AND 599),
  ip TEXT NOT NULL DEFAULT '' CHECK(length(ip) <= 45)
);
CREATE INDEX security_audit_created ON security_audit(created_at);
CREATE INDEX security_audit_action ON security_audit(action, id);
