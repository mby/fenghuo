import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { EyeOff, MoreHorizontal, Plus } from "lucide-react";
import { type FormEvent, useState } from "react";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { CopyField } from "@/components/CopyField";
import { Expiry } from "@/components/Expiry";
import { Field } from "@/components/Field";
import { NodeTags } from "@/components/NodeTags";
import { StatusDot } from "@/components/StatusDot";
import { QuotaText, TrafficFigures } from "@/components/Traffic";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import { ApiError, api, useAdminNodes } from "@/lib/api";
import { date, fromDateInput, GB, toDateInput } from "@/lib/format";
import { isPending, issues } from "@/lib/health";
import { useI18n } from "@/lib/i18n";
import { probeInstallFlags } from "@/lib/probes";
import type { AdminNode, CostSummary, NodeInput, Settings, TrafficMode } from "@/lib/types";

const INSTALL_SCRIPT = "https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh";

export function Servers() {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const nodes = useAdminNodes();
  const [editing, setEditing] = useState<AdminNode | "new" | null>(null);
  const [token, setToken] = useState<string | null>(null);
  const [search, setSearch] = useState("");
  const costs = useQuery({ queryKey: ["admin", "costs"], queryFn: () => api<CostSummary[]>("/admin/api/costs") });
  const [confirm, setConfirm] = useState<{ node: AdminNode; action: "delete" | "rotate" } | null>(null);

  const refresh = () => {
    qc.invalidateQueries({ queryKey: ["admin", "nodes"] });
    qc.invalidateQueries({ queryKey: ["admin", "costs"] });
    qc.invalidateQueries({ queryKey: ["nodes"] });
  };

  const rotate = useMutation({
    mutationFn: (id: number) => api<{ token: string }>(`/admin/api/nodes/${id}/token`, { method: "POST" }),
    onSuccess: (r) => {
      setToken(r.token);
      refresh();
    },
    onError: (e) => toast.error(t("common.error", { m: e.message })),
  });
  const remove = useMutation({
    mutationFn: (id: number) => api(`/admin/api/nodes/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success(t("servers.deleted"));
      refresh();
    },
    onError: (e) => toast.error(t("common.error", { m: e.message })),
  });

  const list = (nodes.data ?? []).filter((n) =>
    [n.name, n.group, n.provider, n.region, n.tags]
      .join(" ")
      .toLocaleLowerCase()
      .includes(search.trim().toLocaleLowerCase()),
  );
  const money = (value: number, currency: string) =>
    `${value.toLocaleString(lang === "zh" ? "zh-CN" : "en-US", { minimumFractionDigits: 2, maximumFractionDigits: 2 })} ${currency}`;

  return (
    <div className="grid gap-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-medium">{t("admin.servers")}</h2>
        <Button size="sm" onClick={() => setEditing("new")}>
          <Plus className="size-4" aria-hidden />
          {t("servers.add")}
        </Button>
      </div>

      <section className="grid gap-3 rounded-xl border bg-card p-4 sm:p-5" aria-label={t("assets.costs")}>
        <div>
          <h3 className="font-medium">{t("assets.costs")}</h3>
          <p className="mt-1 text-xs text-muted-foreground">{t("assets.costHint")}</p>
        </div>
        {costs.isPending ? (
          <p className="text-sm text-muted-foreground">{t("hero.loading")}</p>
        ) : costs.isError ? (
          <p role="alert" className="text-sm text-destructive">
            {t("error.load")}
          </p>
        ) : costs.data?.length ? (
          <dl className="flex flex-wrap gap-x-10 gap-y-4">
            {costs.data.map((c) => (
              <div key={c.currency}>
                <dt className="text-xs text-muted-foreground">
                  {t("assets.costNodes", { n: c.nodes, currency: c.currency })}
                </dt>
                <dd className="mt-1 text-xl font-medium tabular-nums">
                  {money(c.monthly, c.currency)}{" "}
                  <span className="text-xs font-normal text-muted-foreground">{t("assets.perMonth")}</span>
                </dd>
                <dd className="mt-1 text-xs text-muted-foreground tabular-nums">
                  {money(c.annual, c.currency)} {t("assets.perYear")}
                </dd>
              </div>
            ))}
          </dl>
        ) : (
          <p className="text-sm text-muted-foreground">{t("assets.costEmpty")}</p>
        )}
      </section>
      <div className="max-w-md">
        <label className="sr-only" htmlFor="asset-search">
          {t("assets.search")}
        </label>
        <Input
          id="asset-search"
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder={t("assets.search")}
        />
      </div>
      {nodes.isError && (
        <p role="alert" className="text-sm text-destructive">
          {t("error.load")}
        </p>
      )}
      {nodes.isPending && <p className="text-sm text-muted-foreground">{t("hero.loading")}</p>}

      {list.length === 0 && !nodes.isPending && !nodes.isError ? (
        <p className="rounded-xl border border-dashed p-10 text-center text-sm text-muted-foreground">
          {search ? t("assets.noResults") : t("servers.empty")}
        </p>
      ) : (
        <>
          <ul className="divide-y rounded-xl border bg-card md:hidden">
            {list.map((n) => {
              const pending = isPending(n);
              const lit = issues(n).length > 0;
              return (
                <li key={n.id} className="flex items-start gap-3 px-4 py-3">
                  <StatusDot state={pending ? "pending" : lit ? "lit" : "ok"} className="mt-1.5" />
                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2 font-medium">
                      <span className="truncate">{n.name}</span>
                      {n.hidden && (
                        <EyeOff className="size-3.5 shrink-0 text-muted-foreground" aria-label={t("servers.hidden")} />
                      )}
                      {n.group && (
                        <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-xs font-normal text-muted-foreground">
                          {n.group}
                        </span>
                      )}
                    </div>
                    <div className="mt-1 flex flex-wrap gap-x-3 gap-y-0.5 text-xs text-muted-foreground tabular-nums">
                      <span>{pending ? t("node.pending") : n.online ? t("node.online") : t("node.offline")}</span>
                      {n.last_ip && <span className="font-mono">{n.last_ip}</span>}
                      {n.info.agent_version && <span>{n.info.agent_version}</span>}
                      <TrafficFigures traffic={n.traffic} />
                      <QuotaText traffic={n.traffic} />
                      {n.expire_at > 0 && <span>{date(n.expire_at, lang)}</span>}
                      {(n.provider || n.region || n.tags) && (
                        <span className="basis-full break-words">
                          {[n.provider, n.region, n.tags].filter(Boolean).join(" / ")}
                        </span>
                      )}
                      {n.price > 0 && (
                        <span>
                          {money(n.price, n.currency)} / {t("assets.months", { n: n.billing_months })}
                        </span>
                      )}
                    </div>
                  </div>
                  <RowActions
                    onEdit={() => setEditing(n)}
                    onRotate={() => setConfirm({ node: n, action: "rotate" })}
                    onDelete={() => setConfirm({ node: n, action: "delete" })}
                  />
                </li>
              );
            })}
          </ul>
          <div className="hidden overflow-x-auto rounded-xl border bg-card md:block">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("servers.name")}</TableHead>
                  <TableHead>{t("servers.status")}</TableHead>
                  <TableHead>{t("servers.ip")}</TableHead>
                  <TableHead>{t("servers.version")}</TableHead>
                  <TableHead>{t("servers.traffic")}</TableHead>
                  <TableHead>{t("servers.expires")}</TableHead>
                  <TableHead>{t("assets.title")}</TableHead>
                  <TableHead className="w-10">
                    <span className="sr-only">{t("common.actions")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {list.map((n) => {
                  const pending = isPending(n);
                  const lit = issues(n).length > 0;
                  return (
                    <TableRow key={n.id}>
                      <TableCell>
                        <div className="flex items-center gap-2 font-medium">
                          {n.name}
                          {n.hidden && (
                            <EyeOff className="size-3.5 text-muted-foreground" aria-label={t("servers.hidden")} />
                          )}
                        </div>
                        {n.group && <div className="text-xs text-muted-foreground">{n.group}</div>}
                      </TableCell>
                      <TableCell>
                        <span className="flex items-center gap-2 text-sm">
                          <StatusDot state={pending ? "pending" : lit ? "lit" : "ok"} />
                          {pending ? t("node.pending") : n.online ? t("node.online") : t("node.offline")}
                        </span>
                      </TableCell>
                      <TableCell className="font-mono text-xs">{n.last_ip || "-"}</TableCell>
                      <TableCell className="text-xs">{n.info.agent_version || "-"}</TableCell>
                      <TableCell className="text-xs text-muted-foreground">
                        <div className="grid gap-0.5">
                          <TrafficFigures traffic={n.traffic} />
                          <span>
                            <QuotaText traffic={n.traffic} className="me-2" />
                            {t("traffic.counting")} {t(`modeShort.${n.traffic.mode}`)}
                          </span>
                        </div>
                      </TableCell>
                      <TableCell className="text-xs">
                        {n.expire_at ? (
                          <div className="grid justify-items-start gap-0.5">
                            <span>{date(n.expire_at, lang)}</span>
                            <Expiry node={n} now={Date.now() / 1000} className="text-muted-foreground" />
                          </div>
                        ) : (
                          "-"
                        )}
                      </TableCell>
                      <TableCell className="max-w-56 text-xs">
                        <div className="break-words">{[n.provider, n.region].filter(Boolean).join(" / ") || "—"}</div>
                        {n.tags && <div className="mt-1 break-words text-muted-foreground">{n.tags}</div>}
                        {n.price > 0 && (
                          <div className="mt-1 whitespace-nowrap tabular-nums">
                            {money(n.price, n.currency)} / {t("assets.months", { n: n.billing_months })}
                          </div>
                        )}
                        {n.auto_renew && <div className="mt-1 text-muted-foreground">{t("assets.autoRenew")}</div>}
                      </TableCell>
                      <TableCell>
                        <RowActions
                          onEdit={() => setEditing(n)}
                          onRotate={() => setConfirm({ node: n, action: "rotate" })}
                          onDelete={() => setConfirm({ node: n, action: "delete" })}
                        />
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
        </>
      )}

      {editing && (
        <NodeDialog
          node={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
          onCreated={(tok) => {
            setToken(tok);
            refresh();
          }}
          onSaved={refresh}
        />
      )}
      {token && <TokenDialog token={token} onClose={() => setToken(null)} />}
      {confirm && (
        <ConfirmDialog
          open
          onOpenChange={(o) => !o && setConfirm(null)}
          title={t(confirm.action === "delete" ? "servers.deleteTitle" : "servers.rotateTitle", {
            name: confirm.node.name,
          })}
          body={t(confirm.action === "delete" ? "servers.deleteBody" : "servers.rotateBody")}
          confirm={confirm.action === "delete" ? t("common.delete") : t("servers.rotate")}
          destructive
          onConfirm={() => {
            if (confirm.action === "delete") remove.mutate(confirm.node.id);
            else rotate.mutate(confirm.node.id);
            setConfirm(null);
          }}
        />
      )}
    </div>
  );
}

function RowActions({
  onEdit,
  onRotate,
  onDelete,
}: {
  onEdit: () => void;
  onRotate: () => void;
  onDelete: () => void;
}) {
  const { t } = useI18n();
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button variant="ghost" size="icon" className="size-8 shrink-0" aria-label={t("common.actions")}>
          <MoreHorizontal className="size-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuItem onSelect={onEdit}>{t("common.edit")}</DropdownMenuItem>
        <DropdownMenuItem onSelect={onRotate}>{t("servers.rotate")}</DropdownMenuItem>
        <DropdownMenuSeparator />
        <DropdownMenuItem className="text-destructive focus:text-destructive" onSelect={onDelete}>
          {t("common.delete")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function NodeDialog({
  node,
  onClose,
  onCreated,
  onSaved,
}: {
  node: AdminNode | null;
  onClose: () => void;
  onCreated: (token: string) => void;
  onSaved: () => void;
}) {
  const { t } = useI18n();
  const [f, setF] = useState({
    provider: node?.provider ?? "",
    region: node?.region ?? "",
    tags: node?.tags ?? "",
    price: String(node?.price ?? 0),
    currency: node?.currency || "CNY",
    billing_months: String(node?.billing_months ?? 1),
    auto_renew: node?.auto_renew ?? false,
    name: node?.name ?? "",
    group: node?.group ?? "",
    sort: String(node?.sort ?? 0),
    hidden: node?.hidden ?? false,
    public_note: node?.public_note ?? "",
    public_tags: node?.public_tags ?? "",
    note: node?.note ?? "",
    expire: toDateInput(node?.expire_at ?? 0),
    limit: node?.traffic_limit ? String(+(node.traffic_limit / GB).toFixed(2)) : "0",
    mode: (node?.traffic_mode ?? "sum") as TrafficMode,
    reset: String(node?.traffic_reset_day ?? 1),
  });
  const [error, setError] = useState("");
  const set = <K extends keyof typeof f>(k: K, v: (typeof f)[K]) => setF((p) => ({ ...p, [k]: v }));

  const save = useMutation({
    mutationFn: (body: NodeInput) =>
      node
        ? api(`/admin/api/nodes/${node.id}`, { method: "PUT", body })
        : api<{ token: string }>("/admin/api/nodes", { method: "POST", body }),
    onSuccess: (r) => {
      if (!node && r && typeof r === "object" && "token" in r) {
        toast.success(t("servers.created"));
        onCreated((r as { token: string }).token);
      } else {
        toast.success(t("common.saved"));
        onSaved();
      }
      onClose();
    },
    onError: (e) => setError(e instanceof ApiError && e.message ? e.message : String(e)),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    save.mutate({
      provider: f.provider,
      region: f.region,
      tags: f.tags,
      price: Number(f.price),
      currency: f.currency,
      billing_months: Number(f.billing_months),
      auto_renew: f.auto_renew,
      name: f.name,
      group: f.group,
      sort: Number(f.sort) || 0,
      hidden: f.hidden,
      note: f.note,
      public_note: f.public_note,
      public_tags: f.public_tags,
      expire_at: fromDateInput(f.expire),
      traffic_limit: Math.round((Number(f.limit) || 0) * GB),
      traffic_mode: f.mode,
      traffic_reset_day: Number(f.reset) || 1,
    });
  }

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="max-h-[90dvh] overflow-y-auto sm:max-w-2xl">
        <form onSubmit={submit} className="grid gap-4">
          <DialogHeader>
            <DialogTitle>{node ? t("form.editTitle") : t("form.addTitle")}</DialogTitle>
          </DialogHeader>
          <div className="grid gap-4 sm:grid-cols-2">
            <Field id="name" label={t("form.name")}>
              <Input id="name" value={f.name} onChange={(e) => set("name", e.target.value)} required maxLength={64} />
            </Field>
            <Field id="group" label={t("form.group")}>
              <Input id="group" value={f.group} onChange={(e) => set("group", e.target.value)} maxLength={32} />
            </Field>
            <Field id="expire" label={t("form.expire")}>
              <Input id="expire" type="date" value={f.expire} onChange={(e) => set("expire", e.target.value)} />
            </Field>
            <Field id="sort" label={t("form.sort")} hint={t("form.sortHint")}>
              <Input id="sort" type="number" value={f.sort} onChange={(e) => set("sort", e.target.value)} />
            </Field>
            <Field id="limit" label={t("form.trafficLimit")} hint={t("form.trafficLimitHint")}>
              <Input
                id="limit"
                type="number"
                min={0}
                step="any"
                value={f.limit}
                onChange={(e) => set("limit", e.target.value)}
              />
            </Field>
            <Field id="reset" label={t("form.resetDay")} hint={t("form.resetDayHint")}>
              <Input
                id="reset"
                type="number"
                min={1}
                max={31}
                value={f.reset}
                onChange={(e) => set("reset", e.target.value)}
              />
            </Field>
            <Field id="mode" label={t("form.trafficMode")} hint={t("form.trafficModeHint")}>
              <Select value={f.mode} onValueChange={(v) => set("mode", v as TrafficMode)}>
                <SelectTrigger id="mode" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {(["sum", "up", "down", "max"] as const).map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(`mode.${m}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-center gap-2 self-end pb-2">
              <Switch id="hidden" checked={f.hidden} onCheckedChange={(v) => set("hidden", v)} />
              <label htmlFor="hidden" className="text-sm">
                {t("form.hidden")}
              </label>
            </div>
          </div>
          <fieldset className="grid gap-4 border-t pt-4">
            <legend className="px-1 text-sm font-medium">{t("form.publicDisplay")}</legend>
            <Field id="public_tags" label={t("form.publicTags")} hint={t("form.publicTagsHint")}>
              <Input
                id="public_tags"
                value={f.public_tags}
                onChange={(e) => set("public_tags", e.target.value)}
                placeholder={t("form.publicTagsPlaceholder")}
                maxLength={200}
              />
              <NodeTags value={f.public_tags} />
            </Field>
            <Field id="public_note" label={t("form.publicNote")} hint={t("form.publicNoteHint")}>
              <Input
                id="public_note"
                value={f.public_note}
                onChange={(e) => set("public_note", e.target.value)}
                maxLength={200}
              />
            </Field>
          </fieldset>
          <fieldset className="grid gap-4 border-t pt-4 sm:grid-cols-2">
            <legend className="px-1 text-sm font-medium">{t("assets.privateTitle")}</legend>
            {(["provider", "region", "tags"] as const).map((key) => (
              <Field key={key} id={key} label={t(`assets.${key}`)}>
                <Input
                  id={key}
                  value={f[key]}
                  maxLength={key === "tags" ? 200 : 64}
                  onChange={(e) => set(key, e.target.value)}
                />
              </Field>
            ))}
            <Field id="price" label={t("assets.price")}>
              <Input
                id="price"
                type="number"
                min={0}
                max={10000000}
                step="0.01"
                required
                value={f.price}
                onChange={(e) => set("price", e.target.value)}
              />
            </Field>
            <Field id="currency" label={t("assets.currency")}>
              <Input
                id="currency"
                required
                minLength={3}
                maxLength={3}
                pattern="[A-Za-z]{3}"
                value={f.currency}
                onChange={(e) => set("currency", e.target.value.toUpperCase())}
              />
            </Field>
            <Field id="billing_months" label={t("assets.billing")}>
              <Select value={f.billing_months} onValueChange={(v) => set("billing_months", v)}>
                <SelectTrigger id="billing_months" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {[1, 3, 6, 12, 24, 36].map((v) => (
                    <SelectItem key={v} value={String(v)}>
                      {t("assets.months", { n: v })}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
            <div className="flex items-center gap-2">
              <Switch id="auto_renew" checked={f.auto_renew} onCheckedChange={(v) => set("auto_renew", v)} />
              <label htmlFor="auto_renew" className="text-sm">
                {t("assets.autoRenew")}
              </label>
            </div>
          </fieldset>
          <Field id="note" label={t("form.note")}>
            <Textarea
              id="note"
              value={f.note}
              onChange={(e) => set("note", e.target.value)}
              maxLength={1000}
              rows={2}
            />
          </Field>
          {error && (
            <p role="alert" className="text-sm text-destructive">
              {error}
            </p>
          )}
          <DialogFooter className="sticky -bottom-6 -mx-6 -mb-6 border-t bg-background px-6 py-4">
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {node ? t("common.save") : t("servers.add")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function TokenDialog({ token, onClose }: { token: string; onClose: () => void }) {
  const { t } = useI18n();
  const settings = useQuery({ queryKey: ["admin", "settings"], queryFn: () => api<Settings>("/admin/api/settings") });
  const server = settings.data?.public_url || "https://probe.example.com";
  const shellQuote = (value: string) => `'${value.replaceAll("'", "'\\''")}'`;
  const command = `curl -fsSL ${INSTALL_SCRIPT} | sudo bash -s -- --server ${shellQuote(server)} --token ${shellQuote(token)} --idle-interval ${settings.data?.agent_idle_interval ?? 15}s${settings.data ? probeInstallFlags(settings.data) : ""}`;

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("token.title")}</DialogTitle>
          <DialogDescription>{t("token.body")}</DialogDescription>
        </DialogHeader>
        {settings.data && !settings.data.public_url && <p className="text-sm text-amber">{t("token.noUrl")}</p>}
        <CopyField value={command} />
        <CopyField label={t("token.label")} value={token} />
        <p className="text-xs text-muted-foreground">{t("token.manual")}</p>
        <DialogFooter>
          <Button onClick={onClose}>{t("common.close")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
