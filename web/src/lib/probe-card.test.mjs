import assert from "node:assert/strict";
import test from "node:test";
import { probeCard } from "./probe-card.ts";

const end = 1_800_000_000;
const point = (t, samples = 2, successes = 2, avg_ms = 100) => ({ t, samples, successes, avg_ms });
const probe = (history = []) => ({
  target: "CT",
  kind: "icmp",
  window_end: end,
  median_ms: 99,
  fresh: true,
  latency_fresh: true,
  avg_ms: 100,
  samples: 24,
  successes: 24,
  history,
});

test("twelve full minute cells use server time despite three minutes of browser clock skew", () => {
  const history = Array.from({ length: 12 }, (_, i) => point(end - (12 - i) * 60));
  const result = probeCard(probe(history), end + 180);
  assert.equal(result.buckets.length, 12);
  assert.equal(result.buckets[0].t, end - 720);
  assert.equal(result.buckets.at(-1).t, end - 60);
  assert.ok(result.buckets.every((b) => b.samples === 2));
  assert.equal(result.samples, 24);
  assert.equal(result.latency, 99);
  assert.equal(result.failure, 0);
});

test("persisted minutes fill restart gaps without double-counting live records", () => {
  const history = [
    {
      name: "CT",
      kind: "icmp",
      points: Array.from({ length: 12 }, (_, i) => ({
        t: end - (12 - i) * 60,
        ms: 200,
        samples: 2,
        successes: 2,
        loss: 0,
      })),
    },
  ];
  const result = probeCard(probe([point(end - 60, 1, 1, 100)]), end + 180, history);
  assert.ok(result.buckets.every((b) => b.samples > 0));
  assert.equal(result.samples, 23);
  assert.equal(result.successes, 23);
  assert.equal(result.latency, 4500 / 23);
  assert.equal(result.isMedian, false);
});

test("missing minutes, failed attempts and unknown legacy counts remain distinct", () => {
  const p = probe([point(end - 120, 2, 0, -1), point(end, 3, 3, 1)]);
  const history = [
    { name: "CT", kind: "icmp", points: [{ t: end - 180, ms: 10, samples: null, successes: null, loss: 0 }] },
  ];
  const result = probeCard(p, end + 180, history);
  assert.equal(result.samples, 2);
  assert.equal(result.latency, -1);
  assert.equal(result.failure, 100);
  assert.equal(result.buckets.at(-1).samples, 0);
  assert.equal(result.buckets.at(-3).samples, 0);
  assert.equal(result.buckets.at(-2).successes, 0);
});

test("same named TCP history cannot fill ICMP cells", () => {
  const result = probeCard(probe(), end, [
    { name: "CT", kind: "tcp", points: [{ t: end - 60, ms: 1, samples: 2, successes: 2, loss: 0 }] },
  ]);
  assert.equal(result.samples, 0);
  assert.equal(result.failure, null);
});
