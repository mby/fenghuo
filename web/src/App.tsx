import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { lazy, type ReactNode, Suspense } from "react";
import { BrowserRouter, Link, Route, Routes, useLocation } from "react-router";
import { Shell } from "@/components/Shell";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { ApiError } from "@/lib/api";
import { I18nProvider, useI18n } from "@/lib/i18n";
import { nodeDetailBackground } from "@/lib/node-detail-route";
import { Dashboard } from "@/pages/Dashboard";

// Charts and the admin UI are loaded on demand to keep the public page light.
const NodeDetailDialog = lazy(() => import("@/pages/NodeDetailDialog").then((m) => ({ default: m.NodeDetailDialog })));
const NodeDetail = lazy(() => import("@/pages/NodeDetail").then((m) => ({ default: m.NodeDetail })));
const Login = lazy(() => import("@/pages/Login").then((m) => ({ default: m.Login })));
const AdminLayout = lazy(() => import("@/pages/admin/AdminLayout").then((m) => ({ default: m.AdminLayout })));
const Servers = lazy(() => import("@/pages/admin/Servers").then((m) => ({ default: m.Servers })));
const Alerts = lazy(() => import("@/pages/admin/Alerts").then((m) => ({ default: m.Alerts })));
const SettingsPage = lazy(() => import("@/pages/admin/SettingsPage").then((m) => ({ default: m.SettingsPage })));
const Security = lazy(() => import("@/pages/admin/Security").then((m) => ({ default: m.Security })));
const Compare = lazy(() => import("@/pages/Compare").then((m) => ({ default: m.Compare })));

function Page({ children }: { children: ReactNode }) {
  return <Suspense fallback={null}>{children}</Suspense>;
}

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      refetchOnWindowFocus: true,
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
    },
  },
});

export function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <I18nProvider>
        <TooltipProvider>
          <BrowserRouter>
            <AppRoutes />
          </BrowserRouter>
          <Toaster position="bottom-right" />
        </TooltipProvider>
      </I18nProvider>
    </QueryClientProvider>
  );
}

function NotFound() {
  const { t } = useI18n();
  return (
    <Shell>
      <div className="py-24 text-center">
        <p className="text-lg">404</p>
        <Link to="/" className="mt-2 inline-block text-sm text-muted-foreground underline underline-offset-4">
          {t("detail.back")}
        </Link>
      </div>
    </Shell>
  );
}

function AppRoutes() {
  const location = useLocation();
  const background = nodeDetailBackground(location);
  return (
    <>
      <Routes location={background ?? location}>
        <Route path="/" element={<Dashboard />} />
        <Route
          path="/compare"
          element={
            <Page>
              <Compare />
            </Page>
          }
        />
        <Route
          path="/node/:uuid"
          element={
            <Page>
              <NodeDetail />
            </Page>
          }
        />
        <Route
          path="/login"
          element={
            <Page>
              <Login />
            </Page>
          }
        />
        <Route
          path="/admin"
          element={
            <Page>
              <AdminLayout />
            </Page>
          }
        >
          <Route
            path="security"
            element={
              <Page>
                <Security />
              </Page>
            }
          />
          <Route
            index
            element={
              <Page>
                <Servers />
              </Page>
            }
          />
          <Route
            path="alerts"
            element={
              <Page>
                <Alerts />
              </Page>
            }
          />
          <Route
            path="settings"
            element={
              <Page>
                <SettingsPage />
              </Page>
            }
          />
        </Route>
        <Route path="*" element={<NotFound />} />
      </Routes>
      {background && (
        <Routes>
          <Route
            path="/node/:uuid"
            element={
              <Page>
                <NodeDetailDialog />
              </Page>
            }
          />
        </Routes>
      )}
    </>
  );
}
