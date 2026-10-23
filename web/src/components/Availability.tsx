import { useQuery } from "@tanstack/react-query";
import { useId, useState } from "react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError, api } from "@/lib/api";
import { type MessageKey, useI18n } from "@/lib/i18n";
import type { AvailabilitySummary } from "@/lib/types";
import { cn } from "@/lib/utils";

export function Availability({ uuid }: { uuid: string }) {
  const { t, lang } = useI18n();
  const titleId = useId();
  const [range, setRange] = useState<"7d" | "30d" | "90d">("7d");
  type AvailabilityKey =
    Extract<MessageKey, `availability.${string}`> extends `availability.${infer Key}` ? Key : never;
  const message = (key: AvailabilityKey, vars?: Record<string, string | number>) => t(`availability.${key}`, vars);
  const query = useQuery({
    queryKey: ["availability", uuid, range],
    queryFn: () => api<AvailabilitySummary>(`/api/v1/nodes/${encodeURIComponent(uuid)}/availability?range=${range}`),
    enabled: !!uuid,
    refetchInterval: 60_000,
    staleTime: 30_000,
    retry: (count, err) => !(err instanceof ApiError && err.status === 404) && count < 2,
  });
  const data = query.data;
  const locale = lang === "zh" ? "zh-CN" : "en";
  const number = new Intl.NumberFormat(locale, { maximumFractionDigits: 2 });
  const percent = (value: number) =>
    new Intl.NumberFormat(locale, { style: "percent", maximumFractionDigits: 2 }).format(value / 100);
  const duration = (seconds: number) => {
    if (seconds >= 86400) return message("days", { n: number.format(seconds / 86400) });
    if (seconds >= 3600) return message("hours", { n: number.format(seconds / 3600) });
    if (seconds >= 60) return message("minutes", { n: number.format(seconds / 60) });
    return message("seconds", { n: number.format(seconds) });
  };
  const dateTime = (seconds: number) =>
    new Date(seconds * 1000).toLocaleString(locale, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
    });

  return (
    <section aria-labelledby={titleId} className="mt-8 border-t pt-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 id={titleId} className="text-base font-medium">
          {message("title")}
        </h2>
        <fieldset aria-label={message("range")} className="flex gap-1">
          {(["7d", "30d", "90d"] as const).map((value) => (
            <Button
              key={value}
              type="button"
              variant={range === value ? "secondary" : "ghost"}
              aria-pressed={range === value}
              onClick={() => setRange(value)}
              className="min-h-10"
            >
              {t(`range.${value}`)}
            </Button>
          ))}
        </fieldset>
      </div>
      <p className="mt-2 max-w-prose text-sm text-muted-foreground">{message("method")}</p>
      {query.isPending && (
        <div role="status" className="mt-4">
          <span className="sr-only">{message("loading")}</span>
          <Skeleton className="h-28 w-full rounded-md" />
        </div>
      )}
      {query.isError && (
        <div role="alert" className="mt-4 flex flex-wrap items-center gap-3 text-sm">
          <p>{message("error")}</p>
          <Button type="button" variant="outline" onClick={() => void query.refetch()} disabled={query.isFetching}>
            {message("retry")}
          </Button>
        </div>
      )}
      {data && !query.isError && (
        <>
          <dl className="mt-5 grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-3">
            {[
              ["uptime", data.uptime_percent === null ? message("noKnown") : percent(data.uptime_percent)],
              ["coverage", percent(data.coverage_percent)],
              ["unknown", duration(data.unknown_seconds)],
              ["offline", duration(data.offline_seconds)],
              ["outages", number.format(data.outage_count)],
              ["longest", duration(data.longest_outage_seconds)],
            ].map(([key, value]) => (
              <div key={key} className="min-w-0">
                <dt className="text-muted-foreground">{message(key as AvailabilityKey)}</dt>
                <dd className="mt-1 break-words font-medium tabular-nums">{value}</dd>
              </div>
            ))}
          </dl>
          {data.uptime_percent === null && <p className="mt-4 text-sm text-muted-foreground">{message("empty")}</p>}
          <details className="mt-5">
            <summary className="w-fit cursor-pointer rounded-sm py-2 text-sm font-medium focus-visible:outline-2 focus-visible:outline-ring focus-visible:outline-offset-2">
              {message("timeline")}
            </summary>
            <p className="mt-1 max-w-prose text-xs text-muted-foreground">{message("timelineHelp")}</p>
            {data.timeline_truncated && <p className="mt-2 text-xs text-muted-foreground">{message("truncated")}</p>}
            <ol className="mt-3 divide-y">
              {[...data.timeline].reverse().map((span) => (
                <li
                  key={`${span.start}-${span.end}`}
                  className="flex flex-wrap items-start justify-between gap-x-4 gap-y-1 py-3 text-sm"
                >
                  <div className="min-w-0">
                    <p className={cn("font-medium", span.state === "offline" && "text-ember")}>{message(span.state)}</p>
                    <p className="mt-1 text-xs text-muted-foreground">
                      <time dateTime={new Date(span.start * 1000).toISOString()}>{dateTime(span.start)}</time>
                      {message("to")}
                      <time dateTime={new Date(span.end * 1000).toISOString()}>{dateTime(span.end)}</time>
                    </p>
                  </div>
                  <span className="shrink-0 tabular-nums text-muted-foreground">{duration(span.end - span.start)}</span>
                </li>
              ))}
            </ol>
          </details>
        </>
      )}
    </section>
  );
}
