import { useQueries } from "@tanstack/react-query";
import { ArrowLeft, Copy, X } from "lucide-react";
import { useState } from "react";
import { Link, useSearchParams } from "react-router";
import { CartesianGrid, Line, LineChart, XAxis, YAxis } from "recharts";
import { Shell } from "@/components/Shell";
import { Button } from "@/components/ui/button";
import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError, api, LIVE_INTERVAL, type Range, useMeta, useNodes } from "@/lib/api";
import { summaryLine } from "@/lib/display";
import { bytes, clock, date, percent, rate, ratio } from "@/lib/format";
import { isPending, issues } from "@/lib/health";
import { type MessageKey, useI18n } from "@/lib/i18n";
import {
  alignMetrics,
  type CompareMetric,
  comparisonChartRows,
  finiteValue,
  isVisible,
  MAX_COMPARE,
  parseCompareIds,
  visibleCompareNodes,
} from "@/lib/node-view";
import type { Metric, PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";

const RANGE_HOURS: Record<Range, number> = { live: 0, "1h": 1, "6h": 6, "24h": 24, "7d": 168, "30d": 720, "90d": 2160 };
// Comparison hues identify series only; ember remains reserved for faults.
const COLORS = ["var(--chart-1)", "var(--chart-4)", "var(--foreground)", "var(--chart-2)"];
const DASHES = [undefined, "7 3", "2 3", "9 3 2 3"];

export function Compare() {
  const { t } = useI18n();
  const [params, setParams] = useSearchParams();
  const nodes = useNodes();
  const meta = useMeta();
  const [copied, setCopied] = useState<string | null>(null);
  const ids = parseCompareIds(params.get("ids"));
  const allowedRanges = (Object.keys(RANGE_HOURS) as Range[]).filter(
    (r) => RANGE_HOURS[r] <= (meta.data?.keep_hour_days ?? 90) * 24,
  );
  const requestedRange = params.get("range") as Range;
  const range = allowedRanges.includes(requestedRange) ? requestedRange : "live";
  // A cached list from an earlier viewer is not authorization for a deep link.
  const ready = nodes.isFetchedAfterMount && !nodes.isError;
  const available = ready ? (nodes.data ?? []) : [];
  const selected = visibleCompareNodes(ids, available);
  const metrics = useQueries({
    queries: selected.map((node) => ({
      queryKey: ["metrics", node.uuid, range],
      queryFn: () => api<Metric[]>(`/api/v1/nodes/${encodeURIComponent(node.uuid)}/metrics?range=${range}`),
      refetchInterval: range === "live" ? LIVE_INTERVAL : 60_000,
      retry: (count: number, error: Error) => !(error instanceof ApiError && error.status < 500) && count < 2,
    })),
  });
  const series = metrics.map((query) => (query.isFetchedAfterMount && !query.isError ? (query.data ?? []) : []));
  const waiting = metrics.some((query) => !query.isFetchedAfterMount);
  const failed = metrics.some((query) => query.isError);
  const disabled = nodes.error instanceof ApiError && nodes.error.code === "public_disabled";
  const unavailable = ready && ids.some((id) => !available.some((node) => node.uuid === id));
  const update = (nextIds: string[], nextRange = range) => {
    const next = new URLSearchParams(params);
    if (nextIds.length) next.set("ids", parseCompareIds(nextIds.join(",")).join(","));
    else next.delete("ids");
    if (nextRange === "live") next.delete("range");
    else next.set("range", nextRange);
    setParams(next);
    setCopied(null);
  };
  const share = async () => {
    try {
      await navigator.clipboard.writeText(window.location.href);
      setCopied("compare.copied");
    } catch {
      setCopied("compare.copyFailed");
    }
  };
  const stepUnit = RANGE_HOURS[range] <= (meta.data?.keep_minute_days ?? 7) * 24 ? 60 : 3600;
  const gapSeconds =
    range === "live" ? 180 : Math.max(stepUnit, Math.ceil((RANGE_HOURS[range] * 3600) / 360 / stepUnit) * stepUnit) * 2;

  return (
    <Shell>
      <Link
        to="/"
        className="mt-3 mb-5 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground"
      >
        <ArrowLeft className="size-4" aria-hidden />
        {t("detail.back")}
      </Link>
      <header className="mb-6 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">{t("compare.title")}</h1>
          <p className="mt-2 max-w-prose text-sm leading-relaxed text-muted-foreground">{t("compare.description")}</p>
        </div>
        {selected.length > 0 && (
          <Button variant="outline" onClick={() => void share()}>
            <Copy className="size-4" aria-hidden />
            {t("compare.share")}
          </Button>
        )}
      </header>
      {copied && (
        <p role="status" className="mb-4 text-sm text-muted-foreground">
          {t(copied as MessageKey)}
        </p>
      )}
      {nodes.isError ? (
        <div role="alert" className="rounded-xl border p-6">
          <p className="text-sm text-ember">{t(disabled ? "public.disabled" : "error.load")}</p>
          {disabled ? (
            meta.data?.admin && (
              <Link to="/login" className="mt-3 inline-block text-sm underline underline-offset-4">
                {t("public.disabledHint")}
              </Link>
            )
          ) : (
            <Button variant="outline" className="mt-3" onClick={() => void nodes.refetch()}>
              {t("public.retry")}
            </Button>
          )}
        </div>
      ) : !ready ? (
        <div role="status" aria-busy="true" aria-label={t("hero.loading")}>
          <Skeleton className="h-32 rounded-xl" />
        </div>
      ) : (
        <>
          <section className="mb-6 space-y-3" aria-label={t("compare.selection")}>
            <div className="flex flex-wrap items-center gap-3">
              <label htmlFor="compare-add" className="text-sm font-medium">
                {t("compare.add")}
              </label>
              <select
                id="compare-add"
                className="public-select min-w-0 max-w-full flex-1 sm:max-w-80 sm:flex-none"
                value=""
                disabled={selected.length >= MAX_COMPARE || available.length === selected.length}
                onChange={(e) => {
                  if (e.target.value) update([...selected.map((n) => n.uuid), e.target.value]);
                }}
              >
                <option value="">{t(selected.length >= MAX_COMPARE ? "compare.limit" : "compare.pick")}</option>
                {available
                  .filter((node) => !selected.some((n) => n.uuid === node.uuid))
                  .map((node) => (
                    <option key={node.uuid} value={node.uuid}>
                      {node.name}
                      {node.group ? ` (${node.group})` : ""}
                    </option>
                  ))}
              </select>
              <span className="text-xs text-muted-foreground tabular-nums" role="status">
                {t("compare.count", { n: selected.length })}
              </span>
              {ids.length > 0 && (
                <Button variant="ghost" size="sm" onClick={() => update([])}>
                  {t("compare.clear")}
                </Button>
              )}
            </div>
            {unavailable && (
              <p className="text-sm text-muted-foreground" role="status">
                {t("compare.unavailable")}
              </p>
            )}
            {selected.length > 0 && (
              <ul className="flex flex-wrap gap-2">
                {selected.map((node, index) => (
                  <li
                    key={node.uuid}
                    className="flex min-w-0 max-w-full items-center gap-2 rounded-md border bg-card py-1 ps-3 pe-1"
                  >
                    <span className="h-0.5 w-4 shrink-0" style={{ background: COLORS[index] }} aria-hidden />
                    <span className="truncate text-sm" title={node.name}>
                      {node.name}
                    </span>
                    <Button
                      variant="ghost"
                      size="icon"
                      className="size-8 shrink-0"
                      aria-label={t("compare.removeNode", { name: node.name })}
                      onClick={() => update(selected.filter((n) => n.uuid !== node.uuid).map((n) => n.uuid))}
                    >
                      <X className="size-4" aria-hidden />
                    </Button>
                  </li>
                ))}
              </ul>
            )}
          </section>
          {selected.length === 0 ? (
            <div className="rounded-xl border border-dashed px-5 py-12 text-center">
              <h2 className="font-medium">{t(available.length ? "compare.empty" : "hero.empty")}</h2>
              <p className="mt-2 text-sm text-muted-foreground">
                {t(available.length ? "compare.emptyHint" : "compare.noNodes")}
              </p>
            </div>
          ) : (
            <>
              <CurrentSpecs nodes={selected} />
              <section className="mt-8" aria-label={t("compare.history")}>
                <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                  <h2 className="text-lg font-medium">{t("compare.history")}</h2>
                  <div className="flex items-center gap-2">
                    <label htmlFor="compare-range" className="text-sm text-muted-foreground">
                      {t("compare.range")}
                    </label>
                    <select
                      id="compare-range"
                      className="public-select"
                      value={range}
                      onChange={(e) => update(ids, e.target.value as Range)}
                    >
                      {allowedRanges.map((value) => (
                        <option key={value} value={value}>
                          {t(`range.${value}`)}
                        </option>
                      ))}
                    </select>
                  </div>
                </div>
                {failed && (
                  <div role="alert" className="mb-4 flex flex-wrap items-center gap-3 text-sm">
                    <p className="text-ember">{t("compare.metricsError")}</p>
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        for (const query of metrics) if (query.isError) void query.refetch();
                      }}
                    >
                      {t("public.retry")}
                    </Button>
                  </div>
                )}
                <ul className="mb-4 flex flex-wrap gap-x-5 gap-y-2 text-xs text-muted-foreground">
                  {selected.map((node, i) => (
                    <li key={node.uuid} className="flex min-w-0 max-w-full items-center gap-2">
                      <svg width="22" height="8" viewBox="0 0 22 8" aria-hidden className="shrink-0">
                        <path d="M0 4H22" stroke={COLORS[i]} strokeWidth="2" strokeDasharray={DASHES[i]} />
                      </svg>
                      <span className="truncate">{node.name}</span>
                      <span className="shrink-0">
                        {t(
                          !metrics[i]?.isFetchedAfterMount
                            ? "compare.loading"
                            : metrics[i]?.isError
                              ? "compare.failed"
                              : !series[i]?.length
                                ? "compare.noSamples"
                                : "compare.ready",
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
                {waiting && series.every((points) => points.length === 0) ? (
                  <div
                    role="status"
                    aria-busy="true"
                    aria-label={t("compare.loading")}
                    className="grid gap-3 lg:grid-cols-2"
                  >
                    {Array.from({ length: 4 }, (_, i) => (
                      <Skeleton key={i} className="h-64 rounded-xl" />
                    ))}
                  </div>
                ) : (
                  <div className="grid gap-4 lg:grid-cols-2">
                    {(
                      [
                        { metric: "cpu", title: "chart.cpu", format: percent },
                        { metric: "mem_used", title: "compare.memoryUsed", format: bytes },
                        { metric: "net_in", title: "series.in", format: rate },
                        { metric: "net_out", title: "series.out", format: rate },
                      ] as const
                    ).map(({ metric, title, format }) => (
                      <ComparisonChart
                        key={metric}
                        title={t(title)}
                        metric={metric}
                        nodes={selected}
                        series={series}
                        range={range}
                        gapSeconds={gapSeconds}
                        format={format}
                      />
                    ))}
                  </div>
                )}
                <p className="mt-3 text-xs leading-relaxed text-muted-foreground">{t("compare.alignment")}</p>
              </section>
            </>
          )}
        </>
      )}
    </Shell>
  );
}

function CurrentSpecs({ nodes }: { nodes: PublicNode[] }) {
  const { t, lang } = useI18n();
  const missing = t("compare.notAvailable");
  const value = (n: number | null | undefined, format: (n: number) => string) =>
    finiteValue(n) === null ? missing : format(n as number);
  const rows: { label: string; cell: (node: PublicNode) => string }[] = [
    {
      label: t("servers.status"),
      cell: (n) => t(isPending(n) ? "node.pending" : n.online ? "node.online" : "node.offline"),
    },
    { label: t("metric.cpu"), cell: (n) => (n.online ? value(n.status?.cpu, percent) : missing) },
    {
      label: t("metric.mem"),
      cell: (n) =>
        n.online && n.status && n.status.mem_total > 0
          ? `${bytes(n.status.mem_used)} / ${bytes(n.status.mem_total)} (${percent(ratio(n.status.mem_used, n.status.mem_total))})`
          : missing,
    },
    { label: t("series.in"), cell: (n) => (n.online ? value(n.status?.net_in, rate) : missing) },
    { label: t("series.out"), cell: (n) => (n.online ? value(n.status?.net_out, rate) : missing) },
  ];
  if (nodes.some((n) => isVisible(n, "system") && summaryLine(n.info)))
    rows.push({
      label: t("detail.os"),
      cell: (n) => (isVisible(n, "system") ? summaryLine(n.info) || missing : missing),
    });
  if (nodes.some((n) => isVisible(n, "system") && n.info.cpu_model))
    rows.push({
      label: t("detail.cpu"),
      cell: (n) => (isVisible(n, "system") ? n.info.cpu_model || missing : missing),
    });
  if (nodes.some((n) => isVisible(n, "traffic")))
    rows.push({
      label: t("summary.traffic"),
      cell: (n) =>
        isVisible(n, "traffic")
          ? `${bytes(n.traffic.used)}${n.traffic.limit > 0 ? ` / ${bytes(n.traffic.limit)}` : ""}`
          : missing,
    });
  if (nodes.some((n) => isVisible(n, "expiry") && n.expire_at > 0))
    rows.push({
      label: t("servers.expires"),
      cell: (n) => (isVisible(n, "expiry") && n.expire_at > 0 ? date(n.expire_at, lang) : missing),
    });
  return (
    <section>
      <h2 className="mb-3 text-lg font-medium">{t("compare.current")}</h2>
      <section
        className="overflow-x-auto rounded-xl border bg-card"
        // biome-ignore lint/a11y/noNoninteractiveTabindex: Keyboard users must be able to scroll the comparison table horizontally.
        tabIndex={0}
        aria-label={t("compare.current")}
      >
        <table className="w-full text-left text-sm">
          <caption className="sr-only">{t("compare.current")}</caption>
          <thead>
            <tr className="border-b">
              <th scope="col" className="min-w-28 px-4 py-3 font-medium text-muted-foreground">
                {t("compare.metric")}
              </th>
              {nodes.map((node, i) => (
                <th key={node.uuid} scope="col" className="min-w-44 max-w-64 px-4 py-3 align-top font-medium">
                  <Link
                    to={`/node/${encodeURIComponent(node.uuid)}`}
                    className="break-words underline decoration-border underline-offset-4 hover:decoration-current"
                  >
                    <span
                      className="mr-2 inline-block size-2 rounded-full"
                      style={{ background: COLORS[i] }}
                      aria-hidden
                    />
                    {node.name}
                  </Link>
                </th>
              ))}
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={row.label} className="border-b last:border-0">
                <th scope="row" className="px-4 py-3 align-top font-normal text-muted-foreground">
                  {row.label}
                </th>
                {nodes.map((node) => (
                  <td
                    key={node.uuid}
                    className={cn(
                      "max-w-64 break-words px-4 py-3 align-top tabular-nums",
                      i === 0 && issues(node).length > 0 && "text-ember",
                    )}
                  >
                    {row.cell(node)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </section>
      <p className="mt-2 text-xs text-muted-foreground">{t("compare.specsHint")}</p>
    </section>
  );
}

function ComparisonChart({
  title,
  metric,
  nodes,
  series,
  range,
  gapSeconds,
  format,
}: {
  title: string;
  metric: CompareMetric;
  nodes: PublicNode[];
  series: Metric[][];
  range: Range;
  gapSeconds: number;
  format: (value: number) => string;
}) {
  const { t, lang } = useI18n();
  const rows = alignMetrics(series, metric);
  const chartRows = comparisonChartRows(series, metric, gapSeconds);
  const config = Object.fromEntries(nodes.map((node, i) => [`node${i}`, { label: node.name, color: COLORS[i] }]));
  const tick = (time: number) => clock(time, lang, RANGE_HOURS[range] > 24, range === "live");
  return (
    <section className="min-w-0 rounded-xl border bg-card p-4" aria-label={title}>
      <h3 className="mb-4 text-sm font-medium">{title}</h3>
      {rows.length === 0 ? (
        <p className="flex h-48 items-center justify-center text-sm text-muted-foreground">{t("chart.empty")}</p>
      ) : (
        <ChartContainer config={config} className="aspect-auto h-48 w-full">
          <LineChart data={chartRows} accessibilityLayer margin={{ left: 0, right: 8, top: 4, bottom: 0 }}>
            <CartesianGrid vertical={false} strokeDasharray="3 3" />
            <XAxis
              dataKey="t"
              type="number"
              domain={[rows[0].t, rows[rows.length - 1].t]}
              scale="time"
              tickFormatter={tick}
              tickLine={false}
              axisLine={false}
              minTickGap={48}
            />
            <YAxis
              domain={metric === "cpu" ? [0, 100] : [0, "auto"]}
              tickFormatter={format}
              tickLine={false}
              axisLine={false}
              width={72}
              tickCount={4}
            />
            <ChartTooltip
              content={({ active, label }) => {
                if (!active || label == null) return null;
                const row = rows.find((point) => point.t === Number(label));
                if (!row) return null;
                return (
                  <div className="grid max-w-72 gap-2 rounded-lg border bg-popover px-3 py-2 text-xs text-popover-foreground">
                    <p className="font-medium">{clock(row.t, lang, true, range === "live")}</p>
                    {nodes.map((node, i) => (
                      <div key={node.uuid} className="flex items-center justify-between gap-4">
                        <span className="truncate">{node.name}</span>
                        <span className="shrink-0 tabular-nums">
                          {row[`node${i}`] == null ? t("compare.notAvailable") : format(row[`node${i}`] as number)}
                        </span>
                      </div>
                    ))}
                  </div>
                );
              }}
            />
            {series.map((points, i) => (
              <Line
                key={nodes[i].uuid}
                dataKey={`node${i}`}
                name={nodes[i].name}
                type="linear"
                stroke={COLORS[i]}
                strokeDasharray={DASHES[i]}
                strokeWidth={1.75}
                dot={points.length < 3 ? { r: 2 } : false}
                connectNulls={false}
                isAnimationActive={false}
              />
            ))}
          </LineChart>
        </ChartContainer>
      )}
    </section>
  );
}
