import assert from "node:assert/strict";
import test from "node:test";
import { demoMetrics, demoNodes, demoResponse } from "./demo.ts";

const now = 1_800_000_000;

test("demo scenarios cover healthy, stressed, offline and pending nodes", () => {
  const nodes = demoNodes(now);
  assert.equal(nodes.length, 9);
  assert.ok(nodes.some((node) => node.online && node.status.cpu >= 90));
  assert.ok(nodes.some((node) => !node.online && node.has_reported));
  assert.ok(nodes.some((node) => !node.online && !node.has_reported && node.status === null));
  assert.ok(nodes.some((node) => node.traffic.limit > 0 && node.traffic.used / node.traffic.limit >= 0.9));
  assert.ok(nodes.some((node) => !node.visibility.system && node.info.cpu_model === ""));
  for (const id of [
    "singapore-api",
    "hong-kong-db",
    "frankfurt-backup",
    "london-archive",
    "new-york-app",
    "sydney-pending",
  ]) {
    assert.equal(nodes.find((node) => node.uuid === id)?.public_note, "", `${id} repeats its status in a note`);
  }
});

test("demo history has realistic timestamps and reporting gaps", () => {
  const points = demoMetrics("singapore-api", "24h", now);
  assert.ok(points.length > 50);
  assert.ok(points.every((point) => point.t <= now && point.cpu >= 0 && point.cpu <= 100));
  assert.ok(points.some((point, index) => index > 0 && point.t - points[index - 1].t > 600));
  assert.deepEqual(demoMetrics("frankfurt-backup", "live", now), []);
  assert.deepEqual(demoMetrics("sydney-pending", "24h", now), []);
});

test("demo responses use the same public API shapes", () => {
  const summary = demoResponse("/api/v1/nodes/frankfurt-backup/availability?range=7d", now);
  assert.ok(summary.offline_seconds > 0);
  assert.equal(summary.online_seconds + summary.offline_seconds + summary.unknown_seconds, 7 * 86400);
  assert.equal(demoResponse("/admin/api/nodes", now), undefined);
});
