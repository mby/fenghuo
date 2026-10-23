import { ArrowDown, ArrowUp } from "lucide-react";
import type { ReactNode } from "react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { bytes, ratio } from "@/lib/format";
import { level } from "@/lib/health";
import { useI18n } from "@/lib/i18n";
import { trafficResetTime } from "@/lib/traffic-reset";
import type { Traffic } from "@/lib/types";
import { cn } from "@/lib/utils";

function counted(tr: Traffic, dir: "up" | "down"): boolean {
  switch (tr.mode) {
    case "up":
    case "down":
      return tr.mode === dir;
    case "max":
      return dir === "up" ? tr.up >= tr.down : tr.down > tr.up;
    default:
      return true;
  }
}

/**
 * Upload and download totals for the billing period. The direction that
 * counts toward the quota is shown at full strength, the other one dimmed.
 */
export function TrafficFigures({ traffic: tr, className }: { traffic: Traffic; className?: string }) {
  const { t } = useI18n();
  const dir = (d: "up" | "down") => (counted(tr, d) ? "text-foreground" : "text-muted-foreground/60");
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span className={cn("relative z-10 inline-flex cursor-help items-center gap-2.5 tabular-nums", className)}>
          <span className={cn("flex items-center gap-0.5 whitespace-nowrap", dir("up"))}>
            <ArrowUp className="size-3 shrink-0" aria-label={t("series.out")} />
            {bytes(tr.up)}
          </span>
          <span className={cn("flex items-center gap-0.5 whitespace-nowrap", dir("down"))}>
            <ArrowDown className="size-3 shrink-0" aria-label={t("series.in")} />
            {bytes(tr.down)}
          </span>
        </span>
      </TooltipTrigger>
      <TooltipContent>
        <ResetDetails traffic={tr} />
      </TooltipContent>
    </Tooltip>
  );
}

/** "120 GB / 1.0 TB" against the quota; empty when there is no limit. */
export function QuotaText({ traffic: tr, className }: { traffic: Traffic; className?: string }) {
  if (tr.limit <= 0) return null;
  const lv = level(ratio(tr.used, tr.limit));
  return (
    <QuotaHint traffic={tr}>
      <span
        className={cn(
          "whitespace-nowrap tabular-nums",
          lv === "hot" && "text-ember",
          lv === "warn" && "text-amber",
          className,
        )}
      >
        {bytes(tr.used)} / {bytes(tr.limit)}
      </span>
    </QuotaHint>
  );
}

export function QuotaHint({
  traffic,
  children,
  block = false,
}: {
  traffic: Traffic;
  children: ReactNode;
  block?: boolean;
}) {
  const Trigger = block ? "div" : "span";
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Trigger className={cn("relative z-10 cursor-help", block ? "block" : "inline-flex")}>{children}</Trigger>
      </TooltipTrigger>
      <TooltipContent>
        <ResetDetails traffic={traffic} />
      </TooltipContent>
    </Tooltip>
  );
}

function ResetDetails({ traffic }: { traffic: Traffic }) {
  const { t, lang } = useI18n();
  const reset = trafficResetTime(traffic, lang);
  return (
    <div className="grid max-w-72 gap-1.5 text-left leading-relaxed">
      <p className="font-medium">{t("traffic.nextReset")}</p>
      {reset ? (
        <time dateTime={reset.iso}>
          {reset.date} ({reset.zone})
        </time>
      ) : (
        <p>{t("traffic.resetUnknown")}</p>
      )}
      <p>{t("traffic.resetSchedule", { d: traffic.reset_day })}</p>
      {traffic.reset_day > 28 && <p>{t("traffic.shortMonth")}</p>}
      <p>{t("traffic.countedMode", { mode: t(`modeShort.${traffic.mode}`) })}</p>
    </div>
  );
}
