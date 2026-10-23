import { type Level, level } from "@/lib/health";
import { cn } from "@/lib/utils";

function Bar({ pct, lv }: { pct: number; lv: Level }) {
  return (
    <div className="h-1.5 overflow-hidden rounded-full bg-muted" aria-hidden>
      <div
        className={cn(
          "h-full rounded-full transition-[width] duration-500",
          lv === "ok" && "bg-foreground/55",
          lv === "warn" && "bg-amber",
          lv === "hot" && "bg-ember",
        )}
        style={{ width: `${Math.min(100, Math.max(0, pct))}%` }}
      />
    </div>
  );
}

/** Label, bar and value on one line; used in server cards. */
export function UsageBar({
  label,
  pct,
  detail,
  className,
}: {
  label: string;
  pct: number;
  detail?: string;
  className?: string;
}) {
  const lv = level(pct);
  return (
    <div className={cn("grid grid-cols-[3.25rem_1fr_auto] items-center gap-2 text-xs", className)}>
      <span className="text-muted-foreground">{label}</span>
      <Bar pct={pct} lv={lv} />
      <span className={cn("min-w-[2.75rem] text-right tabular-nums", lv === "hot" && "text-ember")}>
        {detail ?? `${Math.round(pct)}%`}
      </span>
    </div>
  );
}

/** Value above a bar; used in list rows where columns are narrow. */
export function UsageMeter({
  label,
  pct,
  detail,
  detailLabel,
  className,
}: {
  label?: string;
  pct: number;
  detail?: string;
  detailLabel?: string;
  className?: string;
}) {
  const lv = level(pct);
  return (
    <div className={cn("grid content-start gap-1 text-xs", className)}>
      <div className="flex items-baseline justify-between gap-2">
        {label && <span className="text-muted-foreground">{label}</span>}
        <span className={cn("ms-auto tabular-nums", lv === "hot" && "text-ember")}>{Math.round(pct)}%</span>
      </div>
      <Bar pct={pct} lv={lv} />
      <span
        aria-hidden={!detail}
        title={detailLabel}
        className="min-h-[1.5em] text-[11px] leading-relaxed tabular-nums text-muted-foreground"
      >
        {detail && detailLabel && <span className="sr-only">{detailLabel}: </span>}
        {detail}
      </span>
    </div>
  );
}
