import { NodeDetailLink } from "@/components/NodeDetailLink";
import { issueText, summaryLine } from "@/lib/display";
import { bytes, dateTime, duration, ratio } from "@/lib/format";
import { isPending, issues } from "@/lib/health";
import { useI18n } from "@/lib/i18n";
import { compactCapacity, isVisible } from "@/lib/node-view";
import type { PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Expiry } from "./Expiry";
import { NetRates } from "./NetRates";
import type { NodeSelection } from "./NodeCard";
import { NodeTags } from "./NodeTags";
import { StatusDot } from "./StatusDot";
import { QuotaText, TrafficFigures } from "./Traffic";
import { UsageMeter } from "./UsageBar";

// One template for the header and every row so the columns line up. Below `xl`
// the columns no longer fit and each row switches to a stacked layout.
const COLUMNS = "xl:grid-cols-[minmax(14rem,1.8fr)_repeat(3,minmax(5rem,1fr))_7rem_10.5rem_7.5rem]";

export function NodeList({
  nodes,
  now,
  selectionFor,
}: {
  nodes: PublicNode[];
  now: number;
  selectionFor?: (node: PublicNode) => NodeSelection;
}) {
  const { t } = useI18n();
  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <div className={cn("hidden gap-4 border-b px-4 py-2 text-xs text-muted-foreground xl:grid", COLUMNS)} aria-hidden>
        <span>{t("list.server")}</span>
        <span>{t("metric.cpu")}</span>
        <span>{t("metric.mem")}</span>
        <span>{t("metric.disk")}</span>
        <span>{t("list.net")}</span>
        <span>{t("node.traffic")}</span>
        <span>{t("list.uptime")}</span>
      </div>
      <ul className="divide-y">
        {nodes.map((n) => (
          <li key={n.uuid} className="relative">
            {selectionFor && (
              <label className="flex min-h-10 items-center gap-2 border-b border-dashed px-4 text-xs text-muted-foreground">
                <input
                  type="checkbox"
                  checked={selectionFor(n).selected}
                  disabled={selectionFor(n).disabled}
                  onChange={selectionFor(n).onToggle}
                  aria-label={t("compare.selectNode", { name: n.name })}
                  className="size-4 accent-primary"
                />
                {t("compare.select")}
              </label>
            )}
            <NodeRow node={n} now={now} />
          </li>
        ))}
      </ul>
    </div>
  );
}

function NodeRow({ node, now }: { node: PublicNode; now: number }) {
  const { t, lang } = useI18n();
  const lit = issues(node).length > 0;
  const pending = isPending(node);
  const s = node.online ? node.status : null;
  const cpu = s?.cpu ?? 0;
  const load = s ? `${s.load1.toFixed(2)}, ${s.load5.toFixed(2)}, ${s.load15.toFixed(2)}` : "";
  const mem = ratio(s?.mem_used ?? 0, s?.mem_total ?? 0);
  const disk = ratio(s?.disk_used ?? 0, s?.disk_total ?? 0);
  const tr = node.traffic;
  const uptime = node.online && s ? duration(s.uptime, lang) : "-";
  const problem = lit ? issueText(node, now, t, lang) : "";
  const sub = isVisible(node, "system") ? summaryLine(node.info, compactCapacity(node)) : "";
  const memDetail =
    s && s.mem_total > 0
      ? t("card.usedTotal", { used: bytes(s.mem_used), total: bytes(s.mem_total) })
      : t("card.unknown");
  const diskDetail =
    s && s.disk_total > 0
      ? t("card.usedTotal", { used: bytes(s.disk_used), total: bytes(s.disk_total) })
      : t("card.unknown");

  const name = (
    <div className="flex min-w-0 items-center gap-2">
      <StatusDot state={pending ? "pending" : lit ? "lit" : "ok"} />
      <span className="truncate font-medium" title={node.name}>
        {node.name}
      </span>
      <span className="sr-only">{t(pending ? "node.pending" : node.online ? "node.online" : "node.offline")}</span>
      {node.group && (
        <span
          className="max-w-24 shrink-0 truncate rounded-md bg-muted px-1.5 py-0.5 text-xs text-muted-foreground"
          title={node.group}
        >
          {node.group}
        </span>
      )}
    </div>
  );

  const metadata = (
    <div className="min-w-0">
      {name}
      {sub && (
        <p className="mt-1 truncate ps-4 text-xs text-muted-foreground" title={sub}>
          {sub}
        </p>
      )}
      <NodeTags value={node.public_tags} className="mt-1.5 ps-4" />
      {node.public_note && (
        <p
          className="mt-1.5 line-clamp-2 break-words ps-4 text-xs leading-relaxed text-muted-foreground"
          title={node.public_note}
        >
          {node.public_note}
        </p>
      )}
      {problem && <p className="mt-1.5 ps-4 text-xs text-ember">{problem}</p>}
    </div>
  );
  const missingMetrics = (
    <div className="grid gap-1 text-xs text-muted-foreground">
      <p>{t(pending ? "node.pending" : node.online ? "node.noData" : "card.offlineMetrics")}</p>
      {!pending && !node.online && node.last_seen > 0 && (
        <time dateTime={new Date(node.last_seen * 1000).toISOString()}>
          {t("card.lastSeen", { time: dateTime(node.last_seen, lang) })}
        </time>
      )}
    </div>
  );

  return (
    <NodeDetailLink
      uuid={node.uuid}
      className={cn(
        "block px-4 py-3 transition-colors hover:bg-accent/40 focus-visible:bg-accent/40 focus-visible:outline-2 focus-visible:-outline-offset-2 focus-visible:outline-ring",
        lit && "shadow-[inset_1px_0_0_var(--ember)]",
      )}
    >
      {/* Wide screens: one aligned row. */}
      <div className={cn("hidden items-center gap-4 xl:grid", COLUMNS)}>
        {metadata}
        {pending || !s ? (
          <div className="col-span-3">{missingMetrics}</div>
        ) : (
          <>
            <UsageMeter pct={cpu} detail={load} detailLabel={t("list.loadMinutes")} />
            <UsageMeter pct={mem} detail={memDetail} />
            <UsageMeter pct={disk} detail={diskDetail} />
          </>
        )}
        <NetRates node={node} stacked className="text-xs text-muted-foreground" />
        <div className="grid gap-0.5 text-xs text-muted-foreground">
          {isVisible(node, "traffic") ? (
            <>
              <TrafficFigures traffic={tr} />
              <QuotaText traffic={tr} />
            </>
          ) : (
            <span>{t("compare.notAvailable")}</span>
          )}
        </div>
        <div className="grid justify-items-start gap-0.5 text-xs tabular-nums text-muted-foreground">
          <span>{uptime}</span>
          {isVisible(node, "expiry") && <Expiry node={node} now={now} />}
        </div>
      </div>

      {/* Narrow screens retain the same information in stacked groups. */}
      <div className="xl:hidden">
        {metadata}
        {pending ? (
          <p className="mt-1 ps-4 text-xs text-muted-foreground">{t("node.pending")}</p>
        ) : (
          <>
            {s ? (
              <div className="mt-3 grid grid-cols-3 gap-x-3 ps-4">
                <UsageMeter label={t("metric.cpu")} pct={cpu} detail={load} detailLabel={t("list.loadMinutes")} />
                <UsageMeter label={t("metric.mem")} pct={mem} detail={memDetail} />
                <UsageMeter label={t("metric.disk")} pct={disk} detail={diskDetail} />
              </div>
            ) : (
              <div className="mt-2 ps-4">{missingMetrics}</div>
            )}
            {s && (
              <div className="mt-3 flex flex-wrap items-center justify-between gap-2 ps-4 text-xs text-muted-foreground">
                <NetRates node={node} />
                <span>{t("node.uptime", { d: uptime })}</span>
              </div>
            )}
            <div className="mt-2 flex flex-wrap items-center justify-between gap-x-4 gap-y-1 ps-4 text-xs text-muted-foreground">
              {isVisible(node, "traffic") && (
                <span className="flex flex-wrap items-center gap-x-2.5 gap-y-0.5">
                  {t("node.traffic")}
                  <TrafficFigures traffic={tr} />
                  <QuotaText traffic={tr} />
                </span>
              )}
              {isVisible(node, "expiry") && <Expiry node={node} now={now} />}
            </div>
          </>
        )}
      </div>
    </NodeDetailLink>
  );
}
