import { pingBucketSeconds } from "./ping-history.ts";
import type { AvailabilitySpan, AvailabilitySummary, Meta, Metric, PingSeries, PublicNode } from "./types";

const GIB = 1024 ** 3;
const TIB = 1024 ** 4;
const visibility = { system: true, expiry: true, traffic: true, ping: true };
const keepMinuteDays = 7;

type Scenario = {
  id: string;
  name: string;
  group: string;
  tags: string;
  note?: string;
  cpu: number;
  memory: number;
  disk: number;
  traffic: number;
  limit: number;
  expiresIn?: number;
  offline?: boolean;
  pending?: boolean;
  private?: boolean;
};

const scenarios: Scenario[] = [
  {
    id: "tokyo-edge",
    name: "Tokyo Edge · 01",
    group: "Asia Pacific",
    tags: "Edge, Production",
    note: "正常运行的边缘缓存",
    cpu: 18,
    memory: 36,
    disk: 42,
    traffic: 0.27,
    limit: TIB,
  },
  {
    id: "singapore-api",
    name: "Singapore API Gateway",
    group: "Asia Pacific",
    tags: "API, High load",
    cpu: 96,
    memory: 74,
    disk: 51,
    traffic: 0.52,
    limit: TIB,
  },
  {
    id: "hong-kong-db",
    name: "Hong Kong Primary Database",
    group: "Asia Pacific",
    tags: "Database, Critical",
    cpu: 49,
    memory: 94,
    disk: 92,
    traffic: 0.65,
    limit: 2 * TIB,
  },
  {
    id: "frankfurt-backup",
    name: "Frankfurt Backup",
    group: "Europe",
    tags: "Backup, Offline",
    cpu: 31,
    memory: 48,
    disk: 68,
    traffic: 0.4,
    limit: TIB,
    offline: true,
  },
  {
    id: "london-archive",
    name: "London Archive Storage",
    group: "Europe",
    tags: "Storage, Quota",
    cpu: 12,
    memory: 28,
    disk: 77,
    traffic: 0.96,
    limit: 500 * GIB,
  },
  {
    id: "new-york-app",
    name: "New York Application Server",
    group: "North America",
    tags: "Application, Renewal",
    cpu: 44,
    memory: 62,
    disk: 34,
    traffic: 0.31,
    limit: TIB,
    expiresIn: 1,
  },
  {
    id: "sao-paulo-worker",
    name: "São Paulo Background Worker With A Deliberately Long Display Name",
    group: "South America",
    tags: "Worker, Long name",
    note: "用于检查卡片、列表和图表中的长文本换行",
    cpu: 53,
    memory: 57,
    disk: 29,
    traffic: 0.18,
    limit: 0,
  },
  {
    id: "sydney-pending",
    name: "Sydney New Node",
    group: "Oceania",
    tags: "Pending",
    cpu: 0,
    memory: 0,
    disk: 0,
    traffic: 0,
    limit: 0,
    pending: true,
  },
  {
    id: "private-observer",
    name: "Restricted Public View",
    group: "Oceania",
    tags: "Privacy",
    note: "公开视图仅展示部分信息",
    cpu: 24,
    memory: 40,
    disk: 35,
    traffic: 0,
    limit: 0,
    private: true,
  },
];

export function demoNodes(now: number): PublicNode[] {
  return scenarios.map((s) => {
    const hidden = s.private === true;
    const memTotal = s.pending || hidden ? 0 : 8 * GIB;
    const diskTotal = s.pending || hidden ? 0 : (s.id === "hong-kong-db" ? 200 : 100) * GIB;
    const online = !s.pending && !s.offline;
    const reset = Math.floor(now / 86400) * 86400;
    return {
      uuid: s.id,
      name: s.name,
      group: s.group,
      public_tags: s.tags,
      public_note: s.note ?? "",
      has_reported: !s.pending,
      visibility: hidden ? { system: false, expiry: false, traffic: false, ping: false } : { ...visibility },
      expire_at: hidden || s.pending ? 0 : now + (s.expiresIn ?? 120) * 86400,
      online,
      network:
        online && !hidden && (s.id === "hong-kong-db" || s.id === "tokyo-edge")
          ? ["上海电信", "上海联通", "上海移动"].map((target, i) => {
              const history = Array.from({ length: 10 }, (_, j) => {
                const failure = s.id === "hong-kong-db" && i === 2 && j % 4 === 0;
                return {
                  t: Math.floor(now / 30) * 30 - (9 - j) * 30,
                  avg_ms: failure ? -1 : 130 + i * 18 + Math.sin(j / 2) * 35,
                  samples: 1,
                  successes: failure ? 0 : 1,
                };
              });
              const successes = history.reduce((sum, p) => sum + p.successes, 0);
              return {
                target,
                avg_ms: history.reduce((sum, p) => sum + (p.successes ? p.avg_ms : 0), 0) / successes,
                failure_rate: (10 - successes) * 10,
                fresh: true,
                latency_fresh: true,
                samples: 10,
                successes,
                history,
              };
            })
          : [],
      last_seen: s.pending ? 0 : now - (s.offline ? 4200 : 5),
      info: {
        mem_total: memTotal,
        disk_total: diskTotal,
        os: hidden || s.pending ? "" : "Debian 13",
        kernel: hidden || s.pending ? "" : "6.12",
        arch: hidden || s.pending ? "" : "amd64",
        cpu_model: hidden || s.pending ? "" : "AMD EPYC",
        cpu_cores: hidden || s.pending ? 0 : 4,
        virt: hidden || s.pending ? "" : "kvm",
        agent_version: hidden || s.pending ? "" : "demo",
      },
      status: s.pending
        ? null
        : {
            cpu: s.cpu,
            load1: +(s.cpu / 25).toFixed(2),
            load5: +(s.cpu / 28).toFixed(2),
            load15: +(s.cpu / 30).toFixed(2),
            mem_total: memTotal,
            mem_used: Math.round((memTotal * s.memory) / 100),
            swap_total: hidden ? 0 : GIB,
            swap_used: 0,
            disk_total: diskTotal,
            disk_used: Math.round((diskTotal * s.disk) / 100),
            net_in: 2_000_000,
            net_out: 800_000,
            tcp: 52,
            udp: 8,
            procs: 113,
            uptime: 864000,
          },
      traffic: {
        up: hidden ? 0 : Math.round(s.traffic * s.limit * 0.35),
        down: hidden ? 0 : Math.round(s.traffic * s.limit * 0.65),
        used: hidden ? 0 : Math.round(s.traffic * s.limit),
        limit: hidden ? 0 : s.limit,
        mode: "sum",
        reset_day: 1,
        period_start: reset,
        next_reset_at: reset + 30 * 86400,
        reset_utc_offset: 0,
      },
    };
  });
}

const ranges = {
  live: { seconds: 600, step: 3 },
  "1h": { seconds: 3600, step: 60 },
  "6h": { seconds: 21600, step: 300 },
  "24h": { seconds: 86400, step: 600 },
  "7d": { seconds: 604800, step: 3600 },
  "30d": { seconds: 2592000, step: 10800 },
  "90d": { seconds: 7776000, step: 21600 },
} as const;

type Range = keyof typeof ranges;

export function demoMetrics(id: string, range: Range, now: number): Metric[] {
  const index = scenarios.findIndex((s) => s.id === id);
  if (index < 0 || scenarios[index].pending || (scenarios[index].offline && range === "live")) return [];
  const s = scenarios[index];
  const { seconds, step } = ranges[range];
  const end = now - (s.offline ? 4200 : 0);
  const start = now - seconds;
  const out: Metric[] = [];
  for (let t = Math.ceil(start / step) * step; t <= end; t += step) {
    // Reporting gaps exercise chart breaks and comparison alignment.
    if (index % 3 === 1 && t > now - seconds * 0.48 && t < now - seconds * 0.38) continue;
    const phase = (t / step + index * 7) / 8;
    const wave = Math.sin(phase) * 8 + Math.sin(phase / 3) * 4;
    const cpu = Math.max(0, Math.min(100, s.cpu + wave));
    const memTotal = 8 * GIB;
    const diskTotal = (id === "hong-kong-db" ? 200 : 100) * GIB;
    out.push({
      t,
      cpu: +cpu.toFixed(1),
      load1: +(cpu / 25).toFixed(2),
      mem_used: Math.round((memTotal * Math.max(5, Math.min(98, s.memory + wave / 3))) / 100),
      swap_used: 0,
      disk_used: Math.round((diskTotal * s.disk) / 100),
      net_in: Math.round(1_500_000 + (wave + 12) * 85_000),
      net_out: Math.round(600_000 + (wave + 12) * 40_000),
      tcp: 45 + Math.round((wave + 12) * 2),
      udp: 8,
      procs: 110 + Math.round(wave),
    });
  }
  return out;
}

function demoPing(id: string, range: Range, now: number): PingSeries[] {
  const s = scenarios.find((item) => item.id === id);
  if (!s || s.pending || s.private) return [];
  const { seconds } = ranges[range];
  const step = pingBucketSeconds(seconds / 3600, keepMinuteDays * 24);
  const end = now - (s.offline ? 4200 : 0);
  return ["上海电信", "上海联通", "上海移动", "北京电信", "北京联通", "北京移动"].map((name, index) => ({
    name,
    points: Array.from({ length: Math.floor(seconds / step) }, (_, i) => {
      const t = Math.floor((now - seconds) / step) * step + (i + 1) * step;
      const failure = s.id === "hong-kong-db" && index === 1 && i % 11 === 0;
      return { t, ms: failure ? -1 : 18 + index * 26 + Math.abs(Math.sin(i / 9)) * 30, loss: failure ? 100 : 0 };
    }).filter((point) => point.t <= end),
  }));
}

function demoAvailability(id: string, range: "7d" | "30d" | "90d", now: number): AvailabilitySummary {
  const s = scenarios.find((item) => item.id === id);
  const from = now - ranges[range].seconds;
  const unknownEnd = from + Math.floor((now - from) * 0.08);
  const outageStart = now - (s?.offline ? 4200 : 86400 * 2);
  const outageEnd = s?.offline ? now : outageStart + 1800;
  const timeline: AvailabilitySpan[] =
    !s || s.pending
      ? [{ start: from, end: now, state: "unknown" }]
      : [
          { start: from, end: unknownEnd, state: "unknown" },
          { start: unknownEnd, end: outageStart, state: "online" },
          { start: outageStart, end: outageEnd, state: "offline" },
          ...(outageEnd < now ? [{ start: outageEnd, end: now, state: "online" as const }] : []),
        ];
  const unknown = timeline.filter((x) => x.state === "unknown").reduce((sum, x) => sum + x.end - x.start, 0);
  const offline = timeline.filter((x) => x.state === "offline").reduce((sum, x) => sum + x.end - x.start, 0);
  const online = now - from - unknown - offline;
  return {
    from,
    to: now,
    online_seconds: online,
    offline_seconds: offline,
    unknown_seconds: unknown,
    uptime_percent: online + offline > 0 ? (online / (online + offline)) * 100 : null,
    coverage_percent: ((online + offline) / (now - from)) * 100,
    outage_count: offline > 0 ? 1 : 0,
    longest_outage_seconds: offline,
    timeline,
    timeline_truncated: false,
  };
}

export function demoResponse(path: string, now = Math.floor(Date.now() / 1000)): unknown | undefined {
  const url = new URL(path, "http://localhost");
  if (url.pathname === "/api/v1/meta") {
    return {
      site_name: "Fenghuo · UI Demo",
      version: "demo",
      public_enabled: true,
      admin: false,
      keep_minute_days: keepMinuteDays,
      keep_hour_days: 90,
    } satisfies Meta;
  }
  if (url.pathname === "/api/v1/nodes") return demoNodes(now);
  const match = /^\/api\/v1\/nodes\/([^/]+)\/(metrics|ping|availability)$/.exec(url.pathname);
  if (!match) return undefined;
  const id = decodeURIComponent(match[1]);
  const range = url.searchParams.get("range") ?? "live";
  if (!scenarios.some((s) => s.id === id) || !(range in ranges)) return undefined;
  if (match[2] === "metrics") return demoMetrics(id, range as Range, now);
  if (match[2] === "ping") return demoPing(id, range as Range, now);
  if (match[2] === "availability" && (range === "7d" || range === "30d" || range === "90d")) {
    return demoAvailability(id, range, now);
  }
  return undefined;
}
