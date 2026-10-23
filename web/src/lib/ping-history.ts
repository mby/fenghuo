import type { PingSeries } from "./types";

export type PingChartRow = { t: number } & Record<string, number | null>;

export function pingBucketSeconds(hours: number, minuteHours: number): number {
  // Match api.Server.plan so valid aggregate buckets are never mistaken for reporting gaps.
  const unit = hours <= minuteHours ? 60 : 3600;
  return Math.max(unit, Math.ceil(Math.floor((hours * 3600) / 360) / unit) * unit);
}

export function pingChartRows(series: PingSeries[], gapSeconds: number): PingChartRow[] {
  const byT = new Map<number, PingChartRow>();
  series.forEach((s, i) => {
    for (const p of s.points) {
      const row: PingChartRow = byT.get(p.t) ?? { t: p.t };
      row[`p${i}`] = p.ms >= 0 ? p.ms : null;
      row[`loss${i}`] = p.loss;
      byT.set(p.t, row);
    }
  });
  const sorted = [...byT.values()].sort((a, b) => a.t - b.t);
  return sorted.flatMap((row, i) => {
    const previous = sorted[i - 1];
    return previous && row.t - previous.t > gapSeconds ? [{ t: (previous.t + row.t) / 2 }, row] : [row];
  });
}
