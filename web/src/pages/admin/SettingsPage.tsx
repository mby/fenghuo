import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import QRCode from "qrcode";
import { type FormEvent, useEffect, useState } from "react";
import { toast } from "sonner";
import { CopyField } from "@/components/CopyField";
import { Field } from "@/components/Field";
import { ProbePresetFields } from "@/components/ProbePresetFields";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { ApiError, api, useMe } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { probeConfigureCommand } from "@/lib/probes";
import type { Settings } from "@/lib/types";

export function SettingsPage() {
  return (
    <div className="grid max-w-2xl gap-10">
      <SiteSettings />
      <PasswordForm />
      <TwoFactor />
    </div>
  );
}

function Section({ title, hint, children }: { title: string; hint?: string; children: React.ReactNode }) {
  return (
    <section className="grid gap-4">
      <div>
        <h2 className="text-lg font-medium">{title}</h2>
        {hint && <p className="text-sm text-muted-foreground">{hint}</p>}
      </div>
      {children}
    </section>
  );
}

function errorText(e: unknown) {
  return e instanceof ApiError && e.message ? e.message : String(e);
}

function SiteSettings() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const settings = useQuery({ queryKey: ["admin", "settings"], queryFn: () => api<Settings>("/admin/api/settings") });
  const [f, setF] = useState<Settings | null>(null);
  useEffect(() => {
    if (settings.data && !f) setF(settings.data);
  }, [settings.data, f]);

  const save = useMutation({
    mutationFn: (body: Settings) => api<Settings>("/admin/api/settings", { method: "PUT", body }),
    onSuccess: (s) => {
      setF(s);
      qc.setQueryData(["admin", "settings"], s);
      qc.invalidateQueries({ queryKey: ["meta"] });
      qc.invalidateQueries({ queryKey: ["nodes"] });
      qc.removeQueries({ queryKey: ["ping"] });
      toast.success(t("common.saved"));
    },
    onError: (e) => toast.error(t("common.error", { m: errorText(e) })),
  });

  if (settings.isError)
    return (
      <p role="alert" className="text-sm text-destructive">
        {t("error.load")}
      </p>
    );
  if (!f) return <p className="text-sm text-muted-foreground">{t("hero.loading")}</p>;
  const submit = (e: FormEvent) => {
    e.preventDefault();
    save.mutate(f);
  };

  return (
    <Section title={t("settings.site")}>
      <form onSubmit={submit} className="grid gap-4 rounded-xl border bg-card p-4">
        <Field id="site_name" label={t("settings.siteName")}>
          <Input
            id="site_name"
            value={f.site_name}
            onChange={(e) => setF({ ...f, site_name: e.target.value })}
            required
            maxLength={64}
          />
        </Field>
        <Field id="public_url" label={t("settings.publicUrl")} hint={t("settings.publicUrlHint")}>
          <Input
            id="public_url"
            value={f.public_url}
            onChange={(e) => setF({ ...f, public_url: e.target.value })}
            placeholder="https://probe.example.com"
          />
        </Field>
        <Field id="lang" label={t("settings.lang")}>
          <Select value={f.lang} onValueChange={(v) => setF({ ...f, lang: v as Settings["lang"] })}>
            <SelectTrigger id="lang" className="w-48">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="zh">中文</SelectItem>
              <SelectItem value="en">English</SelectItem>
            </SelectContent>
          </Select>
        </Field>
        <div className="flex items-start gap-3">
          <Switch
            id="public_enabled"
            checked={f.public_enabled}
            onCheckedChange={(v) => setF({ ...f, public_enabled: v })}
            className="mt-0.5"
          />
          <div>
            <label htmlFor="public_enabled" className="text-sm font-medium">
              {t("settings.public")}
            </label>
            <p className="text-xs text-muted-foreground">{t("settings.publicHint")}</p>
          </div>
        </div>
        <fieldset className="grid gap-3 border-t pt-4">
          <legend className="float-left mb-2 w-full text-sm font-medium">{t("privacy.title")}</legend>
          <p className="text-xs text-muted-foreground">{t("privacy.hint")}</p>
          {(["public_system", "public_expiry", "public_traffic", "public_ping"] as const).map((key) => (
            <div key={key} className="flex items-center justify-between gap-4">
              <label htmlFor={key} className="text-sm">
                {t(`privacy.${key}`)}
              </label>
              <Switch id={key} checked={f[key]} onCheckedChange={(v) => setF({ ...f, [key]: v })} />
            </div>
          ))}
        </fieldset>
        <div className="grid gap-3 border-t pt-4">
          <Field id="agent_idle_interval" label={t("settings.idleInterval")} hint={t("settings.idleHint")}>
            <Select
              value={String(f.agent_idle_interval)}
              onValueChange={(v) => setF({ ...f, agent_idle_interval: Number(v) })}
            >
              <SelectTrigger id="agent_idle_interval" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[3, 15, 30, 60].map((v) => (
                  <SelectItem key={v} value={String(v)}>
                    {t("settings.seconds", { n: v })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <CopyField label={t("settings.idleLocal")} value={`FENGHUO_IDLE_INTERVAL=${f.agent_idle_interval}s`} />
          <p className="text-xs text-muted-foreground">{t("settings.idleApply")}</p>
        </div>
        <fieldset className="grid gap-3 border-t pt-4">
          <legend className="float-left mb-1 w-full text-sm font-medium">{t("settings.probeTitle")}</legend>
          <p className="text-xs text-muted-foreground">{t("settings.probeHint")}</p>
          <ProbePresetFields
            targets={f.probe_targets}
            cards={f.probe_cards}
            onChange={(targets, cards) => setF({ ...f, probe_targets: targets, probe_cards: cards })}
          />
          <Field id="probe_interval" label={t("settings.probeInterval")}>
            <Select
              value={String(f.probe_interval ?? 30)}
              onValueChange={(v) => setF({ ...f, probe_interval: Number(v) })}
            >
              <SelectTrigger id="probe_interval" className="w-48">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {[15, 30, 60].map((seconds) => (
                  <SelectItem key={seconds} value={String(seconds)}>
                    {t("settings.seconds", { n: seconds })}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
        </fieldset>
        <div>
          <Button type="submit" disabled={save.isPending}>
            {t("common.save")}
          </Button>
        </div>
      </form>
      {settings.data && (
        <div className="grid gap-2">
          <CopyField
            label={t(settings.data.probe_targets ? "settings.probeApply" : "settings.probeDisable")}
            value={probeConfigureCommand(settings.data)}
          />
          <p className="text-xs text-muted-foreground">{t("settings.probeApplyHint")}</p>
        </div>
      )}
    </Section>
  );
}

function PasswordForm() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");

  const save = useMutation({
    mutationFn: () => api("/admin/api/password", { method: "POST", body: { current, new: next } }),
    onSuccess: () => {
      setCurrent("");
      setNext("");
      setConfirm("");
      qc.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("settings.passwordChanged"));
    },
    onError: (e) =>
      setError(e instanceof ApiError && e.code === "invalid_credentials" ? t("settings.wrongPassword") : errorText(e)),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (next !== confirm) {
      setError(t("settings.passwordMismatch"));
      return;
    }
    save.mutate();
  }

  return (
    <Section title={t("settings.account")}>
      <form onSubmit={submit} className="grid gap-4 rounded-xl border bg-card p-4">
        <input type="text" name="username" autoComplete="username" value="admin" readOnly hidden />
        <Field id="pw-current" label={t("settings.currentPassword")}>
          <Input
            id="pw-current"
            type="password"
            autoComplete="current-password"
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            required
          />
        </Field>
        <div className="grid gap-4 sm:grid-cols-2">
          <Field id="pw-new" label={t("settings.newPassword")} hint={t("settings.passwordHint")}>
            <Input
              id="pw-new"
              type="password"
              autoComplete="new-password"
              minLength={10}
              maxLength={128}
              value={next}
              onChange={(e) => setNext(e.target.value)}
              required
            />
          </Field>
          <Field id="pw-confirm" label={t("settings.confirmPassword")}>
            <Input
              id="pw-confirm"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              required
            />
          </Field>
        </div>
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <div>
          <Button type="submit" disabled={save.isPending}>
            {t("settings.changePassword")}
          </Button>
        </div>
      </form>
    </Section>
  );
}

function TwoFactor() {
  const { t } = useI18n();
  const qc = useQueryClient();
  const me = useMe();
  const [setup, setSetup] = useState<{ secret: string; uri: string; qr: string } | null>(null);
  const [code, setCode] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const enabled = me.data?.totp_enabled ?? false;

  async function start() {
    setError("");
    const s = await api<{ secret: string; uri: string }>("/admin/api/totp/setup");
    const qr = await QRCode.toDataURL(s.uri, { margin: 1, width: 200 });
    setSetup({ ...s, qr });
  }

  const enable = useMutation({
    mutationFn: () => api("/admin/api/totp/enable", { method: "POST", body: { secret: setup?.secret, code } }),
    onSuccess: () => {
      setSetup(null);
      setCode("");
      qc.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("settings.totpEnabled"));
    },
    onError: () => setError(t("login.invalidCode")),
  });
  const disable = useMutation({
    mutationFn: () => api("/admin/api/totp/disable", { method: "POST", body: { password, code } }),
    onSuccess: () => {
      setPassword("");
      setCode("");
      qc.invalidateQueries({ queryKey: ["me"] });
      toast.success(t("settings.totpDisabled"));
    },
    onError: (e) =>
      setError(
        e instanceof ApiError && e.code === "invalid_credentials"
          ? t("settings.wrongPassword")
          : t("login.invalidCode"),
      ),
  });

  const codeInput = (
    <Field id="totp-code" label={t("login.code")}>
      <Input
        id="totp-code"
        inputMode="numeric"
        autoComplete="one-time-code"
        maxLength={6}
        pattern="[0-9]{6}"
        className="w-40"
        value={code}
        onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
        required
      />
    </Field>
  );

  return (
    <Section title={t("settings.totp")} hint={enabled ? t("settings.totpOn") : t("settings.totpOff")}>
      <div className="grid gap-4 rounded-xl border bg-card p-4">
        {enabled ? (
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              setError("");
              disable.mutate();
            }}
          >
            <Field id="totp-pw" label={t("settings.currentPassword")}>
              <Input
                id="totp-pw"
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
              />
            </Field>
            {codeInput}
            {error && <p className="text-sm text-destructive">{error}</p>}
            <div>
              <Button type="submit" variant="outline" disabled={disable.isPending}>
                {t("settings.totpDisable")}
              </Button>
            </div>
          </form>
        ) : setup ? (
          <form
            className="grid gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              setError("");
              enable.mutate();
            }}
          >
            <p className="text-sm">{t("settings.totpScan")}</p>
            <img src={setup.qr} alt="" className="size-48 rounded-lg bg-white p-2" />
            <CopyField label={t("settings.totpKey")} value={setup.secret} />
            {codeInput}
            {error && <p className="text-sm text-destructive">{error}</p>}
            <div className="flex gap-2">
              <Button type="submit" disabled={enable.isPending}>
                {t("settings.totpEnable")}
              </Button>
              <Button type="button" variant="ghost" onClick={() => setSetup(null)}>
                {t("common.cancel")}
              </Button>
            </div>
          </form>
        ) : (
          <div>
            <Button onClick={start}>{t("settings.totpSetup")}</Button>
          </div>
        )}
      </div>
    </Section>
  );
}
