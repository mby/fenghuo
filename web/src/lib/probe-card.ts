import type { NetworkBucket, NetworkProbe, PingSeries } from "./types";

export const PROBE_CARD_MINUTES = 12;
export const PROBE_CARD_BUCKET_SECONDS = 60;

export function probeCard(probe: NetworkProbe, now: number, history: PingSeries[] = []) {
  // The browser clock may differ from the collector/server clock by minutes.
  const end = probe.window_end || Math.floor(now / 60) * 60;
  const start = end - PROBE_CARD_MINUTES * 60;
  const saved = new Map<number, NetworkBucket>();
  const live = new Map<number, NetworkBucket>();
  const series = history.find((s) => s.name === probe.target && (s.kind ?? "tcp") === (probe.kind ?? "tcp"));
  for (const point of series?.points ?? []) {
    if (point.t < start || point.t >= end || point.samples == null || point.successes == null || point.samples <= 0)
      continue;
    saved.set(point.t, { t: point.t, samples: point.samples, successes: point.successes, avg_ms: point.ms });
  }
  for (const point of probe.history ?? []) {
    const t = Math.floor(point.t / 60) * 60;
    if (t < start || t >= end || point.samples <= 0) continue;
    const prior = live.get(t);
    const successes = point.successes + (prior?.successes ?? 0);
    const total =
      (point.successes > 0 ? point.avg_ms * point.successes : 0) +
      (prior && prior.successes > 0 ? prior.avg_ms * prior.successes : 0);
    live.set(t, {
      t,
      samples: point.samples + (prior?.samples ?? 0),
      successes,
      avg_ms: successes ? total / successes : -1,
    });
  }
  let usesSaved = false;
  const buckets = Array.from({ length: PROBE_CARD_MINUTES }, (_, i) => {
    const t = start + i * 60;
    // Persisted and live records can cover the same attempts: never add them.
    const bucket = live.get(t) ?? saved.get(t);
    if (!live.has(t) && saved.has(t)) usesSaved = true;
    return {
      t,
      ...bucket,
      samples: bucket?.samples ?? 0,
      successes: bucket?.successes ?? 0,
      avg_ms: bucket?.avg_ms ?? -1,
    };
  });
  const samples = buckets.reduce((sum, b) => sum + b.samples, 0);
  const successes = buckets.reduce((sum, b) => sum + b.successes, 0);
  const total = buckets.reduce((sum, b) => sum + (b.successes > 0 ? b.avg_ms * b.successes : 0), 0);
  // Minute means cannot reconstruct a median. Label the weighted mean while
  // saved history fills a server-restart gap; use the true live median otherwise.
  const isMedian = !usesSaved && probe.kind === "icmp" && (probe.median_ms ?? -1) >= 0;
  return {
    buckets,
    samples,
    successes,
    isMedian,
    latency: successes > 0 && !probe.error ? (isMedian ? (probe.median_ms as number) : total / successes) : -1,
    failure: samples > 0 ? ((samples - successes) / samples) * 100 : null,
  };
}
