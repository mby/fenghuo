import assert from "node:assert/strict";
import test from "node:test";
import { demoResponse } from "./demo.ts";
import { pingBucketSeconds, pingChartRows } from "./ping-history.ts";

const ranges = [
  ["1h", 1, 60],
  ["6h", 6, 60],
  ["24h", 24, 240],
  ["7d", 168, 1680],
  ["30d", 720, 7200],
  ["90d", 2160, 21600],
];

test("probe history bucket sizes match the server query plan and retention", () => {
  for (const [range, hours, step] of ranges) {
    assert.equal(pingBucketSeconds(hours, 168), step, range);
  }
  assert.equal(pingBucketSeconds(24, 6), 3600);
});

test("healthy demo probe history renders connected curves across every range", () => {
  const now = 1_800_000_037;
  for (const [range, hours, step] of ranges) {
    const series = demoResponse(`/api/v1/nodes/tokyo-edge/ping?range=${range}`, now);
    assert.equal(series.length, 6);
    for (const { points } of series) {
      assert.ok(points.length > 1 && points.length <= 360, range);
      assert.ok(points[0].t > now - hours * 3600 && points[0].t <= now - hours * 3600 + step, range);
      assert.ok(points.at(-1).t <= now && points.at(-1).t > now - step, range);
    }
    const rows = pingChartRows(series, pingBucketSeconds(hours, 168) * 1.5);
    assert.equal(rows.length, series[0].points.length, `${range}: valid intervals must not insert gaps`);
    assert.ok(
      rows.every((row) => series.every((_, i) => Number.isFinite(row[`p${i}`]))),
      range,
    );
  }
});

test("probe charts preserve unknown gaps and measured connection failures", () => {
  const series = [
    {
      name: "CT",
      points: [
        { t: 0, ms: 20, loss: 0 },
        { t: 60, ms: -1, loss: 100 },
        { t: 240, ms: 22, loss: 0 },
      ],
    },
    {
      name: "CU",
      points: [
        { t: 0, ms: 30, loss: 0 },
        { t: 240, ms: 32, loss: 0 },
      ],
    },
  ];
  const rows = pingChartRows(series, 90);
  assert.deepEqual(
    rows.map((row) => row.t),
    [0, 60, 150, 240],
  );
  assert.equal(rows[1].p0, null);
  assert.equal(rows[1].loss0, 100);
  assert.equal(rows[1].p1, undefined);
  assert.equal(rows[1].loss1, undefined);
  assert.deepEqual(rows[2], { t: 150 });
  assert.equal(rows[3].p0, 22);
  assert.deepEqual(pingChartRows([], 90), []);
});
