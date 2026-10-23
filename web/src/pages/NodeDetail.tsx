import { ArrowLeft } from "lucide-react";
import { Fragment, useMemo, useState } from "react";
import { Link, useParams } from "react-router";
import { Availability } from "@/components/Availability";
import { MetricChart, type Series } from "@/components/MetricChart";
import { NodeTags } from "@/components/NodeTags";
import { Shell } from "@/components/Shell";
import { StatusDot } from "@/components/StatusDot";
import { TrafficFigures } from "@/components/Traffic";
import { Skeleton } from "@/components/ui/skeleton";
import { type Range, useMeta, useMetrics, useNodes, usePing } from "@/lib/api";
import { bytes, date, daysUntil, duration, listSep, percent, rate, ratio } from "@/lib/format";
import { isPending, issues } from "@/lib/health";
import { type MessageKey, useI18n } from "@/lib/i18n";
import { pingBucketSeconds, pingChartRows } from "@/lib/ping-history";
import type { PingSeries, PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";

const RANGE_HOURS: Record<Range, number> = { live: 0, "1h": 1, "6h": 6, "24h": 24, "7d": 168, "30d": 720, "90d": 2160 };
const PING_COLORS = ["var(--chart-1)", "var(--chart-2)", "var(--chart-4)", "var(--chart-5)", "var(--chart-3)"];

export function NodeDetail({ embedded = false }: { embedded?: boolean }) {
  const Frame = embedded ? Fragment : Shell;
  const { uuid = "" } = useParams();
  const { t } = useI18n();
  const nodes = useNodes();
  const meta = useMeta();
  const [range, setRange] = useState<Range>("live");
  const node = nodes.data?.find((n) => n.uuid === uuid);

  const keepHours = (meta.data?.keep_hour_days ?? 90) * 24;
  const keepMinuteHours = (meta.data?.keep_minute_days ?? 7) * 24;
  const ranges = (Object.keys(RANGE_HOURS) as Range[]).filter((r) => RANGE_HOURS[r] <= keepHours);

  const metrics = useMetrics(uuid, range);

  if (nodes.isPending) {
    return (
      <Frame>
        <Skeleton className="mt-8 h-40 rounded-xl" />
      </Frame>
    );
  }
  if (nodes.isError)
    return (
      <Frame>
        {!embedded && <BackLink />}
        <p role="alert" className="py-12 text-sm text-destructive">
          {t("error.load")}
        </p>
      </Frame>
    );
  if (!node) {
    return (
      <Frame>
        {!embedded && <BackLink />}
        <p className="py-24 text-center text-lg">{t("detail.notFound")}</p>
      </Frame>
    );
  }

  const scale = range === "live" ? "live" : RANGE_HOURS[range] > 24 ? "long" : "short";
  const s = node.status;
  const data = metrics.data ?? [];

  return (
    <Frame>
      {!embedded && <BackLink />}
      <Header node={node} />
      <InfoGrid node={node} />
      <Availability uuid={uuid} />

      <div
        className="-mx-4 mt-6 mb-3 flex gap-1 overflow-x-auto px-4 [scrollbar-width:none] sm:mx-0 sm:mt-8 sm:px-0"
        role="tablist"
      >
        {ranges.map((r) => (
          <button
            key={r}
            type="button"
            role="tab"
            aria-selected={range === r}
            onClick={() => setRange(r)}
            className={cn(
              "h-8 shrink-0 rounded-md px-3 text-sm transition-colors",
              range === r ? "bg-secondary text-secondary-foreground" : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t(`range.${r}` as MessageKey)}
          </button>
        ))}
      </div>

      {metrics.isError && (
        <p role="alert" className="mb-4 text-sm text-destructive">
          {t("error.load")}
        </p>
      )}
      {metrics.isPending && <Skeleton className="mb-4 h-44 rounded-xl" />}
      <div className="grid gap-3 lg:grid-cols-2">
        <MetricChart
          title={t("chart.cpu")}
          data={data.map((m) => ({ t: m.t, cpu: m.cpu }))}
          series={[{ key: "cpu", label: t("metric.cpu"), color: "var(--chart-1)" }]}
          format={percent}
          domain={[0, 100]}
          scale={scale}
        />
        <MetricChart
          title={t("chart.mem")}
          aside={s ? bytes(s.mem_total) : undefined}
          data={data.map((m) => ({ t: m.t, mem: m.mem_used, swap: m.swap_used }))}
          series={[
            { key: "mem", label: t("series.used"), color: "var(--chart-4)" },
            ...(s && s.swap_total > 0 ? [{ key: "swap", label: t("series.swap"), color: "var(--chart-5)" }] : []),
          ]}
          format={bytes}
          domain={s?.mem_total ? [0, s.mem_total] : undefined}
          scale={scale}
        />
        <MetricChart
          title={t("chart.net")}
          data={data.map((m) => ({ t: m.t, in: m.net_in, out: m.net_out }))}
          series={[
            { key: "in", label: t("series.in"), color: "var(--chart-2)" },
            { key: "out", label: t("series.out"), color: "var(--chart-1)" },
          ]}
          format={rate}
          scale={scale}
        />
        <MetricChart
          title={t("chart.disk")}
          aside={s ? bytes(s.disk_total) : undefined}
          data={data.map((m) => ({ t: m.t, disk: m.disk_used }))}
          series={[{ key: "disk", label: t("series.used"), color: "var(--chart-5)" }]}
          format={bytes}
          domain={s?.disk_total ? [0, s.disk_total] : undefined}
          scale={scale}
        />
        <MetricChart
          title={t("chart.load")}
          data={data.map((m) => ({ t: m.t, load: m.load1 }))}
          series={[{ key: "load", label: t("metric.load"), color: "var(--chart-4)" }]}
          format={(v) => v.toFixed(2)}
          kind="line"
          scale={scale}
        />
        <MetricChart
          title={t("chart.conn")}
          data={data.map((m) => ({ t: m.t, tcp: m.tcp, udp: m.udp }))}
          series={[
            { key: "tcp", label: "TCP", color: "var(--chart-1)" },
            { key: "udp", label: "UDP", color: "var(--chart-2)" },
          ]}
          format={(v) => String(Math.round(v))}
          kind="line"
          scale={scale}
        />
      </div>
      {node.visibility.ping && <NetworkHistory uuid={uuid} keepHours={keepHours} minuteHours={keepMinuteHours} />}
    </Frame>
  );
}

function BackLink() {
  const { t } = useI18n();
  return (
    <Link to="/" className="mt-2 inline-flex items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
      <ArrowLeft className="size-4" aria-hidden />
      {t("detail.back")}
    </Link>
  );
}

function Header({ node }: { node: PublicNode }) {
  const { t, lang } = useI18n();
  const problems = issues(node);
  const pending = isPending(node);
  const now = Date.now() / 1000;
  const status = pending
    ? t("node.pending")
    : node.online
      ? t("node.online")
      : node.last_seen
        ? t("node.offlineFor", { d: duration(now - node.last_seen, lang) })
        : t("node.offline");

  return (
    <div className="mt-6 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-2xl font-medium tracking-tight sm:text-3xl">{node.name}</h1>
        <p className="mt-2 flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <StatusDot state={pending ? "pending" : problems.length ? "lit" : "ok"} />
          <span className={cn(problems.length > 0 && "text-ember")}>
            {[status, ...problems.filter((p) => p !== "offline").map((p) => t(`issue.${p}`))].join(listSep(lang))}
          </span>
          {node.group && <span className="rounded-md bg-muted px-1.5 py-0.5 text-xs">{node.group}</span>}
        </p>
        <NodeTags value={node.public_tags} className="mt-3" />
      </div>
      {node.public_note && <p className="max-w-prose text-sm text-muted-foreground">{node.public_note}</p>}
    </div>
  );
}

function InfoGrid({ node }: { node: PublicNode }) {
  const { t, lang } = useI18n();
  const s = node.status;
  const i = node.info;
  const tr = node.traffic;
  const now = Date.now() / 1000;

  const items: [string, string][] = [
    [t("detail.os"), i.os || "-"],
    [t("detail.kernel"), i.kernel || "-"],
    [
      t("detail.cpu"),
      [i.cpu_model, i.cpu_cores ? t("detail.cores", { n: i.cpu_cores }) : ""].filter(Boolean).join(", ") || "-",
    ],
    [t("detail.virt"), i.virt && i.virt !== "none" ? i.virt : "-"],
    [t("detail.arch"), i.arch || "-"],
    [t("detail.agent"), i.agent_version || "-"],
  ];
  if (!node.visibility.system) items.length = 0;
  if (s) {
    items.push(
      [t("detail.uptime"), duration(s.uptime, lang)],
      [t("metric.mem"), `${bytes(s.mem_used)} / ${bytes(s.mem_total)} (${percent(ratio(s.mem_used, s.mem_total))})`],
      [
        t("metric.disk"),
        `${bytes(s.disk_used)} / ${bytes(s.disk_total)} (${percent(ratio(s.disk_used, s.disk_total))})`,
      ],
      [t("metric.load"), `${s.load1.toFixed(2)}  ${s.load5.toFixed(2)}  ${s.load15.toFixed(2)}`],
      [t("metric.conn"), `TCP ${s.tcp}, UDP ${s.udp}`],
      [t("metric.procs"), String(s.procs)],
    );
  }
  if (node.visibility.traffic)
    items.push([t("detail.period"), `${bytes(tr.used)} / ${tr.limit > 0 ? bytes(tr.limit) : t("detail.unlimited")}`]);
  if (node.expire_at > 0) {
    const days = daysUntil(node.expire_at, now);
    items.push([
      t("servers.expires"),
      `${date(node.expire_at, lang)} (${days < 0 ? t("node.expired") : t("node.expiresIn", { n: days })})`,
    ]);
  }
  if (node.last_seen) {
    items.push([t("detail.lastSeen"), t("detail.ago", { d: duration(now - node.last_seen, lang) })]);
  }

  return (
    <dl className="mt-5 grid grid-cols-2 gap-x-4 gap-y-3 rounded-xl border bg-card p-4 text-sm sm:mt-6 sm:gap-x-8 lg:grid-cols-3">
      {items.map(([k, v]) => (
        <div key={k} className="min-w-0">
          <dt className="text-xs text-muted-foreground">{k}</dt>
          <dd className="mt-0.5 break-words tabular-nums lg:truncate" title={v}>
            {v}
          </dd>
        </div>
      ))}
      {node.visibility.traffic && (
        <div className="col-span-2 min-w-0 lg:col-span-3">
          <dt className="text-xs text-muted-foreground">{t("node.traffic")}</dt>
          <dd className="mt-0.5 flex flex-wrap items-center gap-x-4 gap-y-1 tabular-nums">
            <TrafficFigures traffic={tr} />
            <span className="text-xs text-muted-foreground">
              {t("traffic.tip", { mode: t(`modeShort.${tr.mode}`), d: tr.reset_day })}
            </span>
          </dd>
        </div>
      )}
    </dl>
  );
}

function NetworkHistory({ uuid, keepHours, minuteHours }: { uuid: string; keepHours: number; minuteHours: number }) {
  const { t } = useI18n();
  const [range, setRange] = useState<Range>("24h");
  const [selected, setSelected] = useState<string[] | null>(null);
  const ping = usePing(uuid, range, true);
  const all = ping.data ?? [];
  const names = selected ?? all.slice(0, 3).map((series) => series.name);
  const visible = all.filter((series) => names.includes(series.name));
  const gapSeconds = pingBucketSeconds(RANGE_HOURS[range], minuteHours) * 1.5;
  const ranges = (["1h", "6h", "24h", "7d", "30d", "90d"] as Range[]).filter(
    (r) => RANGE_HOURS[r] <= Math.max(keepHours, minuteHours),
  );
  return (
    <section className="mt-8 grid gap-4" aria-label={t("network.history")}>
      <div>
        <h2 className="text-lg font-medium">{t("network.history")}</h2>
        <p className="mt-1 text-xs text-muted-foreground">
          {t("network.historyHint", { d: minuteHours / 24, h: keepHours / 24 })}
        </p>
      </div>
      <fieldset className="flex flex-wrap gap-1" aria-label={t("network.historyRange")}>
        {ranges.map((value) => (
          <button
            type="button"
            key={value}
            aria-pressed={range === value}
            onClick={() => setRange(value)}
            className={cn(
              "h-9 rounded-md px-3 text-sm",
              range === value
                ? "bg-secondary text-secondary-foreground"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t(`range.${value}` as MessageKey)}
          </button>
        ))}
      </fieldset>
      {ping.isPending || ping.isPlaceholderData ? (
        <Skeleton className="h-44 rounded-xl" />
      ) : ping.isError ? (
        <p role="alert" className="text-sm text-destructive">
          {t("error.load")}
        </p>
      ) : all.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t("network.historyEmpty")}</p>
      ) : (
        <>
          <fieldset className="flex flex-wrap gap-x-4 gap-y-1">
            <legend className="sr-only">{t("network.historyTargets")}</legend>
            {all.map((series) => (
              <label key={series.name} className="flex min-h-9 items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={names.includes(series.name)}
                  className="size-4 accent-primary"
                  onChange={(e) =>
                    setSelected(
                      e.target.checked ? [...names, series.name] : names.filter((name) => name !== series.name),
                    )
                  }
                />
                <span className="break-all">{series.name}</span>
              </label>
            ))}
          </fieldset>
          {visible.length > 0 ? (
            <div className="grid gap-3 lg:grid-cols-2">
              <PingChart series={visible} scale={RANGE_HOURS[range] > 24 ? "long" : "short"} gapSeconds={gapSeconds} />
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">{t("network.selectTarget")}</p>
          )}
        </>
      )}
    </section>
  );
}

function PingChart({
  series,
  scale,
  gapSeconds,
}: {
  series: PingSeries[];
  scale: "short" | "long";
  gapSeconds: number;
}) {
  const { t } = useI18n();
  const { rows, defs } = useMemo(() => {
    const defs: Series[] = series.map((s, i) => ({
      key: `p${i}`,
      label: s.name,
      color: PING_COLORS[i % PING_COLORS.length],
    }));
    return { rows: pingChartRows(series, gapSeconds), defs };
  }, [series, gapSeconds]);

  return (
    <>
      <MetricChart
        title={t("chart.ping")}
        data={rows}
        series={defs}
        format={(v) => `${v.toFixed(v >= 100 ? 0 : 1)} ms`}
        kind="line"
        scale={scale}
      />
      <MetricChart
        title={t("network.failureRate")}
        aside={t("network.tcpHint")}
        data={rows}
        series={defs.map((d, i) => ({ ...d, key: `loss${i}` }))}
        format={percent}
        domain={[0, 100]}
        kind="line"
        scale={scale}
      />
    </>
  );
}
