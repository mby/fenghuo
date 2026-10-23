CREATE TABLE notification_deliveries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  channel_id INTEGER NOT NULL,
  payload TEXT NOT NULL,
  kind TEXT NOT NULL,
  firing INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'pending',
  attempts INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 5,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  next_attempt_at INTEGER NOT NULL,
  lease_until INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NOT NULL DEFAULT ''
);

CREATE INDEX notification_deliveries_due ON notification_deliveries(status, next_attempt_at);
CREATE INDEX notification_deliveries_channel ON notification_deliveries(channel_id, id);

CREATE TABLE notification_attempts (
  delivery_id INTEGER NOT NULL REFERENCES notification_deliveries(id) ON DELETE CASCADE,
  number INTEGER NOT NULL,
  started_at INTEGER NOT NULL,
  finished_at INTEGER NOT NULL DEFAULT 0,
  status TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (delivery_id, number)
);

-- Cancel in the same transaction so disabling and re-enabling, or reusing a
-- deleted channel ID, cannot resurrect a queued message.
CREATE TRIGGER notification_channel_deleted BEFORE DELETE ON notify_channels
BEGIN
  UPDATE notification_attempts SET status = 'canceled', error = 'channel_unavailable',
    finished_at = unixepoch()
    WHERE status = 'sending' AND delivery_id IN
      (SELECT id FROM notification_deliveries WHERE channel_id = OLD.id);
  UPDATE notification_deliveries SET status = 'canceled', last_error = 'channel_unavailable',
    updated_at = unixepoch(), next_attempt_at = 0, lease_until = 0, payload = ''
    WHERE channel_id = OLD.id AND status IN ('pending', 'retrying', 'sending');
END;

CREATE TRIGGER notification_channel_disabled AFTER UPDATE OF enabled ON notify_channels
WHEN NEW.enabled = 0
BEGIN
  UPDATE notification_attempts SET status = 'canceled', error = 'channel_unavailable',
    finished_at = unixepoch()
    WHERE status = 'sending' AND delivery_id IN
      (SELECT id FROM notification_deliveries WHERE channel_id = NEW.id);
  UPDATE notification_deliveries SET status = 'canceled', last_error = 'channel_unavailable',
    updated_at = unixepoch(), next_attempt_at = 0, lease_until = 0, payload = ''
    WHERE channel_id = NEW.id AND status IN ('pending', 'retrying', 'sending');
END;
