ALTER TABLE ping_1m RENAME TO ping_1m_legacy;
ALTER TABLE ping_1h RENAME TO ping_1h_legacy;
CREATE TABLE ping_1m (
  node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'tcp' CHECK (kind IN ('tcp','icmp')),
  ts INTEGER NOT NULL,
  avg_ms REAL NOT NULL,
  loss REAL NOT NULL,
  samples INTEGER,
  successes INTEGER,
  sum_ms REAL,
  PRIMARY KEY (node_id,target,kind,ts)
) WITHOUT ROWID;
CREATE TABLE ping_1h (
  node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
  target TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT 'tcp' CHECK (kind IN ('tcp','icmp')),
  ts INTEGER NOT NULL,
  avg_ms REAL NOT NULL,
  loss REAL NOT NULL,
  samples INTEGER,
  successes INTEGER,
  sum_ms REAL,
  PRIMARY KEY (node_id,target,kind,ts)
) WITHOUT ROWID;
-- Published history has no counts. NULL keeps that limitation explicit.
INSERT INTO ping_1m(node_id,target,ts,avg_ms,loss) SELECT node_id,target,ts,avg_ms,loss FROM ping_1m_legacy;
INSERT INTO ping_1h(node_id,target,ts,avg_ms,loss) SELECT node_id,target,ts,avg_ms,loss FROM ping_1h_legacy;
DROP TABLE ping_1m_legacy;
DROP TABLE ping_1h_legacy;
