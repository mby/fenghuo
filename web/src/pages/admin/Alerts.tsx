import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Plus } from "lucide-react";
import { type FormEvent, type ReactNode, useState } from "react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { DeliveryHistory } from "@/components/DeliveryHistory";
import { Field } from "@/components/Field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Textarea } from "@/components/ui/textarea";
import { ApiError, api, useAdminNodes } from "@/lib/api";
import { dateTime, duration, listSep } from "@/lib/format";
import { useI18n } from "@/lib/i18n";
import type { AlertEvent, Channel, Rule, RuleKind } from "@/lib/types";
import { cn } from "@/lib/utils";

const KINDS: RuleKind[] = ["offline", "cpu", "mem", "disk", "traffic", "expire", "latency", "packet_loss"];

export function Alerts() {
  return (
    <div className="grid gap-10">
      <Channels />
      <Rules />
      <Events />
      <DeliveryHistory />
    </div>
  );
}

function SectionHead({ title, hint, action }: { title: string; hint?: string; action?: ReactNode }) {
  return (
    <div className="mb-3 flex items-end justify-between gap-4">
      <div>
        <h2 className="text-lg font-medium">{title}</h2>
        {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
      </div>
      {action}
    </div>
  );
}

function errorText(e: unknown) {
  return e instanceof ApiError && e.message ? e.message : String(e);
}

function Channels() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const channels = useQuery({ queryKey: ["admin", "channels"], queryFn: () => api<Channel[]>("/admin/api/channels") });
  const [editing, setEditing] = useState<Channel | "new" | null>(null);
  const [removing, setRemoving] = useState<Channel | null>(null);

  const test = useMutation({
    mutationFn: (id: number) => api(`/admin/api/channels/${id}/test`, { method: "POST" }),
    onSuccess: () => toast.success(t("alerts.testSent")),
    onError: (e) => toast.error(errorText(e)),
  });
  const remove = useMutation({
    mutationFn: (id: number) => api(`/admin/api/channels/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["admin", "channels"] }),
  });

  const list = channels.data ?? [];
  return (
    <section>
      <SectionHead
        title={t("alerts.channels")}
        hint={t("alerts.channelsHint")}
        action={
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" aria-hidden />
            {t("alerts.addChannel")}
          </Button>
        }
      />
      {list.length === 0 && !channels.isPending ? (
        <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          {t("alerts.noChannels")}
        </p>
      ) : (
        <ul className="divide-y rounded-xl border bg-card">
          {list.map((c) => (
            <li key={c.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div className="min-w-0">
                <div className="flex items-center gap-2 font-medium">
                  {c.name}
                  <Badge variant="secondary">{c.type === "telegram" ? "Telegram" : "Webhook"}</Badge>
                  {!c.enabled && <Badge variant="outline">{t("common.disabled")}</Badge>}
                </div>
              </div>
              <div className="flex gap-1">
                <Button size="sm" variant="outline" onClick={() => test.mutate(c.id)} disabled={test.isPending}>
                  {t("alerts.test")}
                </Button>
                <Button size="sm" variant="ghost" onClick={() => setEditing(c)}>
                  {t("common.edit")}
                </Button>
                <Button size="sm" variant="ghost" className="text-destructive" onClick={() => setRemoving(c)}>
                  {t("common.delete")}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {editing && <ChannelDialog channel={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {removing && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setRemoving(null)}
          title={t("alerts.deleteChannel", { name: removing.name })}
          body=""
          confirm={t("common.delete")}
          destructive
          onConfirm={() => {
            remove.mutate(removing.id);
            setRemoving(null);
          }}
        />
      )}
    </section>
  );
}

function parseHeaders(text: string): Record<string, string> {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const i = line.indexOf(":");
    if (i > 0) out[line.slice(0, i).trim()] = line.slice(i + 1).trim();
  }
  return out;
}

function ChannelDialog({ channel, onClose }: { channel: Channel | null; onClose: () => void }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const [type, setType] = useState<Channel["type"]>(channel?.type ?? "telegram");
  const [name, setName] = useState(channel?.name ?? "");
  const [enabled, setEnabled] = useState(channel?.enabled ?? true);
  const tg = channel?.type === "telegram" ? channel.config : null;
  const wh = channel?.type === "webhook" ? channel.config : null;
  const [botToken, setBotToken] = useState(tg?.bot_token ?? "");
  const [chatId, setChatId] = useState(tg?.chat_id ?? "");
  const [apiBase, setApiBase] = useState(tg?.api_base ?? "");
  const [url, setUrl] = useState(wh?.url ?? "");
  const [method, setMethod] = useState<"GET" | "POST">(wh?.method ?? "POST");
  const [headers, setHeaders] = useState(
    Object.entries(wh?.headers ?? {})
      .map(([k, v]) => `${k}: ${v}`)
      .join("\n"),
  );
  const [body, setBody] = useState(wh?.body ?? "");
  const [error, setError] = useState("");

  const save = useMutation({
    mutationFn: (payload: unknown) =>
      channel
        ? api(`/admin/api/channels/${channel.id}`, { method: "PUT", body: payload })
        : api("/admin/api/channels", { method: "POST", body: payload }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "channels"] });
      toast.success(t("common.saved"));
      onClose();
    },
    onError: (e) => setError(errorText(e)),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    const config =
      type === "telegram"
        ? { bot_token: botToken, chat_id: chatId, ...(apiBase ? { api_base: apiBase } : {}) }
        : { url, method, headers: parseHeaders(headers), ...(body ? { body } : {}) };
    save.mutate({ name, type, config, enabled });
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{channel ? t("channel.editTitle") : t("channel.addTitle")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="ch-name" label={t("channel.name")}>
              <Input id="ch-name" value={name} onChange={(e) => setName(e.target.value)} required maxLength={64} />
            </Field>
            <Field id="ch-type" label={t("channel.type")}>
              <Select value={type} onValueChange={(v) => setType(v as Channel["type"])}>
                <SelectTrigger id="ch-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="telegram">Telegram</SelectItem>
                  <SelectItem value="webhook">Webhook</SelectItem>
                </SelectContent>
              </Select>
            </Field>
          </div>
          {type === "telegram" ? (
            <>
              <Field id="ch-bot" label={t("channel.botToken")}>
                <Input id="ch-bot" value={botToken} onChange={(e) => setBotToken(e.target.value)} required />
              </Field>
              <Field id="ch-chat" label={t("channel.chatId")}>
                <Input id="ch-chat" value={chatId} onChange={(e) => setChatId(e.target.value)} required />
              </Field>
              <Field id="ch-api" label={t("channel.apiBase")} hint={t("channel.apiBaseHint")}>
                <Input
                  id="ch-api"
                  value={apiBase}
                  onChange={(e) => setApiBase(e.target.value)}
                  placeholder="https://api.telegram.org"
                />
              </Field>
            </>
          ) : (
            <>
              <div className="grid gap-4 sm:grid-cols-[1fr_7rem]">
                <Field id="ch-url" label={t("channel.url")}>
                  <Input
                    id="ch-url"
                    value={url}
                    onChange={(e) => setUrl(e.target.value)}
                    required
                    placeholder="https://"
                  />
                </Field>
                <Field id="ch-method" label={t("channel.method")}>
                  <Select value={method} onValueChange={(v) => setMethod(v as "GET" | "POST")}>
                    <SelectTrigger id="ch-method" className="w-full">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="POST">POST</SelectItem>
                      <SelectItem value="GET">GET</SelectItem>
                    </SelectContent>
                  </Select>
                </Field>
              </div>
              <Field id="ch-headers" label={t("channel.headers")} hint={t("channel.headersHint")}>
                <Textarea
                  id="ch-headers"
                  value={headers}
                  onChange={(e) => setHeaders(e.target.value)}
                  rows={2}
                  className="font-mono text-xs"
                />
              </Field>
              {method === "POST" && (
                <Field id="ch-body" label={t("channel.body")} hint={t("channel.bodyHint")}>
                  <Textarea
                    id="ch-body"
                    value={body}
                    onChange={(e) => setBody(e.target.value)}
                    rows={4}
                    className="font-mono text-xs"
                    placeholder={'{"msgtype":"text","text":{"content":"{{title}}\\n{{text}}"}}'}
                  />
                </Field>
              )}
            </>
          )}
          <div className="flex items-center gap-2">
            <Switch id="ch-enabled" checked={enabled} onCheckedChange={setEnabled} />
            <label htmlFor="ch-enabled" className="text-sm">
              {t("common.enabled")}
            </label>
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function useConditionText() {
  const { t, lang } = useI18n();
  return (r: Rule) => {
    const d = duration(r.duration, lang);
    switch (r.kind) {
      case "offline":
        return t("cond.offline", { d });
      case "cpu":
      case "mem":
      case "disk": {
        const m = t(`kind.${r.kind}`);
        return r.duration > 0 ? t("cond.usage", { m, v: r.threshold, d }) : t("cond.usageNow", { m, v: r.threshold });
      }
      case "traffic":
        return t("cond.traffic", { v: r.threshold });
      case "expire":
        return t("cond.expire", { v: r.threshold });
      case "latency":
      case "packet_loss":
        return t("network.condition", {
          target: r.target || t("network.anyTarget"),
          v: r.threshold,
          unit: r.kind === "latency" ? "ms" : "%",
          d,
        });
    }
  };
}

function Rules() {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const rules = useQuery({ queryKey: ["admin", "rules"], queryFn: () => api<Rule[]>("/admin/api/rules") });
  const [editing, setEditing] = useState<Rule | "new" | null>(null);
  const [removing, setRemoving] = useState<Rule | null>(null);
  const condition = useConditionText();
  const refresh = () => qc.invalidateQueries({ queryKey: ["admin", "rules"] });

  const toggle = useMutation({
    mutationFn: (r: Rule) => api(`/admin/api/rules/${r.id}`, { method: "PUT", body: { ...r, enabled: !r.enabled } }),
    onSuccess: refresh,
    onError: (e) => toast.error(errorText(e)),
  });
  const remove = useMutation({
    mutationFn: (id: number) => api(`/admin/api/rules/${id}`, { method: "DELETE" }),
    onSuccess: refresh,
  });

  const list = rules.data ?? [];
  return (
    <section>
      <SectionHead
        title={t("alerts.rules")}
        action={
          <Button size="sm" onClick={() => setEditing("new")}>
            <Plus className="size-4" aria-hidden />
            {t("alerts.addRule")}
          </Button>
        }
      />
      {list.length === 0 && !rules.isPending ? (
        <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          {t("alerts.noRules")}
        </p>
      ) : (
        <ul className="divide-y rounded-xl border bg-card">
          {list.map((r) => (
            <li key={r.id} className="flex flex-wrap items-center justify-between gap-3 px-4 py-3">
              <div className={cn("min-w-0", !r.enabled && "opacity-60")}>
                <div className="font-medium">{r.name}</div>
                <div className="text-sm text-muted-foreground">
                  {condition(r)}
                  {listSep(lang)}
                  {r.node_ids.length ? t("rule.someNodes", { n: r.node_ids.length }) : t("rule.allNodes")}
                </div>
              </div>
              <div className="flex items-center gap-1">
                <Switch
                  checked={r.enabled}
                  onCheckedChange={() => toggle.mutate(r)}
                  aria-label={t("common.enabled")}
                  className="mr-2"
                />
                <Button size="sm" variant="ghost" onClick={() => setEditing(r)}>
                  {t("common.edit")}
                </Button>
                <Button size="sm" variant="ghost" className="text-destructive" onClick={() => setRemoving(r)}>
                  {t("common.delete")}
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}
      {editing && <RuleDialog rule={editing === "new" ? null : editing} onClose={() => setEditing(null)} />}
      {removing && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setRemoving(null)}
          title={t("alerts.deleteRule", { name: removing.name })}
          body=""
          confirm={t("common.delete")}
          destructive
          onConfirm={() => {
            remove.mutate(removing.id);
            setRemoving(null);
          }}
        />
      )}
    </section>
  );
}

const DEFAULTS: Record<RuleKind, { threshold: number; minutes: number }> = {
  offline: { threshold: 0, minutes: 3 },
  cpu: { threshold: 90, minutes: 5 },
  mem: { threshold: 90, minutes: 5 },
  disk: { threshold: 90, minutes: 0 },
  traffic: { threshold: 90, minutes: 0 },
  expire: { threshold: 3, minutes: 0 },
  latency: { threshold: 200, minutes: 2 },
  packet_loss: { threshold: 20, minutes: 2 },
};

function RuleDialog({ rule, onClose }: { rule: Rule | null; onClose: () => void }) {
  const { t } = useI18n();
  const qc = useQueryClient();
  const nodes = useAdminNodes();
  const [kind, setKind] = useState<RuleKind>(rule?.kind ?? "offline");
  const [name, setName] = useState(rule?.name ?? "");
  const [target, setTarget] = useState(rule?.target ?? "");
  const [threshold, setThreshold] = useState(String(rule?.threshold ?? DEFAULTS.offline.threshold));
  const [minutes, setMinutes] = useState(String(rule ? rule.duration / 60 : DEFAULTS.offline.minutes));
  const [nodeIds, setNodeIds] = useState<number[]>(rule?.node_ids ?? []);
  const [enabled, setEnabled] = useState(rule?.enabled ?? true);
  const [error, setError] = useState("");

  const save = useMutation({
    mutationFn: (payload: unknown) =>
      rule
        ? api(`/admin/api/rules/${rule.id}`, { method: "PUT", body: payload })
        : api("/admin/api/rules", { method: "POST", body: payload }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["admin", "rules"] });
      toast.success(t("common.saved"));
      onClose();
    },
    onError: (e) => setError(errorText(e)),
  });

  function changeKind(k: RuleKind) {
    setKind(k);
    if (!rule) {
      setThreshold(String(DEFAULTS[k].threshold));
      setMinutes(String(DEFAULTS[k].minutes));
    }
  }

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    save.mutate({
      name: name || t(`kind.${kind}`),
      kind,
      target: kind === "latency" || kind === "packet_loss" ? target : "",
      threshold: Number(threshold) || 0,
      duration: Math.round((Number(minutes) || 0) * 60),
      node_ids: nodeIds,
      enabled,
    });
  }

  const hasThreshold = kind !== "offline";
  const networkRule = kind === "latency" || kind === "packet_loss";
  const hasDuration = kind === "offline" || kind === "cpu" || kind === "mem" || kind === "disk" || networkRule;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{rule ? t("rule.editTitle") : t("rule.addTitle")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="r-kind" label={t("rule.kind")}>
              <Select value={kind} onValueChange={(v) => changeKind(v as RuleKind)}>
                <SelectTrigger id="r-kind" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {KINDS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(`kind.${k}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <Field id="r-name" label={t("rule.name")}>
              <Input
                id="r-name"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder={t(`kind.${kind}`)}
                maxLength={64}
              />
            </Field>
            {hasThreshold && (
              <Field
                id="r-threshold"
                label={
                  kind === "expire"
                    ? t("rule.days")
                    : kind === "latency"
                      ? t("network.latencyThreshold")
                      : t("rule.threshold")
                }
              >
                <Input
                  id="r-threshold"
                  type="number"
                  min={0}
                  max={kind === "expire" ? 365 : kind === "latency" ? 60000 : 100}
                  step="any"
                  value={threshold}
                  onChange={(e) => setThreshold(e.target.value)}
                  required
                />
              </Field>
            )}
            {hasDuration && (
              <Field id="r-minutes" label={t("rule.duration")}>
                <Input
                  id="r-minutes"
                  type="number"
                  min={kind === "offline" ? 0.5 : 0}
                  step="any"
                  value={minutes}
                  onChange={(e) => setMinutes(e.target.value)}
                  required
                />
              </Field>
            )}
          </div>
          {networkRule && (
            <Field id="r-target" label={t("network.target")} hint={t("network.targetHint")}>
              <Input
                id="r-target"
                maxLength={32}
                value={target}
                onChange={(e) => setTarget(e.target.value)}
                placeholder={t("network.anyTarget")}
              />
            </Field>
          )}
          <fieldset className="grid gap-2">
            <legend className="mb-1.5 text-sm font-medium">{t("rule.nodes")}</legend>
            <label className="flex items-center gap-2 text-sm">
              <input
                type="checkbox"
                className="size-4 accent-foreground"
                checked={nodeIds.length === 0}
                onChange={() => setNodeIds([])}
              />
              {t("rule.allNodes")}
            </label>
            <div className="grid max-h-40 gap-1.5 overflow-y-auto sm:grid-cols-2">
              {(nodes.data ?? []).map((n) => (
                <label key={n.id} className="flex items-center gap-2 text-sm">
                  <input
                    type="checkbox"
                    className="size-4 accent-foreground"
                    checked={nodeIds.includes(n.id)}
                    onChange={(e) =>
                      setNodeIds((ids) => (e.target.checked ? [...ids, n.id] : ids.filter((i) => i !== n.id)))
                    }
                  />
                  <span className="truncate">{n.name}</span>
                </label>
              ))}
            </div>
          </fieldset>
          <div className="flex items-center gap-2">
            <Switch id="r-enabled" checked={enabled} onCheckedChange={setEnabled} />
            <label htmlFor="r-enabled" className="text-sm">
              {t("common.enabled")}
            </label>
          </div>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function Events() {
  const { t, lang } = useI18n();
  const events = useQuery({
    queryKey: ["admin", "events"],
    queryFn: () => api<AlertEvent[]>("/admin/api/events?limit=100"),
    refetchInterval: 30_000,
  });
  const list = events.data ?? [];
  return (
    <section>
      <SectionHead title={t("alerts.events")} />
      {list.length === 0 && !events.isPending ? (
        <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
          {t("alerts.noEvents")}
        </p>
      ) : (
        <ul className="divide-y rounded-xl border bg-card">
          {list.map((e) => (
            <li key={e.id} className="flex items-start gap-3 px-4 py-2.5 text-sm">
              <span
                className={cn(
                  "mt-0.5 shrink-0 rounded px-1.5 py-0.5 text-xs",
                  e.firing ? "bg-ember/15 text-ember" : "bg-jade/15 text-jade",
                )}
              >
                {e.firing ? t("alerts.firing") : t("alerts.resolved")}
              </span>
              <span className="min-w-0 flex-1">{e.message}</span>
              <time className="shrink-0 text-xs text-muted-foreground tabular-nums">
                {dateTime(e.created_at, lang)}
              </time>
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}
