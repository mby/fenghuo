CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE TABLE admin (
  id            INTEGER PRIMARY KEY CHECK (id = 1),
  username      TEXT    NOT NULL,
  password_hash TEXT    NOT NULL,
  totp_secret   TEXT    NOT NULL DEFAULT '',
  updated_at    INTEGER NOT NULL
);

CREATE TABLE sessions (
  token_hash TEXT PRIMARY KEY,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  ip         TEXT    NOT NULL DEFAULT '',
  user_agent TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE nodes (
  id                INTEGER PRIMARY KEY,
  uuid              TEXT    NOT NULL UNIQUE,
  name              TEXT    NOT NULL,
  token_hash        TEXT    NOT NULL UNIQUE,
  group_name        TEXT    NOT NULL DEFAULT '',
  sort              INTEGER NOT NULL DEFAULT 0,
  hidden            INTEGER NOT NULL DEFAULT 0,
  note              TEXT    NOT NULL DEFAULT '',
  public_note       TEXT    NOT NULL DEFAULT '',
  expire_at         INTEGER NOT NULL DEFAULT 0,
  traffic_limit     INTEGER NOT NULL DEFAULT 0,
  traffic_mode      TEXT    NOT NULL DEFAULT 'sum',
  traffic_reset_day INTEGER NOT NULL DEFAULT 1,
  created_at        INTEGER NOT NULL
);

CREATE TABLE node_info (
  node_id       INTEGER PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  os            TEXT    NOT NULL DEFAULT '',
  kernel        TEXT    NOT NULL DEFAULT '',
  arch          TEXT    NOT NULL DEFAULT '',
  cpu_model     TEXT    NOT NULL DEFAULT '',
  cpu_cores     INTEGER NOT NULL DEFAULT 0,
  virt          TEXT    NOT NULL DEFAULT '',
  agent_version TEXT    NOT NULL DEFAULT '',
  last_ip       TEXT    NOT NULL DEFAULT '',
  updated_at    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE traffic (
  node_id      INTEGER PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
  period_start INTEGER NOT NULL,
  up           INTEGER NOT NULL DEFAULT 0,
  down         INTEGER NOT NULL DEFAULT 0,
  boot_time    INTEGER NOT NULL DEFAULT 0,
  last_rx      INTEGER NOT NULL DEFAULT 0,
  last_tx      INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE metrics_1m (
  node_id   INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  ts        INTEGER NOT NULL,
  cpu       REAL    NOT NULL,
  load1     REAL    NOT NULL,
  mem_used  INTEGER NOT NULL,
  swap_used INTEGER NOT NULL,
  disk_used INTEGER NOT NULL,
  net_in    INTEGER NOT NULL,
  net_out   INTEGER NOT NULL,
  tcp       INTEGER NOT NULL,
  udp       INTEGER NOT NULL,
  procs     INTEGER NOT NULL,
  PRIMARY KEY (node_id, ts)
) WITHOUT ROWID;

CREATE TABLE metrics_1h (
  node_id   INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  ts        INTEGER NOT NULL,
  cpu       REAL    NOT NULL,
  load1     REAL    NOT NULL,
  mem_used  INTEGER NOT NULL,
  swap_used INTEGER NOT NULL,
  disk_used INTEGER NOT NULL,
  net_in    INTEGER NOT NULL,
  net_out   INTEGER NOT NULL,
  tcp       INTEGER NOT NULL,
  udp       INTEGER NOT NULL,
  procs     INTEGER NOT NULL,
  PRIMARY KEY (node_id, ts)
) WITHOUT ROWID;

CREATE TABLE ping_1m (
  node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target  TEXT    NOT NULL,
  ts      INTEGER NOT NULL,
  avg_ms  REAL    NOT NULL,
  loss    REAL    NOT NULL,
  PRIMARY KEY (node_id, target, ts)
) WITHOUT ROWID;

CREATE TABLE notify_channels (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  type       TEXT    NOT NULL,
  config     TEXT    NOT NULL,
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE TABLE alert_rules (
  id         INTEGER PRIMARY KEY,
  name       TEXT    NOT NULL,
  kind       TEXT    NOT NULL,
  threshold  REAL    NOT NULL DEFAULT 0,
  duration   INTEGER NOT NULL DEFAULT 0,
  node_ids   TEXT    NOT NULL DEFAULT '[]',
  enabled    INTEGER NOT NULL DEFAULT 1,
  created_at INTEGER NOT NULL
);

CREATE TABLE alert_state (
  rule_id  INTEGER NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  node_id  INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  ref      INTEGER NOT NULL DEFAULT 0,
  fired_at INTEGER NOT NULL,
  PRIMARY KEY (rule_id, node_id)
);

CREATE TABLE alert_events (
  id         INTEGER PRIMARY KEY,
  rule_id    INTEGER NOT NULL,
  node_id    INTEGER NOT NULL,
  kind       TEXT    NOT NULL,
  firing     INTEGER NOT NULL,
  message    TEXT    NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE INDEX alert_events_created_at ON alert_events (created_at);
