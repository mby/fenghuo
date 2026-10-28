import { ArrowUpRight, CirclePause, Info, TriangleAlert } from "lucide-react";
import { NodeDetailLink } from "@/components/NodeDetailLink";
import { Button } from "@/components/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { issueText, shortOS } from "@/lib/display";
import { bytes, dateTime, duration, ratio } from "@/lib/format";
import { isPending, issues } from "@/lib/health";
import { useI18n } from "@/lib/i18n";
import { compactCapacity, isVisible } from "@/lib/node-view";
import type { PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";
import { Expiry } from "./Expiry";
import { NetRates } from "./NetRates";
import { NetworkSummary } from "./NetworkSummary";
import { NodeTags } from "./NodeTags";
import { StatusDot } from "./StatusDot";
import { QuotaHint, TrafficFigures } from "./Traffic";
import { UsageBar, UsageMeter } from "./UsageBar";

export interface NodeSelection {
  selected: boolean;
  disabled: boolean;
  onToggle: () => void;
}

export function NodeCard({ node, now, selection }: { node: PublicNode; now: number; selection?: NodeSelection }) {
  const { t, lang } = useI18n();
  const lit = issues(node).length > 0;
  const problem = lit ? issueText(node, now, t, lang) : "";
  const pending = isPending(node);
  const s = node.online ? node.status : null;
  const os = isVisible(node, "system") ? shortOS(node.info.os) : "";
  const specs = isVisible(node, "system")
    ? [node.info.arch, compactCapacity(node), node.info.virt].filter((v) => v && v !== "none").join(" / ")
    : "";
  const tr = node.traffic;
  const uptime = s ? duration(s.uptime, lang) : "";

  return (
    <article
      className={cn(
        "relative flex h-full w-full min-w-0 flex-col rounded-xl border bg-card transition-colors hover:bg-accent/20",
        lit && "lit border-transparent",
        selection?.selected && "ring-2 ring-foreground/50",
      )}
    >
      <div className="flex flex-1 flex-col gap-2.5 p-4 text-card-foreground">
        <div className="grid min-w-0 grid-cols-1 gap-1">
          <div className="flex items-start justify-between gap-3">
            <div className="flex min-w-0 flex-1 items-start gap-2">
              <StatusDot state={pending ? "pending" : lit ? "lit" : "ok"} className="mt-2" />
              <h2 className="min-w-0 flex-1 font-medium leading-6">
                <NodeDetailLink
                  uuid={node.uuid}
                  className="block min-w-0 after:absolute after:inset-0 after:rounded-xl focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:outline-ring"
                >
                  <span className="relative z-10 block truncate" title={node.name}>
                    {node.name}
                  </span>
                </NodeDetailLink>
              </h2>
              {problem && (
                <Popover>
                  <PopoverTrigger asChild>
                    <button
                      type="button"
                      className="relative z-10 -m-1 shrink-0 self-center cursor-pointer rounded p-1.5 text-ember focus-visible:outline-2 focus-visible:outline-ring"
                      aria-label={t("card.issues")}
                    >
                      <TriangleAlert className="size-3.5" aria-hidden />
                    </button>
                  </PopoverTrigger>
                  <PopoverContent>{problem}</PopoverContent>
                </Popover>
              )}
              {node.public_note && (
                <Popover>
                  <PopoverTrigger asChild>
                    <button
                      type="button"
                      className="relative z-10 -m-1 shrink-0 self-center cursor-pointer rounded p-1.5 text-muted-foreground focus-visible:outline-2 focus-visible:outline-ring"
                      aria-label={t("card.note")}
                    >
                      <Info className="size-3.5" aria-hidden />
                    </button>
                  </PopoverTrigger>
                  <PopoverContent>{node.public_note}</PopoverContent>
                </Popover>
              )}
            </div>
            <span
              className={cn(
                "shrink-0 text-xs leading-6",
                !pending && !node.online ? "text-ember" : "text-muted-foreground",
              )}
            >
              {t(pending ? "node.pending" : node.online ? "node.online" : "node.offline")}
            </span>
          </div>
          {(os || specs) && (
            <p
              className="truncate text-xs text-muted-foreground"
              title={`${[os, specs].filter(Boolean).join(" / ")}${!node.online ? ` (${t("card.lastConfig")})` : ""}`}
            >
              {[os, specs].filter(Boolean).join(" / ")}
            </p>
          )}
        </div>

        {s ? (
          <div className="grid min-h-28 content-center gap-2.5">
            <div className="grid grid-cols-2 gap-x-3 gap-y-2.5">
              <UsageMeter
                label={t("metric.cpu")}
                pct={s.cpu}
                detail={`${s.load1.toFixed(2)}, ${s.load5.toFixed(2)}, ${s.load15.toFixed(2)}`}
                detailLabel={t("list.loadMinutes")}
                className="min-w-0"
              />
              <UsageMeter
                label={t("metric.mem")}
                pct={ratio(s.mem_used, s.mem_total)}
                detail={
                  s.mem_total > 0
                    ? t("card.usedTotal", { used: bytes(s.mem_used), total: bytes(s.mem_total) })
                    : t("card.unknown")
                }
                className="min-w-0"
              />
              <UsageMeter
                label={t("metric.disk")}
                pct={ratio(s.disk_used, s.disk_total)}
                detail={
                  s.disk_total > 0
                    ? t("card.usedTotal", { used: bytes(s.disk_used), total: bytes(s.disk_total) })
                    : t("card.unknown")
                }
                className="min-w-0"
              />
              {isVisible(node, "traffic") && tr.limit > 0 && (
                <QuotaHint traffic={tr} block>
                  <UsageMeter
                    label={t("traffic.quota")}
                    pct={ratio(tr.used, tr.limit)}
                    detail={t("card.usedTotal", { used: bytes(tr.used), total: bytes(tr.limit) })}
                    className="min-w-0"
                  />
                </QuotaHint>
              )}
            </div>
            <div className="flex min-w-0 items-center justify-between gap-2 text-xs text-muted-foreground">
              <NetRates node={node} />
              <span className="shrink-0 whitespace-nowrap" title={t("node.uptime", { d: uptime })}>
                <span className="sr-only">{t("node.uptime", { d: uptime })}</span>
                <span aria-hidden>{uptime}</span>
              </span>
            </div>
          </div>
        ) : (
          <div className="flex min-h-28 flex-col justify-center gap-1 rounded-lg bg-muted/40 px-3 py-2.5 text-xs text-muted-foreground">
            <p className="flex items-center gap-1.5 font-medium text-foreground/80">
              {!pending && !node.online && <CirclePause className="size-3.5 shrink-0" aria-hidden />}
              {t(pending ? "node.pending" : node.online ? "node.noData" : "card.offlineMetrics")}
            </p>
            {!pending && node.last_seen > 0 && (
              <time dateTime={new Date(node.last_seen * 1000).toISOString()}>
                {t("card.lastSeen", { time: dateTime(node.last_seen, lang) })}
              </time>
            )}
            {!pending && !node.online && (
              <NodeDetailLink
                uuid={node.uuid}
                className="relative z-10 mt-1 inline-flex min-h-7 w-fit items-center gap-1 rounded text-foreground underline decoration-muted-foreground/40 underline-offset-4 focus-visible:outline-2 focus-visible:outline-ring"
              >
                {t("card.viewDetails")}
                <ArrowUpRight className="size-3.5" aria-hidden />
              </NodeDetailLink>
            )}
          </div>
        )}

        {(isVisible(node, "traffic") || (isVisible(node, "expiry") && node.expire_at > 0)) && !pending && (
          <div className="grid gap-1 border-t pt-2.5 text-xs text-muted-foreground">
            {(isVisible(node, "traffic") || (isVisible(node, "expiry") && node.expire_at > 0)) && (
              <div className="flex min-w-0 items-center gap-1 text-[11px] tabular-nums">
                {isVisible(node, "traffic") && (
                  <div className="flex min-w-0 items-center gap-1.5 whitespace-nowrap">
                    <span className="shrink-0">{t("node.traffic")}</span>
                    <TrafficFigures traffic={tr} className="gap-1" />
                  </div>
                )}
                {isVisible(node, "expiry") && node.expire_at > 0 && (
                  <Expiry node={node} now={now} compact className="ms-auto shrink-0" />
                )}
              </div>
            )}
            {!s && isVisible(node, "traffic") && tr.limit > 0 && (
              <QuotaHint traffic={tr} block>
                <UsageBar
                  label={t("traffic.quota")}
                  pct={ratio(tr.used, tr.limit)}
                  detail={t("card.usedTotal", { used: bytes(tr.used), total: bytes(tr.limit) })}
                />
              </QuotaHint>
            )}
          </div>
        )}
        {node.online && isVisible(node, "ping") && <NetworkSummary probes={node.network ?? []} now={now} />}
        {(node.group || node.public_tags) && (
          <div className="mt-auto flex min-w-0 flex-wrap items-start gap-1.5 pt-1">
            {node.group && (
              <span
                className="max-w-full truncate rounded-md bg-muted px-1.5 py-0.5 text-xs text-muted-foreground"
                title={node.group}
              >
                {node.group}
              </span>
            )}
            <NodeTags value={node.public_tags} />
          </div>
        )}
      </div>
      {selection && (
        <div className="relative z-10 border-t px-3 py-2">
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start"
            aria-pressed={selection.selected}
            aria-label={t(selection.selected ? "compare.removeNode" : "compare.selectNode", { name: node.name })}
            disabled={selection.disabled}
            onClick={selection.onToggle}
          >
            {t(selection.selected ? "compare.selected" : "compare.select")}
          </Button>
        </div>
      )}
    </article>
  );
}
