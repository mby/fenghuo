import { useState } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { clock } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
import { networkOverview } from "@/lib/network-summary";
import type { NetworkProbe } from "@/lib/types";
import { cn } from "@/lib/utils";

const latencyText = (v: number | null) => (v == null ? "—" : `${Math.round(v)} ms`);
const failureText = (v: number | null) => (v == null ? "—" : `${Number(v.toFixed(1))}%`);

export function NetworkSummary({ probes, now }: { probes: NetworkProbe[]; now: number }) {
  const { t } = useI18n();
  const overview = networkOverview(probes, now);
  if (overview.probes.length === 0) return null;

  return (
    <section className="relative z-10 grid gap-2 border-t pt-2.5 text-xs" aria-label={t("network.cardTitle")}>
      <div className="flex items-center justify-between gap-3">
        <span className="shrink-0 text-muted-foreground">{t("network.probesShort")}</span>
        <div className="flex min-w-0 items-center justify-end gap-2">
          {overview.probes.map((probe) => (
            <Tooltip key={probe.target}>
              <TooltipTrigger asChild>
                <button
                  type="button"
                  className="min-w-0 cursor-help text-right tabular-nums"
                  aria-label={`${probe.target}: ${latencyText(probe.latency_fresh && probe.avg_ms >= 0 ? probe.avg_ms : null)}`}
                >
                  <span className="block max-w-20 truncate text-[10px] text-muted-foreground">{probe.target}</span>
                  <span className={cn("font-medium", probe.failure_rate >= 20 && "text-ember")}>
                    {latencyText(probe.latency_fresh && probe.avg_ms >= 0 ? probe.avg_ms : null)}
                  </span>
                </button>
              </TooltipTrigger>
              <TooltipContent sideOffset={6} className="max-w-64 text-left leading-relaxed">
                <p className="font-medium">{probe.target}</p>
                <p>
                  {t("network.averageLatency")}:{" "}
                  {latencyText(probe.latency_fresh && probe.avg_ms >= 0 ? probe.avg_ms : null)}
                </p>
                <p>
                  {t("network.failureRate")}: {failureText(probe.failure_rate)}
                </p>
                <p className="mt-1">{t("network.windowHint")}</p>
                <p>{t("network.tcpHint")}</p>
              </TooltipContent>
            </Tooltip>
          ))}
        </div>
      </div>
      {overview.samples > 0 && (
        <div className="grid grid-cols-2 gap-3">
          <ProbeStrip kind="latency" overview={overview} />
          <ProbeStrip kind="failure" overview={overview} />
        </div>
      )}
    </section>
  );
}

function ProbeStrip({ kind, overview }: { kind: "latency" | "failure"; overview: ReturnType<typeof networkOverview> }) {
  const { t, lang } = useI18n();
  const [active, setActive] = useState<number | null>(null);
  const format = kind === "latency" ? latencyText : failureText;
  const label = t(kind === "latency" ? "network.averageLatency" : "network.averageFailure");
  const point = active == null ? null : overview.history[active];
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className="group min-w-0 cursor-help text-left"
          aria-label={`${label}: ${format(overview[kind])}`}
          onPointerMove={(e) => {
            const bounds = e.currentTarget.getBoundingClientRect();
            setActive(Math.max(0, Math.min(10, Math.floor(((e.clientX - bounds.left) / bounds.width) * 11))));
          }}
          onPointerLeave={() => setActive(null)}
          onFocus={() => setActive(null)}
          onKeyDown={(e) => {
            if (e.key === "ArrowLeft" || e.key === "ArrowRight") {
              e.preventDefault();
              setActive(Math.max(0, Math.min(10, (active ?? 10) + (e.key === "ArrowLeft" ? -1 : 1))));
            }
          }}
        >
          <span className="mb-1 flex items-center justify-between gap-1 text-[11px] tabular-nums">
            <span className="text-muted-foreground">
              {t(kind === "latency" ? "network.latencyShort" : "network.failShort")}
            </span>
            <span className={cn(kind === "failure" && (overview.failure ?? 0) >= 20 && "text-ember")}>
              {format(overview[kind])}
            </span>
          </span>
          <span className="flex h-2 items-end gap-0.5" aria-hidden>
            {overview.history.map((b, i) => {
              const value = b[kind];
              return (
                <span
                  key={b.t}
                  className={cn(
                    "min-w-0 flex-1 rounded-[1px]",
                    value == null
                      ? "bg-muted"
                      : kind === "failure" && value >= 20
                        ? "bg-ember"
                        : kind === "failure" && value > 0
                          ? "bg-amber"
                          : "bg-foreground/45",
                    active === i && "outline outline-offset-1 outline-foreground/50",
                  )}
                  style={{
                    height:
                      value == null ? "50%" : kind === "latency" ? `${Math.max(30, Math.min(100, value / 4))}%` : "70%",
                  }}
                />
              );
            })}
          </span>
        </button>
      </TooltipTrigger>
      <TooltipContent sideOffset={8} className="max-w-64 text-left leading-relaxed">
        <p className="font-medium">
          {label}: {format(point ? point[kind] : overview[kind])}
        </p>
        <p>
          {point
            ? `${clock(point.t, lang, false, true)}–${clock(point.t + 30, lang, false, true)}`
            : t("network.cardTitle")}
        </p>
        {point && (
          <p>
            {point.samples > 0
              ? t("network.bucketCoverage", { n: point.targets, total: overview.probes.length })
              : t("network.unknownSamples")}
          </p>
        )}
        <p className="mt-1">{t("network.averageHint")}</p>
        {kind === "failure" && <p>{t("network.tcpHint")}</p>}
        <p>{t("network.stripHint")}</p>
      </TooltipContent>
    </Tooltip>
  );
}
