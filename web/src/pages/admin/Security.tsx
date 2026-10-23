import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { ConfirmDialog } from "@/components/ConfirmDialog";
import { Field } from "@/components/Field";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { ApiError, api } from "@/lib/api";
import { dateTime } from "@/lib/format";
import { useI18n } from "@/lib/i18n";

import type { AdminSession, SecurityAuditPage } from "@/lib/types";

const ACTIONS = [
  ["login", "security.action.login"],
  ["logout", "security.action.logout"],
  ["password.change", "security.action.password"],
  ["totp.enable", "security.action.totpEnable"],
  ["totp.disable", "security.action.totpDisable"],
  ["settings.update", "security.action.settings"],
  ["node.create", "security.action.nodeCreate"],
  ["node.update", "security.action.nodeUpdate"],
  ["node.delete", "security.action.nodeDelete"],
  ["node.token.rotate", "security.action.nodeToken"],
  ["rule.create", "security.action.ruleCreate"],
  ["rule.update", "security.action.ruleUpdate"],
  ["rule.delete", "security.action.ruleDelete"],
  ["channel.create", "security.action.channelCreate"],
  ["channel.update", "security.action.channelUpdate"],
  ["channel.delete", "security.action.channelDelete"],
  ["channel.test", "security.action.channelTest"],
  ["session.revoke", "security.action.sessionRevoke"],
  ["session.revoke_others", "security.action.sessionOthers"],
] as const;

export function Security() {
  const { t, lang } = useI18n();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [revoking, setRevoking] = useState<AdminSession | "others" | null>(null);
  const [action, setAction] = useState("all");
  const [outcome, setOutcome] = useState("all");
  const [cursors, setCursors] = useState<number[]>([0]);
  const before = cursors[cursors.length - 1];

  const sessions = useQuery({
    queryKey: ["admin", "sessions"],
    queryFn: () => api<AdminSession[]>("/admin/api/sessions"),
    refetchInterval: 30_000,
    retry: false,
  });
  const params = new URLSearchParams({ limit: "25", before: String(before) });
  if (action !== "all") params.set("action", action);
  if (outcome !== "all") params.set("outcome", outcome);
  const audit = useQuery({
    queryKey: ["admin", "audit", action, outcome, before],
    queryFn: () => api<SecurityAuditPage>(`/admin/api/audit?${params}`),
    refetchInterval: before === 0 ? 10_000 : false,
    retry: false,
  });

  useEffect(() => {
    if ([sessions.error, audit.error].some((error) => error instanceof ApiError && error.status === 401)) {
      qc.clear();
      navigate("/login", { replace: true });
    }
  }, [sessions.error, audit.error, navigate, qc]);

  const revoke = useMutation({
    mutationFn: (target: AdminSession | "others") =>
      api<void>(target === "others" ? "/admin/api/sessions" : `/admin/api/sessions/${encodeURIComponent(target.id)}`, {
        method: "DELETE",
      }),
    onSuccess: (_, target) => {
      toast.success(t("security.revoked"));
      if (target !== "others" && target.current) {
        qc.clear();
        navigate("/login", { replace: true });
        return;
      }
      void qc.invalidateQueries({ queryKey: ["admin", "sessions"] });
      void qc.invalidateQueries({ queryKey: ["admin", "audit"] });
    },
    onError: (error) => {
      if (error instanceof ApiError && error.status === 401) {
        qc.clear();
        navigate("/login", { replace: true });
      } else {
        toast.error(t("security.revokeError"));
      }
    },
  });

  const refresh = () => {
    setCursors([0]);
    void qc.invalidateQueries({ queryKey: ["admin", "audit"] });
  };
  const actionLabel = (value: string) => {
    const entry = ACTIONS.find(([name]) => name === value);
    return entry ? t(entry[1]) : t("security.unknownAction");
  };

  return (
    <div className="grid gap-10">
      <section aria-labelledby="sessions-title" className="grid gap-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="sessions-title" className="text-lg font-medium">
              {t("security.sessions")}
            </h2>
            <p className="text-sm text-muted-foreground">{t("security.sessionsHint")}</p>
          </div>
          <Button
            variant="outline"
            disabled={revoke.isPending || !sessions.data?.some((session) => !session.current)}
            onClick={() => setRevoking("others")}
          >
            {t("security.revokeOthers")}
          </Button>
        </div>
        {sessions.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            {t("security.loading")}
          </p>
        ) : sessions.isError ? (
          <div role="alert" className="flex items-center gap-3 text-sm">
            <p>{t("security.loadError")}</p>
            <Button variant="outline" disabled={sessions.isFetching} onClick={() => void sessions.refetch()}>
              {t("security.retry")}
            </Button>
          </div>
        ) : !sessions.data?.length ? (
          <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
            {t("security.noSessions")}
          </p>
        ) : (
          <ul className="divide-y rounded-xl border bg-card">
            {sessions.data.map((session) => (
              <li key={session.id} className="flex flex-wrap items-center justify-between gap-4 p-4">
                <div className="min-w-0 flex-1 space-y-1">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className="font-mono text-sm">{session.ip || t("security.unknown")}</span>
                    {session.current && <Badge variant="secondary">{t("security.current")}</Badge>}
                  </div>
                  <p className="break-all text-sm text-muted-foreground">
                    {session.user_agent || t("security.unknownBrowser")}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t("security.created", { time: dateTime(session.created_at, lang) })}
                  </p>
                  <p className="text-xs text-muted-foreground">
                    {t("security.expires", { time: dateTime(session.expires_at, lang) })}
                  </p>
                </div>
                <Button variant="outline" disabled={revoke.isPending} onClick={() => setRevoking(session)}>
                  {t("security.revoke")}
                </Button>
              </li>
            ))}
          </ul>
        )}
        {revoke.isPending && (
          <p role="status" className="text-sm text-muted-foreground">
            {t("security.revoking")}
          </p>
        )}
      </section>

      <section aria-labelledby="audit-title" className="grid gap-4">
        <div className="flex flex-wrap items-end justify-between gap-3">
          <div>
            <h2 id="audit-title" className="text-lg font-medium">
              {t("security.audit")}
            </h2>
            <p className="text-sm text-muted-foreground">{t("security.auditHint")}</p>
          </div>
          <Button variant="outline" disabled={audit.isFetching} onClick={refresh}>
            {t("security.refresh")}
          </Button>
        </div>
        <div className="flex flex-wrap gap-4">
          <Field id="audit-action" label={t("security.action")}>
            <Select
              value={action}
              onValueChange={(value) => {
                setAction(value);
                setCursors([0]);
              }}
            >
              <SelectTrigger id="audit-action" className="w-56">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("security.allActions")}</SelectItem>
                {ACTIONS.map(([name, key]) => (
                  <SelectItem key={name} value={name}>
                    {t(key)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field id="audit-outcome" label={t("security.outcome")}>
            <Select
              value={outcome}
              onValueChange={(value) => {
                setOutcome(value);
                setCursors([0]);
              }}
            >
              <SelectTrigger id="audit-outcome" className="w-44">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">{t("security.allOutcomes")}</SelectItem>
                <SelectItem value="success">{t("security.success")}</SelectItem>
                <SelectItem value="failure">{t("security.failure")}</SelectItem>
              </SelectContent>
            </Select>
          </Field>
        </div>
        {audit.isPending ? (
          <p role="status" className="text-sm text-muted-foreground">
            {t("security.loading")}
          </p>
        ) : audit.isError ? (
          <div role="alert" className="flex items-center gap-3 text-sm">
            <p>{t("security.loadError")}</p>
            <Button variant="outline" disabled={audit.isFetching} onClick={() => void audit.refetch()}>
              {t("security.retry")}
            </Button>
          </div>
        ) : !audit.data?.items.length ? (
          <p className="rounded-xl border border-dashed p-8 text-center text-sm text-muted-foreground">
            {t("security.noAudit")}
          </p>
        ) : (
          <div className="rounded-xl border bg-card">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("security.time")}</TableHead>
                  <TableHead>{t("security.action")}</TableHead>
                  <TableHead>{t("security.target")}</TableHead>
                  <TableHead>{t("security.outcome")}</TableHead>
                  <TableHead>{t("security.ip")}</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {audit.data.items.map((event) => (
                  <TableRow key={event.id}>
                    <TableCell className="tabular-nums">{dateTime(event.created_at, lang)}</TableCell>
                    <TableCell>{actionLabel(event.action)}</TableCell>
                    <TableCell className="font-mono text-xs">{event.target || t("security.noTarget")}</TableCell>
                    <TableCell>
                      <Badge variant="outline" className={event.status >= 400 ? "text-ember" : undefined}>
                        {t(event.status >= 400 ? "security.failureCode" : "security.successCode", {
                          code: event.status,
                        })}
                      </Badge>
                    </TableCell>
                    <TableCell className="font-mono text-xs">{event.ip || t("security.unknown")}</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        )}
        <nav aria-label={t("security.pagination")} className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            disabled={cursors.length <= 1 || audit.isFetching}
            onClick={() => setCursors((values) => values.slice(0, -1))}
          >
            {t("security.previous")}
          </Button>
          <span className="text-sm text-muted-foreground" aria-live="polite">
            {t("security.page", { n: cursors.length })}
          </span>
          <Button
            variant="outline"
            disabled={!audit.data?.next_before || audit.isFetching || audit.isError || cursors.length >= 400}
            onClick={() => {
              if (audit.data?.next_before) setCursors((values) => [...values, audit.data.next_before]);
            }}
          >
            {t("security.next")}
          </Button>
        </nav>
      </section>

      {revoking && (
        <ConfirmDialog
          open
          onOpenChange={(open) => !open && setRevoking(null)}
          title={t(revoking === "others" ? "security.confirmOthers" : "security.confirmRevoke")}
          body={t(
            revoking === "others"
              ? "security.confirmOthersBody"
              : revoking.current
                ? "security.confirmCurrentBody"
                : "security.confirmRevokeBody",
            {
              ip: revoking === "others" ? "" : revoking.ip || t("security.unknown"),
            },
          )}
          confirm={t("security.revoke")}
          destructive
          onConfirm={() => {
            if (!revoke.isPending) revoke.mutate(revoking);
            setRevoking(null);
          }}
        />
      )}
    </div>
  );
}
