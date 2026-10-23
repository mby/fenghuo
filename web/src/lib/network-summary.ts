import type { NetworkBucket, NetworkProbe } from "./types";

function average(buckets: Pick<NetworkBucket, "avg_ms" | "samples" | "successes">[]) {
  const samples = buckets.reduce((sum, b) => sum + b.samples, 0);
  const successes = buckets.reduce((sum, b) => sum + b.successes, 0);
  const total = buckets.reduce((sum, b) => sum + (b.successes > 0 && b.avg_ms >= 0 ? b.avg_ms * b.successes : 0), 0);
  return {
    latency: successes > 0 ? total / successes : null,
    failure: samples > 0 ? ((samples - successes) / samples) * 100 : null,
    samples,
  };
}

export function networkOverview(probes: NetworkProbe[], now: number) {
  const visible = probes.filter((p) => p.fresh).slice(0, 3);
  const current = visible
    .filter((p) => p.samples != null && p.successes != null)
    .map((p) => ({
      avg_ms: p.avg_ms,
      samples: p.samples as number,
      successes: p.successes as number,
    }));
  const summary = average(current);
  if (!visible.some((p) => p.latency_fresh)) summary.latency = null;
  const end = Math.floor(now / 30) * 30;
  const history = Array.from({ length: 11 }, (_, i) => {
    const t = end - (10 - i) * 30;
    const buckets = visible.flatMap((p) => p.history?.filter((b) => b.t === t) ?? []);
    return { t, ...average(buckets), targets: buckets.length };
  });
  return { probes: visible, ...summary, history };
}
