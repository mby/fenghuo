import assert from "node:assert/strict";
import test from "node:test";
import {
  alignMetrics,
  compactCapacity,
  compareHref,
  isVisible,
  matchesNode,
  metricLine,
  nodeCapacity,
  parseCompareIds,
  publicTags,
  sortNodes,
  visibleCompareNodes,
} from "./node-view.ts";

function node(uuid, overrides = {}) {
  return {
    uuid,
    has_reported: true,
    name: uuid,
    group: "Europe",
    public_note: "Public edge cache",
    online: true,
    last_seen: 100,
    expire_at: 0,
    info: { agent_version: "1", os: "Linux", cpu_cores: 2 },
    status: { cpu: 10, mem_used: 50, mem_total: 100, net_in: 5, net_out: 10 },
    traffic: { used: 100, limit: 0 },
    visibility: { system: true, expiry: true, traffic: true, ping: true },
    ...overrides,
  };
}
const ids = (nodes) => nodes.map((n) => n.uuid);
const sample = (t, cpu) => ({ t, cpu, mem_used: cpu * 1024, net_in: cpu, net_out: cpu * 2 });

test("search combines terms across public fields and never searches private fields", () => {
  const server = node("cache-1", {
    name: "TOKYO 01",
    group: "亚洲",
    public_note: "Public edge",
    public_tags: "生产环境, CDN",
    tags: "private-asset-tag",
    note: "secret",
    last_ip: "192.0.2.1",
    token: "private-token",
  });
  for (const [query, expected] of [
    ["tokyo 亚洲", true],
    [" EDGE tokyo ", true],
    ["CDN 生产环境", true],
    ["private-asset-tag", false],
    ["", true],
    ["secret", false],
    ["192.0.2.1", false],
    ["private-token", false],
    ["tokyo absent", false],
  ]) {
    assert.equal(matchesNode(server, query), expected, query);
  }
  assert.equal(
    matchesNode(node("empty", { name: undefined, group: undefined, public_note: undefined }), "missing"),
    false,
  );
});

test("capacity uses report totals, survives restart from cached info, and respects visibility", () => {
  const base = node("capacity", { info: { cpu_cores: 4, mem_total: 8000, disk_total: 100000 } });
  assert.deepEqual(nodeCapacity(base), { cores: 4, memory: 100, disk: 100000 });
  assert.deepEqual(nodeCapacity({ ...base, online: false, status: null }), { cores: 4, memory: 8000, disk: 100000 });
  assert.deepEqual(nodeCapacity({ ...base, visibility: { system: false } }), { cores: null, memory: null, disk: null });
  assert.deepEqual(nodeCapacity(node("new", { status: null, info: { cpu_cores: 0, mem_total: 0, disk_total: 0 } })), {
    cores: null,
    memory: null,
    disk: null,
  });
  assert.equal(nodeCapacity({ ...base, status: { mem_total: Number.NaN, disk_total: -1 } }).memory, 8000);
});

test("public tags trim separators, deduplicate and bound the display without reading private tags", () => {
  assert.deepEqual(publicTags(" 生产， CDN,生产\n备份, "), ["生产", "CDN", "备份"]);
  assert.deepEqual(publicTags(undefined), []);
  assert.equal(publicTags("a,b,c,d,e,f,g").length, 6);
});

test("compact VPS configuration shows cores and memory, omitting disk and unknown capacities", () => {
  for (const [cores, memory, expected] of [
    [2, 4 * 1024 ** 3, "2C4G"],
    [4, 512 * 1024 ** 2, "4C512M"],
    [0, 0, ""],
    [2, 0, "2C"],
    [0, 1536 * 1024 ** 2, "1.5G"],
  ]) {
    assert.equal(
      compactCapacity(
        node("compact", { status: null, info: { cpu_cores: cores, mem_total: memory, disk_total: 500 * 1024 ** 3 } }),
      ),
      expected,
    );
  }
  assert.equal(compactCapacity(node("hidden", { visibility: { system: false } })), "");
});

test("default order is preserved and name order is natural without mutating the input", () => {
  const nodes = [node("b", { name: "Server 10" }), node("a", { name: "Server 2" })];
  assert.deepEqual(ids(sortNodes(nodes, "default")), ["b", "a"]);
  assert.deepEqual(ids(sortNodes(nodes, "name")), ["a", "b"]);
  assert.deepEqual(ids(sortNodes(nodes, "name", true)), ["b", "a"]);
  assert.deepEqual(ids(nodes), ["b", "a"]);
});

test("CPU, memory percentage and total network rate sort descending, missing/offline last", () => {
  const slow = node("slow");
  const busy = node("busy", { status: { cpu: 80, mem_used: 80, mem_total: 100, net_in: 30, net_out: 1 } });
  const missing = node("missing", { status: null });
  const offline = node("offline", { online: false });
  for (const sort of ["cpu", "memory", "network"]) {
    assert.deepEqual(
      ids(sortNodes([slow, missing, busy, offline], sort)),
      ["busy", "slow", "missing", "offline"],
      sort,
    );
    assert.deepEqual(
      ids(sortNodes([slow, missing, busy, offline], sort, true)),
      ["slow", "busy", "missing", "offline"],
      sort,
    );
  }
  assert.equal(
    ids(sortNodes([node("redacted", { status: { mem_used: 0, mem_total: 0 } }), slow], "memory"))[1],
    "redacted",
  );
});

test("zero and invalid metrics are different; NaN and infinity are sorted last", () => {
  const nodes = [
    node("nan", { status: { cpu: Number.NaN } }),
    node("zero", { status: { cpu: 0 } }),
    node("infinite", { status: { cpu: Infinity } }),
    node("normal"),
  ];
  assert.deepEqual(ids(sortNodes(nodes, "cpu")), ["normal", "zero", "infinite", "nan"]);
  assert.deepEqual(ids(sortNodes(nodes, "cpu", true)), ["zero", "normal", "infinite", "nan"]);
});

test("traffic visibility overrides values while live network remains sortable", () => {
  const hidden = node("hidden", { traffic: { used: 99999 }, visibility: { traffic: false } });
  const visible = node("visible", { traffic: { used: 0 } });
  for (const reverse of [false, true])
    assert.deepEqual(ids(sortNodes([hidden, visible], "traffic", reverse)), ["visible", "hidden"]);
  assert.equal(isVisible(hidden, "traffic"), false);
  assert.deepEqual(ids(sortNodes([hidden, visible], "network")), ["hidden", "visible"]);
});

test("expiry is soonest first with zero and hidden values last in either direction", () => {
  const nodes = [
    node("none"),
    node("late", { expire_at: 200 }),
    node("soon", { expire_at: 100 }),
    node("hidden", { expire_at: 50, visibility: { expiry: false } }),
  ];
  assert.deepEqual(ids(sortNodes(nodes, "expiry")), ["soon", "late", "hidden", "none"]);
  assert.deepEqual(ids(sortNodes(nodes, "expiry", true)), ["late", "soon", "hidden", "none"]);
});

test("status prioritizes offline, then pending, then online", () => {
  const pending = node("pending", {
    has_reported: false,
    online: false,
    status: null,
    last_seen: 0,
    info: { agent_version: "" },
  });
  assert.deepEqual(ids(sortNodes([node("online"), pending, node("offline", { online: false })], "status")), [
    "offline",
    "pending",
    "online",
  ]);
});

test("redacted known nodes remain offline after panel restart", () => {
  const known = node("known", {
    has_reported: true,
    online: false,
    status: null,
    last_seen: 0,
    info: { agent_version: "" },
  });
  const pending = node("waiting", {
    has_reported: false,
    online: false,
    status: null,
    last_seen: 0,
    info: { agent_version: "" },
  });
  assert.deepEqual(ids(sortNodes([pending, known], "status")), ["known", "waiting"]);
});

test("share URLs round trip IDs, reject paths, remove duplicates and enforce four nodes", () => {
  assert.deepEqual(parseCompareIds(" a,a, ../secret,,b,c,d,e"), ["a", "b", "c", "d"]);
  assert.deepEqual(parseCompareIds(null), []);
  assert.deepEqual(parseCompareIds("a/metrics,<script>,%2f"), []);
  const href = compareHref(["uuid-1", "uuid_2"]);
  assert.deepEqual(parseCompareIds(new URL(href, "http://localhost").searchParams.get("ids")), ["uuid-1", "uuid_2"]);
  assert.equal(compareHref([]), "/compare");
});

test("requested hidden, nonexistent and revoked IDs do not produce authorized nodes", () => {
  assert.deepEqual(ids(visibleCompareNodes(["hidden", "b", "a", "absent"], [node("a"), node("b")])), ["b", "a"]);
  assert.deepEqual(visibleCompareNodes(["a"], []), []);
});

test("comparison aligns by actual timestamps, keeping zeros and gaps distinct", () => {
  assert.deepEqual(
    alignMetrics(
      [
        [sample(103, 10), sample(100, 0)],
        [sample(101, 20), sample(103, 30)],
      ],
      "cpu",
    ),
    [
      { t: 100, node0: 0, node1: null },
      { t: 101, node0: null, node1: 20 },
      { t: 103, node0: 10, node1: 30 },
    ],
  );
  assert.deepEqual(alignMetrics([[], [sample(100, Number.NaN)]], "cpu"), [{ t: 100, node0: null, node1: null }]);
  assert.deepEqual(alignMetrics([[sample(Number.NaN, 10), sample(-1, 10)]], "cpu"), []);
});

test("each metric uses its own values, not another node's array indices", () => {
  for (const [metric, expected] of [
    ["mem_used", 2048],
    ["net_in", 2],
    ["net_out", 4],
  ]) {
    assert.deepEqual(alignMetrics([[], [sample(100, 2)]], metric), [{ t: 100, node0: null, node1: expected }]);
  }
});

test("line data retains asynchronous sample times and breaks long reporting gaps", () => {
  assert.deepEqual(metricLine([sample(100, 0), sample(103, 10), sample(500, 20)], "cpu", 180), [
    { t: 100, value: 0 },
    { t: 103, value: 10 },
    { t: 301.5, value: null },
    { t: 500, value: 20 },
  ]);
  assert.deepEqual(metricLine([sample(101, 3), sample(104, 4)], "cpu", 180), [
    { t: 101, value: 3 },
    { t: 104, value: 4 },
  ]);
});

test("chart vertices share timestamps without inventing hover samples or extending series", async () => {
  const { comparisonChartRows } = await import("./node-view.ts");
  const data = [
    [sample(100, 0), sample(104, 40)],
    [sample(102, 20), sample(103, 30)],
  ];
  assert.deepEqual(comparisonChartRows(data, "cpu", 180), [
    { t: 100, node0: 0, node1: null },
    { t: 102, node0: 20, node1: 20 },
    { t: 103, node0: 30, node1: 30 },
    { t: 104, node0: 40, node1: null },
  ]);
  assert.equal(alignMetrics(data, "cpu").find((row) => row.t === 102).node0, null);
});

test("shared chart rows break gaps and keep another continuous series intact", async () => {
  const { comparisonChartRows } = await import("./node-view.ts");
  const rows = comparisonChartRows(
    [
      [sample(100, 10), sample(500, 20)],
      [sample(250, 5), sample(350, 15)],
    ],
    "cpu",
    180,
  );
  assert.deepEqual(
    rows.find((row) => row.t === 300),
    { t: 300, node0: null, node1: 10 },
  );
  assert.equal(rows.find((row) => row.t === 250).node0, null);
  assert.equal(rows.find((row) => row.t === 350).node0, null);
  assert.deepEqual(comparisonChartRows([[], []], "cpu", 180), []);
});
