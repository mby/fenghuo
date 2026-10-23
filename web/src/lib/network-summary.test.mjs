import assert from "node:assert/strict";
import test from "node:test";
import { networkOverview } from "./network-summary.ts";

const now = 1_800_000_000;
const probe = (target, samples, successes, avg_ms, history = []) => ({
  target,
  samples,
  successes,
  avg_ms,
  history,
  fresh: true,
  latency_fresh: successes > 0,
  failure_rate: ((samples - successes) / samples) * 100,
});

test("card averages weight actual attempts and successful connections separately", () => {
  const result = networkOverview([probe("CT", 10, 9, 20), probe("CU", 2, 1, 100)], now);
  assert.equal(result.latency, 28);
  assert.ok(Math.abs(result.failure - 100 / 6) < 1e-10);
  assert.equal(result.samples, 12);
});

test("card history aligns buckets, leaves gaps unknown and preserves all-failed attempts", () => {
  const result = networkOverview(
    [
      probe("CT", 3, 2, 40, [
        { t: now - 30, samples: 2, successes: 2, avg_ms: 40 },
        { t: now, samples: 1, successes: 0, avg_ms: -1 },
      ]),
      probe("CU", 1, 1, 100, [{ t: now - 30, samples: 1, successes: 1, avg_ms: 100 }]),
    ],
    now,
  );
  assert.equal(result.history.length, 11);
  assert.deepEqual(result.history[0], { t: now - 300, latency: null, failure: null, samples: 0, targets: 0 });
  assert.equal(result.history.at(-2).latency, 60);
  assert.equal(result.history.at(-2).targets, 2);
  assert.equal(result.history.at(-1).latency, null);
  assert.equal(result.history.at(-1).failure, 100);
});

test("only the first three fresh targets contribute; old API data stays unknown", () => {
  const stale = { ...probe("stale", 20, 20, 999), fresh: false };
  const result = networkOverview(
    [stale, probe("A", 1, 1, 10), probe("B", 1, 1, 20), probe("C", 1, 1, 30), probe("D", 1, 0, -1)],
    now,
  );
  assert.deepEqual(
    result.probes.map((p) => p.target),
    ["A", "B", "C"],
  );
  assert.equal(result.latency, 20);
  assert.equal(result.failure, 0);
  const old = networkOverview([{ target: "old", avg_ms: 20, failure_rate: 0, fresh: true, latency_fresh: true }], now);
  assert.equal(old.latency, null);
  assert.equal(old.failure, null);
  assert.equal(networkOverview([], now).samples, 0);
  assert.equal(networkOverview([{ ...probe("failures", 3, 1, 10), latency_fresh: false }], now).latency, null);
});
