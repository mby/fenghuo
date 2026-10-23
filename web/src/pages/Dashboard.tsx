import { ArrowDown, ArrowDownUp, ArrowUp, Flame, LayoutGrid, List, Search } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { NodeCard } from "@/components/NodeCard";
import { NodeList } from "@/components/NodeList";
import { Shell } from "@/components/Shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { ApiError, useMeta, useNodes } from "@/lib/api";
import { bytes, rate } from "@/lib/format";
import { isPending, issues } from "@/lib/health";
import { type MessageKey, useI18n } from "@/lib/i18n";
import {
  compareHref,
  finiteValue,
  isVisible,
  MAX_COMPARE,
  matchesNode,
  NODE_SORTS,
  type NodeSort,
  sortNodes,
} from "@/lib/node-view";
import type { PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";

type View = "cards" | "list";
const VIEW_KEY = "fenghuo-view";
const SORT_LABELS: Record<NodeSort, MessageKey> = {
  default: "sort.default",
  name: "servers.name",
  cpu: "metric.cpu",
  memory: "metric.mem",
  network: "list.net",
  traffic: "node.traffic",
  expiry: "servers.expires",
  status: "servers.status",
};

function initialView(): View {
  try {
    const v = localStorage.getItem(VIEW_KEY);
    if (v === "cards" || v === "list") return v;
  } catch {
    /* Storage may be unavailable in private browsing. */
  }
  return window.matchMedia("(max-width: 639px)").matches ? "list" : "cards";
}

export function Dashboard() {
  const { t } = useI18n();
  const meta = useMeta();
  const nodes = useNodes();
  const [group, setGroup] = useState("");
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<NodeSort>("default");
  const [reverse, setReverse] = useState(false);
  const [onlyAttention, setOnlyAttention] = useState(false);
  const [view, setViewState] = useState<View>(initialView);
  const [comparing, setComparing] = useState(false);
  const [selection, setSelection] = useState<string[]>([]);
  const now = Math.floor((nodes.dataUpdatedAt || Date.now()) / 1000);
  const list = nodes.isError ? [] : (nodes.data ?? []);
  const groups = useMemo(() => [...new Set(list.map((n) => n.group).filter(Boolean))].sort(), [list]);
  const selected = selection.filter((id) => list.some((n) => n.uuid === id));
  const shown = group ? list.filter((n) => n.group === group) : list;
  const needsAttention = shown.filter((n) => !isPending(n) && issues(n).length > 0);
  const attention = needsAttention.length;
  const visible = sortNodes(
    (onlyAttention ? needsAttention : shown).filter((n) => matchesNode(n, search)),
    sort,
    reverse,
  );
  const online = shown.filter((n) => n.online).length;
  const up = shown.reduce((a, n) => a + (n.online ? (finiteValue(n.status?.net_out) ?? 0) : 0), 0);
  const down = shown.reduce((a, n) => a + (n.online ? (finiteValue(n.status?.net_in) ?? 0) : 0), 0);
  const trafficVisible = shown.length > 0 && shown.every((n) => isVisible(n, "traffic"));
  const traffic = shown.reduce((a, n) => a + (isVisible(n, "traffic") ? (finiteValue(n.traffic?.used) ?? 0) : 0), 0);
  const filtered = !!(group || search.trim() || onlyAttention);
  const clearFilters = () => {
    setGroup("");
    setSearch("");
    setOnlyAttention(false);
  };
  const selectionFor = (node: PublicNode) => ({
    selected: selected.includes(node.uuid),
    disabled: selected.length >= MAX_COMPARE && !selected.includes(node.uuid),
    onToggle: () =>
      setSelection(
        selected.includes(node.uuid)
          ? selected.filter((id) => id !== node.uuid)
          : [...selected, node.uuid].slice(0, MAX_COMPARE),
      ),
  });
  const setView = (v: View) => {
    try {
      localStorage.setItem(VIEW_KEY, v);
    } catch {
      /* View remains usable without storage. */
    }
    setViewState(v);
  };

  if (nodes.error instanceof ApiError && nodes.error.code === "public_disabled") {
    return (
      <Shell>
        <div className="py-24 text-center">
          <h1 className="text-2xl font-medium">{t("public.disabled")}</h1>
          {meta.data?.admin && (
            <Link to="/login" className="mt-3 inline-block text-sm text-muted-foreground underline underline-offset-4">
              {t("public.disabledHint")}
            </Link>
          )}
        </div>
      </Shell>
    );
  }

  const headline = nodes.isPending
    ? t("hero.loading")
    : nodes.isError
      ? t("dashboard.unavailable")
      : list.length === 0
        ? t("hero.empty")
        : shown.length === 0
          ? t("search.noResults")
          : attention === 0
            ? shown.length === 1
              ? t("hero.quietOne")
              : t("hero.quiet", { n: shown.length })
            : attention === 1
              ? t("hero.attention1", { n: shown.length })
              : t("hero.attention", { n: shown.length, k: attention });

  return (
    <Shell>
      <section className="pt-4 pb-6 sm:pt-10 sm:pb-8">
        <h1 className="flex items-start gap-3 text-xl font-medium tracking-tight text-balance sm:items-center sm:text-3xl">
          {attention > 0 && <Flame className="mt-0.5 size-6 shrink-0 text-ember sm:mt-0 sm:size-7" aria-hidden />}
          {headline}
        </h1>
        {list.length === 0 && !nodes.isPending && !nodes.isError && meta.data?.admin && (
          <Link to="/admin" className="mt-2 inline-block text-sm text-muted-foreground underline underline-offset-4">
            {t("hero.emptyAdmin")}
          </Link>
        )}
        {shown.length > 0 && (
          <dl className="mt-3 flex flex-wrap gap-x-5 gap-y-1.5 text-sm text-muted-foreground tabular-nums sm:mt-4 sm:gap-x-6">
            <div>
              <dt className="sr-only">{t("servers.status")}</dt>
              <dd>{t("summary.online", { o: online, n: shown.length })}</dd>
            </div>
            <div className="flex items-center gap-1">
              <dt className="sr-only">{t("summary.up")}</dt>
              <ArrowUp className="size-3.5" aria-hidden />
              <dd>{rate(up)}</dd>
            </div>
            <div className="flex items-center gap-1">
              <dt className="sr-only">{t("summary.down")}</dt>
              <ArrowDown className="size-3.5" aria-hidden />
              <dd>{rate(down)}</dd>
            </div>
            {trafficVisible && (
              <div className="flex gap-1">
                <dt>{t("summary.traffic")}</dt>
                <dd>{bytes(traffic)}</dd>
              </div>
            )}
          </dl>
        )}
        {nodes.isError && (
          <div role="alert" className="mt-3 flex flex-wrap items-center gap-3 text-sm">
            <p className="text-ember">{t("error.load")}</p>
            <Button variant="outline" size="sm" onClick={() => void nodes.refetch()}>
              {t("public.retry")}
            </Button>
          </div>
        )}
      </section>

      {list.length > 0 && (
        <section aria-label={t("dashboard.controls")} className="mb-5 space-y-3">
          <div className="grid gap-3 sm:grid-cols-[minmax(12rem,1fr)_auto]">
            <div className="relative">
              <label htmlFor="node-search" className="sr-only">
                {t("search.label")}
              </label>
              <Search
                className="pointer-events-none absolute top-2.5 left-3 size-4 text-muted-foreground"
                aria-hidden
              />
              <Input
                id="node-search"
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t("search.placeholder")}
                className="bg-card ps-9"
              />
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <label htmlFor="node-sort" className="text-sm text-muted-foreground">
                {t("sort.label")}
              </label>
              <select
                id="node-sort"
                className="public-select min-w-0 flex-1 sm:flex-none"
                value={sort}
                onChange={(e) => {
                  setSort(e.target.value as NodeSort);
                  setReverse(false);
                }}
              >
                {NODE_SORTS.map((key) => (
                  <option key={key} value={key}>
                    {t(SORT_LABELS[key])}
                  </option>
                ))}
              </select>
              <Button
                variant="outline"
                size="icon"
                disabled={sort === "default"}
                aria-label={t("sort.reverse")}
                aria-pressed={reverse}
                title={t("sort.reverse")}
                onClick={() => setReverse(!reverse)}
              >
                {sort === "default" ? (
                  <ArrowDownUp className="size-4" />
                ) : ["name", "expiry", "status"].includes(sort) !== reverse ? (
                  <ArrowUp className="size-4" />
                ) : (
                  <ArrowDown className="size-4" />
                )}
              </Button>
              <ViewToggle view={view} onChange={setView} />
            </div>
          </div>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex min-w-0 flex-wrap items-center gap-2">
              {groups.length > 0 && (
                <>
                  <label htmlFor="node-group" className="text-sm text-muted-foreground">
                    {t("form.group")}
                  </label>
                  <select
                    id="node-group"
                    className="public-select max-w-48"
                    value={group}
                    onChange={(e) => setGroup(e.target.value)}
                  >
                    <option value="">{t("group.all")}</option>
                    {groups.map((g) => (
                      <option key={g} value={g}>
                        {g}
                      </option>
                    ))}
                  </select>
                </>
              )}
              {(attention > 0 || onlyAttention) && (
                <Button
                  variant={onlyAttention ? "secondary" : "ghost"}
                  size="sm"
                  aria-pressed={onlyAttention}
                  onClick={() => setOnlyAttention(!onlyAttention)}
                >
                  <span className="size-1.5 rounded-full bg-ember" aria-hidden />
                  {t("filter.attention")}
                  <span className="tabular-nums">{attention}</span>
                </Button>
              )}
              {filtered && (
                <Button variant="ghost" size="sm" onClick={clearFilters}>
                  {t("search.clear")}
                </Button>
              )}
            </div>
            <div className="flex items-center gap-3">
              <p className="text-xs text-muted-foreground tabular-nums" role="status">
                {t("search.count", { n: visible.length, total: list.length })}
              </p>
              <Button variant="outline" size="sm" aria-pressed={comparing} onClick={() => setComparing(!comparing)}>
                {t(comparing ? "compare.done" : "compare.title")}
              </Button>
            </div>
          </div>
          {comparing && (
            <div className="flex flex-wrap items-center gap-3 rounded-lg border bg-card p-3">
              <p className="mr-auto text-sm" role="status">
                {t(selected.length === MAX_COMPARE ? "compare.limit" : "compare.count", { n: selected.length })}
              </p>
              {selected.length > 0 && (
                <Button variant="ghost" size="sm" onClick={() => setSelection([])}>
                  {t("compare.clear")}
                </Button>
              )}
              {selected.length >= 2 ? (
                <Button asChild size="sm">
                  <Link to={compareHref(selected)}>{t("compare.open", { n: selected.length })}</Link>
                </Button>
              ) : (
                <Button size="sm" disabled>
                  {t("compare.chooseTwo")}
                </Button>
              )}
            </div>
          )}
        </section>
      )}

      {nodes.isPending ? (
        <div
          role="status"
          className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4"
          aria-label={t("hero.loading")}
          aria-busy="true"
        >
          {Array.from({ length: 4 }, (_, i) => (
            <Skeleton key={i} className="h-56 rounded-xl" />
          ))}
        </div>
      ) : list.length > 0 && visible.length === 0 ? (
        <div className="rounded-xl border border-dashed px-5 py-12 text-center">
          <h2 className="font-medium">
            {t(onlyAttention && !search.trim() && shown.length > 0 ? "filter.allClear" : "search.noResults")}
          </h2>
          <p className="mt-2 text-sm text-muted-foreground">{t("search.hint")}</p>
          <Button variant="outline" className="mt-4" onClick={clearFilters}>
            {t("search.clear")}
          </Button>
        </div>
      ) : view === "list" ? (
        visible.length > 0 && <NodeList nodes={visible} now={now} selectionFor={comparing ? selectionFor : undefined} />
      ) : (
        <div className="columns-1 gap-3 sm:columns-2 lg:columns-3 xl:columns-4 [&>article]:mb-3 [&>article]:break-inside-avoid">
          {visible.map((n) => (
            <NodeCard key={n.uuid} node={n} now={now} selection={comparing ? selectionFor(n) : undefined} />
          ))}
        </div>
      )}
    </Shell>
  );
}

function ViewToggle({ view, onChange }: { view: View; onChange: (v: View) => void }) {
  const { t } = useI18n();
  return (
    <fieldset className="flex shrink-0 rounded-lg border bg-card p-0.5">
      <legend className="sr-only">{t("view.label")}</legend>
      {[
        { value: "cards" as const, label: t("view.cards"), Icon: LayoutGrid },
        { value: "list" as const, label: t("view.list"), Icon: List },
      ].map(({ value, label, Icon }) => (
        <button
          key={value}
          type="button"
          aria-pressed={view === value}
          title={label}
          onClick={() => onChange(value)}
          className={cn(
            "flex size-8 items-center justify-center rounded-md transition-colors",
            view === value ? "bg-secondary text-secondary-foreground" : "text-muted-foreground hover:text-foreground",
          )}
        >
          <Icon className="size-4" aria-hidden />
          <span className="sr-only">{label}</span>
        </button>
      ))}
    </fieldset>
  );
}
