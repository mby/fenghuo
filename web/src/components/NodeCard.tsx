import { Info } from "lucide-react";
import { NodeDetailLink } from "@/components/NodeDetailLink";
import { Button } from "@/components/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
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
import { UsageBar } from "./UsageBar";

export interface NodeSelection {
  selected: boolean;
  disabled: boolean;
  onToggle: () => void;
}

export function NodeCard({ node, now, selection }: { node: PublicNode; now: number; selection?: NodeSelection }) {
  const { t, lang } = useI18n();
  const lit = issues(node).length > 0;
  const pending = isPending(node);
  const s = node.online ? node.status : null;
  const os = isVisible(node, "system") ? shortOS(node.info.os) : "";
  const specs = isVisible(node, "system")
    ? [node.info.arch, compactCapacity(node), node.info.virt].filter((v) => v && v !== "none").join(" / ")
    : "";
  const tr = node.traffic;

  return (
    <article
      className={cn(
        "relative flex w-full min-w-0 flex-col rounded-xl border bg-card transition-colors hover:bg-accent/20",
        lit && "lit border-transparent",
        selection?.selected && "ring-2 ring-foreground/50",
      )}
    >
      <div className="flex flex-col gap-2.5 p-4 text-card-foreground">
        <div className="grid min-w-0 grid-cols-1 gap-1">
          <div className="flex items-center justify-between gap-3">
            <div className="flex min-w-0 flex-1 items-center gap-2">
              <StatusDot state={pending ? "pending" : lit ? "lit" : "ok"} />
              <h2 className="min-w-0 truncate font-medium">
                <NodeDetailLink
                  uuid={node.uuid}
                  title={node.name}
                  className="after:absolute after:inset-0 after:rounded-xl focus-visible:outline-none focus-visible:after:outline-2 focus-visible:after:outline-ring"
                >
                  {node.name}
                </NodeDetailLink>
              </h2>
              {node.public_note && (
                <Tooltip>
                  <TooltipTrigger asChild>
                    <button
                      type="button"
                      className="relative z-10 -m-1 shrink-0 cursor-help rounded p-1 text-muted-foreground"
                      aria-label={t("card.note")}
                    >
                      <Info className="size-3.5" aria-hidden />
                    </button>
                  </TooltipTrigger>
                  <TooltipContent className="max-w-64 text-left leading-relaxed" sideOffset={6}>
                    {node.public_note}
                  </TooltipContent>
                </Tooltip>
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
          <div className="grid gap-2.5">
            {lit && (
              <p className="truncate text-xs text-ember" title={issueText(node, now, t, lang)}>
                {issueText(node, now, t, lang)}
              </p>
            )}
            <div className="grid gap-2">
              <UsageBar label={t("metric.cpu")} pct={s.cpu} />
              <UsageBar
                label={t("metric.mem")}
                pct={ratio(s.mem_used, s.mem_total)}
                detail={
                  s.mem_total > 0
                    ? t("card.usedTotal", { used: bytes(s.mem_used), total: bytes(s.mem_total) })
                    : t("card.unknown")
                }
              />
              <UsageBar
                label={t("metric.disk")}
                pct={ratio(s.disk_used, s.disk_total)}
                detail={
                  s.disk_total > 0
                    ? t("card.usedTotal", { used: bytes(s.disk_used), total: bytes(s.disk_total) })
                    : t("card.unknown")
                }
              />
            </div>
            <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 text-xs text-muted-foreground">
              <NetRates node={node} />
              <span className="whitespace-nowrap">{t("node.uptime", { d: duration(s.uptime, lang) })}</span>
            </div>
          </div>
        ) : (
          <div className="grid gap-1 text-xs text-muted-foreground">
            <p>{t(pending ? "node.pending" : node.online ? "node.noData" : "card.offlineMetrics")}</p>
            {!pending && node.last_seen > 0 && (
              <time dateTime={new Date(node.last_seen * 1000).toISOString()}>
                {t("card.lastSeen", { time: dateTime(node.last_seen, lang) })}
              </time>
            )}
          </div>
        )}

        {(isVisible(node, "traffic") || (isVisible(node, "expiry") && node.expire_at > 0)) && !pending && (
          <div className="grid gap-2 border-t pt-2.5 text-xs text-muted-foreground">
            <div className="flex flex-wrap items-center justify-between gap-x-2 gap-y-1">
              {isVisible(node, "traffic") && (
                <span className="flex flex-wrap items-center gap-2">
                  {t("node.traffic")}
                  <TrafficFigures traffic={tr} />
                </span>
              )}
              {isVisible(node, "expiry") && <Expiry node={node} now={now} />}
            </div>
            {isVisible(node, "traffic") && tr.limit > 0 && (
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
          <div className="flex min-w-0 flex-wrap items-start gap-1.5">
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
