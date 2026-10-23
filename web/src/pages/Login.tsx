import { useQueryClient } from "@tanstack/react-query";
import { type FormEvent, useState } from "react";
import { useNavigate } from "react-router";
import { Shell } from "@/components/Shell";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ApiError, api, useMeta } from "@/lib/api";
import { useI18n } from "@/lib/i18n";

export function Login() {
  const { t } = useI18n();
  const meta = useMeta();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [needCode, setNeedCode] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/admin/api/login", { method: "POST", body: { username, password, code } });
      await qc.invalidateQueries();
      navigate("/admin", { replace: true });
    } catch (err) {
      const c = err instanceof ApiError ? err.code : "";
      if (c === "totp_required") setNeedCode(true);
      else if (c === "invalid_code") setError(t("login.invalidCode"));
      else if (c === "too_many_attempts") setError(t("login.tooMany"));
      else if (err instanceof ApiError && err.status === 404) setError(t("login.notHere"));
      else setError(t("login.invalid"));
    } finally {
      setBusy(false);
    }
  }

  if (meta.data && !meta.data.admin) {
    return (
      <Shell>
        <p className="mx-auto max-w-md py-24 text-center text-muted-foreground">{t("login.notHere")}</p>
      </Shell>
    );
  }

  return (
    <Shell>
      <form onSubmit={submit} className="mx-auto mt-16 grid max-w-sm gap-4 rounded-xl border bg-card p-6">
        <h1 className="text-xl font-medium">{t("login.title")}</h1>
        <div className="grid gap-1.5">
          <Label htmlFor="username">{t("login.username")}</Label>
          <Input
            id="username"
            autoComplete="username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
          />
        </div>
        <div className="grid gap-1.5">
          <Label htmlFor="password">{t("login.password")}</Label>
          <Input
            id="password"
            type="password"
            autoComplete="current-password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            autoFocus
          />
        </div>
        {needCode && (
          <div className="grid gap-1.5">
            <Label htmlFor="code">{t("login.code")}</Label>
            <Input
              id="code"
              inputMode="numeric"
              autoComplete="one-time-code"
              pattern="[0-9]{6}"
              maxLength={6}
              value={code}
              onChange={(e) => setCode(e.target.value.replace(/\D/g, ""))}
              required
              autoFocus
            />
            <p className="text-xs text-muted-foreground">{t("login.codeHint")}</p>
          </div>
        )}
        {error && (
          <p role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
        <Button type="submit" disabled={busy}>
          {t("login.submit")}
        </Button>
      </form>
    </Shell>
  );
}
