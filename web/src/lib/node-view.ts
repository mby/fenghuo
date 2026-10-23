import type { Metric, PublicNode } from "./types";

export const MAX_COMPARE = 4;
export const NODE_SORTS = ["default", "name", "cpu", "memory", "network", "traffic", "expiry", "status"] as const;
export type NodeSort = (typeof NODE_SORTS)[number];

export function isVisible(node: PublicNode, field: "system" | "expiry" | "traffic" | "ping"): boolean {
  return node.visibility?.[field] !== false;
}

export function finiteValue(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

export function matchesNode(node: PublicNode, search: string): boolean {
  const fields = [node.name, node.group, node.public_note, node.public_tags].map((value) =>
    (value ?? "").toLocaleLowerCase(),
  );
  return search
    .trim()
    .toLocaleLowerCase()
    .split(/\s+/)
    .every((term) => fields.some((field) => field.includes(term)));
}

export function publicTags(value: string | undefined): string[] {
  return [
    ...new Set(
      (value ?? "")
        .split(/[,，\n]/)
        .map((tag) => tag.trim())
        .filter(Boolean),
    ),
  ].slice(0, 6);
}

/** Cached totals describe the last observed configuration, never live resource usage. */
export function nodeCapacity(node: PublicNode): { cores: number | null; memory: number | null; disk: number | null } {
  const positive = (value: unknown) => {
    const n = finiteValue(value);
    return n !== null && n > 0 ? n : null;
  };
  if (!isVisible(node, "system")) return { cores: null, memory: null, disk: null };
  return {
    cores: positive(node.info.cpu_cores),
    memory: positive(node.status?.mem_total) ?? positive(node.info.mem_total),
    disk: positive(node.status?.disk_total) ?? positive(node.info.disk_total),
  };
}

/** Familiar VPS shorthand; omit unknown values rather than advertising zero capacity. */
export function compactCapacity(node: PublicNode): string {
  const { cores, memory } = nodeCapacity(node);
  const unit = memory && memory < 1024 ** 3 ? 1024 ** 2 : 1024 ** 3;
  return `${cores ? `${cores}C` : ""}${memory ? `${Number((memory / unit).toFixed(1))}${unit === 1024 ** 2 ? "M" : "G"}` : ""}`;
}

function sortValue(node: PublicNode, sort: NodeSort): number | null {
  const status = node.online ? node.status : null;
  switch (sort) {
    case "cpu":
      return finiteValue(status?.cpu);
    case "memory": {
      const used = finiteValue(status?.mem_used);
      const total = finiteValue(status?.mem_total);
      return used !== null && total ? used / total : null;
    }
    case "network": {
      const incoming = finiteValue(status?.net_in);
      const outgoing = finiteValue(status?.net_out);
      return incoming !== null && outgoing !== null ? incoming + outgoing : null;
    }
    case "traffic":
      return isVisible(node, "traffic") ? finiteValue(node.traffic?.used) : null;
    case "expiry":
      return isVisible(node, "expiry") && node.expire_at > 0 ? finiteValue(node.expire_at) : null;
    case "status":
      return node.online ? 2 : !node.has_reported ? 1 : 0;
    default:
      return null;
  }
}

/** Unknown and redacted values stay last in either direction; never sort them as zero. */
export function sortNodes(nodes: readonly PublicNode[], sort: NodeSort, reverse = false): PublicNode[] {
  if (sort === "default") return [...nodes];
  const direction = (sort === "expiry" || sort === "status" || sort === "name" ? 1 : -1) * (reverse ? -1 : 1);
  return [...nodes].sort((a, b) => {
    const name = a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: "base" });
    if (sort === "name") return name * direction || a.uuid.localeCompare(b.uuid);
    const av = sortValue(a, sort);
    const bv = sortValue(b, sort);
    if (av === null || bv === null) return av === bv ? name : av === null ? 1 : -1;
    return (av - bv) * direction || name || a.uuid.localeCompare(b.uuid);
  });
}

export function parseCompareIds(value: string | null): string[] {
  return [
    ...new Set(
      (value ?? "")
        .split(",")
        .map((id) => id.trim())
        .filter((id) => /^[a-zA-Z0-9_-]{1,128}$/.test(id)),
    ),
  ].slice(0, MAX_COMPARE);
}

export function compareHref(ids: readonly string[]): string {
  const selected = parseCompareIds(ids.join(","));
  return `/compare${selected.length ? `?${new URLSearchParams({ ids: selected.join(",") })}` : ""}`;
}

/** Only the current viewer's node response can authorize a requested comparison. */
export function visibleCompareNodes(ids: readonly string[], nodes: readonly PublicNode[]): PublicNode[] {
  const available = new Map(nodes.map((node) => [node.uuid, node]));
  return parseCompareIds(ids.join(",")).flatMap((id) => {
    const node = available.get(id);
    return node ? [node] : [];
  });
}

export type CompareMetric = "cpu" | "mem_used" | "net_in" | "net_out";
export type ComparisonRow = { t: number } & Record<string, number | null>;

/** Outer join on actual timestamps: absent samples remain null, never index-paired or zero-filled. */
export function alignMetrics(series: readonly (readonly Metric[])[], metric: CompareMetric): ComparisonRow[] {
  const rows = new Map<number, ComparisonRow>();
  series.forEach((points, index) => {
    for (const point of points) {
      if (!Number.isFinite(point.t) || point.t <= 0) continue;
      const row =
        rows.get(point.t) ??
        Object.assign({ t: point.t }, Object.fromEntries(series.map((_, i) => [`node${i}`, null])));
      row[`node${index}`] = finiteValue(point[metric]);
      rows.set(point.t, row);
    }
  });
  return [...rows.values()].sort((a, b) => a.t - b.t);
}

/** Each line retains its own timestamps on the shared axis; long reporting gaps break the line. */
export function metricLine(
  points: readonly Metric[],
  metric: CompareMetric,
  gapSeconds: number,
): { t: number; value: number | null }[] {
  const sorted = alignMetrics([points], metric);
  return sorted.flatMap((point, index) => {
    const previous = sorted[index - 1];
    const current = { t: point.t, value: point.node0 };
    return previous && point.t - previous.t > gapSeconds
      ? [{ t: previous.t + (point.t - previous.t) / 2, value: null }, current]
      : [current];
  });
}

/** Shared chart rows keep Recharts' hover index synchronized. Only drawing vertices are interpolated;
 * tooltips still read alignMetrics' exact samples. Never extrapolate or draw through a reporting gap. */
export function comparisonChartRows(
  series: readonly (readonly Metric[])[],
  metric: CompareMetric,
  gapSeconds: number,
): ComparisonRow[] {
  const lines = series.map((points) => metricLine(points, metric, gapSeconds));
  const times = [...new Set(lines.flatMap((line) => line.map((point) => point.t)))].sort((a, b) => a - b);
  const positions = lines.map(() => 0);
  return times.map((time) => {
    const row: ComparisonRow = { t: time };
    lines.forEach((line, index) => {
      while (positions[index] < line.length && line[positions[index]].t < time) positions[index]++;
      const next = line[positions[index]];
      const previous = line[positions[index] - 1];
      row[`node${index}`] =
        next?.t === time
          ? next.value
          : previous?.value != null && next?.value != null
            ? previous.value + ((next.value - previous.value) * (time - previous.t)) / (next.t - previous.t)
            : null;
    });
    return row;
  });
}
