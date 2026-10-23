import { useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { Link, Navigate, NavLink, Outlet, useNavigate } from "react-router";
import { Shell } from "@/components/Shell";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { api, useMe, useMeta } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { cn } from "@/lib/utils";

export function AdminLayout() {
  const { t } = useI18n();
  const meta = useMeta();
  const me = useMe(meta.data?.admin === true);
  const navigate = useNavigate();
  const qc = useQueryClient();

  if (meta.data && !meta.data.admin) {
    return (
      <Shell>
        <p className="mx-auto max-w-md py-24 text-center text-muted-foreground">{t("login.notHere")}</p>
      </Shell>
    );
  }
  if (me.isError) return <Navigate to="/login" replace />;

  async function logout() {
    await api("/admin/api/logout", { method: "POST" });
    qc.clear();
    navigate("/login", { replace: true });
  }

  const tabs = [
    { to: "/admin", label: t("admin.servers"), end: true },
    { to: "/admin/alerts", label: t("admin.alerts"), end: false },
    { to: "/admin/security", label: t("admin.security"), end: false },
    { to: "/admin/settings", label: t("admin.settings"), end: false },
  ];

  return (
    <Shell
      nav={
        <Button variant="ghost" size="sm" onClick={logout}>
          {t("nav.logout")}
        </Button>
      }
    >
      <nav className="mt-2 mb-6 flex gap-1 overflow-x-auto overflow-y-hidden border-b" aria-label={t("nav.admin")}>
        {tabs.map((tab) => (
          <NavLink
            key={tab.to}
            to={tab.to}
            end={tab.end}
            className={({ isActive }) =>
              cn(
                "-mb-px shrink-0 border-b-2 px-3 py-3 text-sm transition-colors",
                isActive
                  ? "border-foreground font-medium text-foreground"
                  : "border-transparent text-muted-foreground hover:text-foreground",
              )
            }
          >
            {tab.label}
          </NavLink>
        ))}
      </nav>
      {me.data?.default_password && (
        <Link
          to="/admin/settings"
          className="mb-6 flex items-center gap-2 rounded-lg border border-amber/40 bg-amber/10 px-3 py-2 text-sm"
        >
          <TriangleAlert className="size-4 shrink-0 text-amber" aria-hidden />
          {t("admin.defaultPassword")}
        </Link>
      )}
      {me.isPending ? <Skeleton className="h-64 rounded-xl" /> : <Outlet />}
    </Shell>
  );
}
