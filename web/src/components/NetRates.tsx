import { ArrowDown, ArrowUp } from "lucide-react";
import { rate } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
import type { PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Offline/missing samples are unknown, not measured zero throughput. */
export function NetRates({ node, stacked, className }: { node: PublicNode; stacked?: boolean; className?: string }) {
  const { t } = useI18n();
  const s = node.online ? node.status : null;
  return (
    <span
      className={cn("flex tabular-nums", stacked ? "flex-col gap-0.5" : "flex-wrap items-center gap-x-3", className)}
    >
      <span className="flex items-center gap-1 whitespace-nowrap" title={t("series.out")}>
        <ArrowUp className="size-3 shrink-0" aria-label={t("series.out")} />
        {s ? rate(s.net_out) : t("card.unknown")}
      </span>
      <span className="flex items-center gap-1 whitespace-nowrap" title={t("series.in")}>
        <ArrowDown className="size-3 shrink-0" aria-label={t("series.in")} />
        {s ? rate(s.net_in) : t("card.unknown")}
      </span>
    </span>
  );
}
