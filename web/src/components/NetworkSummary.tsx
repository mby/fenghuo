import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { api } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { probeCard } from "@/lib/probe-card";
import type { NetworkProbe, PingSeries } from "@/lib/types";
import { cn } from "@/lib/utils";

const ms = (value: number | undefined) => (value == null || value < 0 ? "—" : `${Math.round(value)} ms`);
const pct = (value: number) => `${Number(value.toFixed(1))}%`;

export function NetworkSummary({ uuid, probes, now }: { uuid: string; probes: NetworkProbe[]; now: number }) {
  const { t } = useI18n();
  const [selected, setSelected] = useState<"icmp" | "tcp">("icmp");
  const saved = useQuery({
    queryKey: ["probe-card", uuid, "12m"],
    queryFn: () => api<PingSeries[]>(`/api/v1/nodes/${encodeURIComponent(uuid)}/ping?range=12m`),
    enabled: probes.some((p) => p.fresh),
    staleTime: 60_000,
    refetchInterval: 60_000,
    retry: 1,
  });
  const available = (["icmp", "tcp"] as const).filter((kind) =>
    probes.some((p) => p.fresh && (p.kind ?? "tcp") === kind),
  );
  if (available.length === 0) return null;
  const kind = available.includes(selected) ? selected : available[0];
  const visible = probes.filter((p) => p.fresh && (p.kind ?? "tcp") === kind).slice(0, 3);
  return (
    <section className="relative z-10 grid gap-3 border-t pt-3 text-xs" aria-label={t("network.probesShort")}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="text-muted-foreground">{t("network.window")}</span>
        <fieldset className="flex items-center gap-1" aria-label={t("network.protocol")}>
          {available.map((value) => (
            <button
              key={value}
              type="button"
              aria-pressed={kind === value}
              onClick={() => setSelected(value)}
              className={cn(
                "min-h-8 rounded-md px-2.5 text-xs font-medium focus-visible:outline-2 focus-visible:outline-ring",
                kind === value ? "bg-secondary text-secondary-foreground" : "text-muted-foreground hover:bg-muted",
              )}
            >
              {t(value === "icmp" ? "network.icmp" : "network.tcp")}
            </button>
          ))}
        </fieldset>
      </div>
      <div className="grid gap-3">
        {visible.map((probe) => {
          const summary = probeCard(probe, now, saved.data);
          const { latency, buckets } = summary;
          const known = summary.samples > 0 && !probe.error;
          const failure = summary.failure ?? 0;
          const label = t(kind === "icmp" ? "network.loss" : "network.failShort");
          return (
            <Tooltip key={probe.target}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="grid w-full gap-1.5 rounded text-left tabular-nums focus-visible:outline-2 focus-visible:outline-ring"
                  aria-label={`${probe.target}, ${t(kind === "icmp" ? "network.icmp" : "network.tcp")}, ${ms(latency)}, ${label} ${known ? pct(failure) : t("network.unknownSamples")}`}
                >
                  <span className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-x-3 gap-y-1">
                    <span className="min-w-0 break-words font-medium [overflow-wrap:anywhere]">{probe.target}</span>
                    <span className="flex items-baseline gap-4 whitespace-nowrap">
                      <span className={cn(latency < 0 ? "text-muted-foreground" : "text-foreground")}>
                        {ms(latency)}
                      </span>
                      <span
                        className={cn(
                          "min-w-12 text-right",
                          known && failure > 0 ? "text-ember" : "text-muted-foreground",
                        )}
                      >
                        {probe.error ? t(`network.error.${probe.error}`) : known ? pct(failure) : "—"}
                      </span>
                    </span>
                  </span>
                  <span className="grid grid-cols-2 gap-4" aria-hidden>
                    {(["latency", "failure"] as const).map((metric) => (
                      <span
                        key={metric}
                        className="grid h-2 w-full grid-cols-12 items-end gap-0.5"
                        data-probe-strip={metric}
                      >
                        {buckets.map((bucket) => {
                          const valid = bucket && bucket.samples > 0;
                          const failed = valid && bucket.successes < bucket.samples;
                          return (
                            <span
                              key={bucket.t}
                              style={{
                                height:
                                  metric === "latency" && valid && bucket.successes > 0
                                    ? `${Math.max(3, Math.min(8, bucket.avg_ms / 50))}px`
                                    : "8px",
                              }}
                              title={
                                valid
                                  ? t("network.bucketCounts", {
                                      ms: ms(bucket.avg_ms),
                                      received: bucket.successes,
                                      sent: bucket.samples,
                                    })
                                  : t("network.unknownSamples")
                              }
                              className={cn(
                                "min-w-0 w-full rounded-[2px]",
                                !valid
                                  ? "bg-muted"
                                  : metric === "failure" && failed
                                    ? "bg-ember"
                                    : metric === "latency" && bucket.successes === 0
                                      ? "bg-ember/60"
                                      : "bg-foreground/30",
                              )}
                            />
                          );
                        })}
                      </span>
                    ))}
                  </span>
                </button>
              </TooltipTrigger>
              <TooltipContent className="max-w-72 space-y-1 text-left leading-relaxed">
                <p className="font-medium">
                  {probe.target} · {t(kind === "icmp" ? "network.icmp" : "network.tcp")}
                </p>
                <p>
                  {t(summary.isMedian ? "network.median" : "network.averageLatency")}: {ms(latency)}
                </p>
                <p>{t("network.sampleCounts", { received: summary.successes, sent: summary.samples })}</p>
                <p>{t("network.minuteCells")}</p>
                <p>{t(kind === "icmp" ? "network.icmpHint" : "network.tcpHint")}</p>
                {(probe.errors ?? 0) > 0 && <p>{t("network.localErrors", { n: probe.errors ?? 0 })}</p>}
              </TooltipContent>
            </Tooltip>
          );
        })}
      </div>
      <div className="grid grid-cols-2 gap-4 text-[10px] text-muted-foreground">
        <span>{t("network.latencyShort")}</span>
        <span>{t(kind === "icmp" ? "network.loss" : "network.failShort")}</span>
      </div>
    </section>
  );
}
