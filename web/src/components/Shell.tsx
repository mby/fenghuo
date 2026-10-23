import { Moon, Sun } from "lucide-react";
import type { ReactNode } from "react";
import { Link } from "react-router";
import { Button } from "@/components/ui/button";
import { isDemoMode, useMeta } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { setTheme, useTheme } from "@/lib/theme";

export function Shell({ children, nav }: { children: ReactNode; nav?: ReactNode }) {
  const { t, lang, setLang } = useI18n();
  const { resolved } = useTheme();
  const meta = useMeta();
  const site = meta.data?.site_name ?? "Fenghuo";
  const demo = isDemoMode();

  return (
    <div className="flex min-h-dvh flex-col">
      <a
        href="#main-content"
        className="sr-only z-50 rounded-md bg-card px-4 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2"
      >
        {t("nav.skip")}
      </a>
      <header className="mx-auto flex w-full max-w-7xl flex-wrap items-center justify-between gap-x-4 gap-y-2 px-4 py-4 sm:px-6">
        <Link to="/" className="flex min-w-0 flex-1 items-center gap-2 rounded-md font-semibold tracking-tight">
          <img src="/favicon.svg" alt="" className="size-6 shrink-0" />
          <span className="truncate">{site}</span>
        </Link>
        <nav className="flex shrink-0 items-center gap-1" aria-label={t("nav.label")}>
          {nav}
          {meta.data?.admin && !nav && (
            <Button asChild variant="ghost" size="sm">
              <Link to="/admin">{t("nav.admin")}</Link>
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={() => setLang(lang === "zh" ? "en" : "zh")}>
            {t("nav.lang")}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            className="size-8"
            aria-label={t("nav.theme")}
            onClick={() => setTheme(resolved === "dark" ? "light" : "dark")}
          >
            {resolved === "dark" ? <Sun className="size-4" /> : <Moon className="size-4" />}
          </Button>
        </nav>
      </header>
      {demo && (
        <div className="mx-auto mb-2 flex w-full max-w-7xl items-center justify-between gap-3 px-4 text-xs text-muted-foreground sm:px-6">
          <span>{t("demo.mode")}</span>
          <a className="shrink-0 underline underline-offset-4 hover:text-foreground" href="/?demo=0">
            {t("demo.exit")}
          </a>
        </div>
      )}
      <main id="main-content" tabIndex={-1} className="mx-auto w-full max-w-7xl min-w-0 flex-1 px-4 pb-12 sm:px-6">
        {children}
      </main>
      <footer className="mx-auto w-full max-w-7xl px-4 pb-6 text-xs text-muted-foreground sm:px-6">
        <a href="https://github.com/mby/fenghuo" target="_blank" rel="noreferrer" className="hover:text-foreground">
          Fenghuo {meta.data?.version ?? ""}
        </a>
      </footer>
    </div>
  );
}
