import { useQuery } from "@tanstack/react-query";
import { Button } from "@/components/ui/button";
import { api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { type MessageKey, useI18n } from "@/lib/i18n";
import type { Channel, Delivery } from "@/lib/types";
import { cn } from "@/lib/utils";

const ERROR_KEYS: Record<string, string> = {
  timeout: "delivery.error.timeout",
  canceled: "delivery.error.canceled",
  transport_error: "delivery.error.transport",
  invalid_config: "delivery.error.config",
  invalid_payload: "delivery.error.payload",
  http_error: "delivery.error.http",
  telegram_error: "delivery.error.telegram",
  channel_unavailable: "delivery.error.channel",
  interrupted: "delivery.error.interrupted",
};

export function DeliveryHistory() {
  const { t, lang } = useI18n();
  const query = useQuery({
    queryKey: ["admin", "deliveries"],
    queryFn: () => api<Delivery[]>("/admin/api/deliveries?limit=100"),
    refetchInterval: 10_000,
  });
  const channels = useQuery({ queryKey: ["admin", "channels"], queryFn: () => api<Channel[]>("/admin/api/channels") });
  const channelNames = new Map(channels.data?.map((c) => [c.id, c.name]));
  const errorLabel = (code: string) => t((ERROR_KEYS[code] ?? "delivery.error.other") as MessageKey);
  return (
    <section aria-labelledby="delivery-title">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="delivery-title" className="text-lg font-medium">
            {t("delivery.title")}
          </h2>
          <p className="mt-1 max-w-prose text-sm text-muted-foreground">{t("delivery.hint")}</p>
        </div>
        <Button variant="outline" size="sm" onClick={() => void query.refetch()} disabled={query.isFetching}>
          {t("delivery.refresh")}
        </Button>
      </div>
      {query.isPending && <p className="text-sm text-muted-foreground">{t("hero.loading")}</p>}
      {query.isError && (
        <p role="alert" className="text-sm text-destructive">
          {t("error.load")}
        </p>
      )}
      {!query.isError && query.data?.length === 0 && (
        <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          {t("delivery.empty")}
        </p>
      )}
      {!query.isError && !!query.data?.length && (
        <ul className="divide-y rounded-xl border bg-card">
          {query.data.map((d) => (
            <li key={d.id} className="px-4 py-3">
              <div className="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
                <div className="min-w-0">
                  <p className="break-words text-sm font-medium">
                    {channelNames.get(d.channel_id) ?? t("delivery.channel", { id: d.channel_id })}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground tabular-nums">
                    {dateTime(d.created_at, lang)} · {t(d.firing ? "delivery.firing" : "delivery.resolved")}
                  </p>
                </div>
                <div className="text-end">
                  <p className={cn("text-sm", d.status === "failed" && "text-ember")}>
                    {t(`delivery.status.${d.status}`)}
                  </p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {t("delivery.attempts", { n: d.attempts, max: d.max_attempts })}
                  </p>
                </div>
              </div>
              {d.last_error && <p className="mt-2 text-xs text-muted-foreground">{errorLabel(d.last_error)}</p>}
              {d.next_attempt_at > 0 && d.status === "retrying" && (
                <p className="mt-1 text-xs text-muted-foreground">
                  {t("delivery.next", { time: dateTime(d.next_attempt_at, lang) })}
                </p>
              )}
              {d.history.length > 0 && (
                <details className="mt-2 text-xs">
                  <summary className="w-fit cursor-pointer py-1 text-muted-foreground">{t("delivery.history")}</summary>
                  <ol className="mt-1 grid gap-2 border-t pt-2">
                    {d.history.map((a) => (
                      <li key={a.number} className="flex flex-wrap justify-between gap-2">
                        <span className="tabular-nums">
                          {t("delivery.attempt", { n: a.number })} · {dateTime(a.started_at, lang)}
                        </span>
                        <span>
                          {a.error
                            ? errorLabel(a.error)
                            : a.status === "succeeded"
                              ? t("delivery.status.succeeded")
                              : t("delivery.status.sending")}
                        </span>
                      </li>
                    ))}
                  </ol>
                </details>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
