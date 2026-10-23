import { keepPreviousData, useQuery } from "@tanstack/react-query";
import type { AdminNode, Me, Meta, Metric, PingSeries, PublicNode } from "./types";

export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message?: string,
  ) {
    super(message || code);
  }
}

const DEMO_KEY = "fenghuo-ui-demo";

export function isDemoMode(): boolean {
  if (!import.meta.env.DEV) return false;
  const choice = new URLSearchParams(window.location.search).get("demo");
  try {
    if (choice === "1") sessionStorage.setItem(DEMO_KEY, "1");
    if (choice === "0") sessionStorage.removeItem(DEMO_KEY);
    return choice === "1" || (choice !== "0" && sessionStorage.getItem(DEMO_KEY) === "1");
  } catch {
    return choice === "1";
  }
}

export async function api<T>(path: string, opts: { method?: string; body?: unknown } = {}): Promise<T> {
  if (import.meta.env.DEV && (opts.method ?? "GET") === "GET" && isDemoMode()) {
    const { demoResponse } = await import("./demo");
    const fixture = demoResponse(path);
    if (fixture !== undefined) return fixture as T;
    if (path.startsWith("/api/v1/")) throw new ApiError(404, "demo_not_found");
  }
  const res = await fetch(path, {
    method: opts.method ?? "GET",
    credentials: "same-origin",
    headers: opts.body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(res.status, data.error ?? "error", data.message);
  return data as T;
}

export const LIVE_INTERVAL = 3000;

export function useMeta() {
  return useQuery({ queryKey: ["meta"], queryFn: () => api<Meta>("/api/v1/meta"), staleTime: 60_000 });
}

export function useNodes() {
  return useQuery({
    queryKey: ["nodes"],
    queryFn: () => api<PublicNode[]>("/api/v1/nodes"),
    refetchInterval: LIVE_INTERVAL,
    retry: (count, err) => !(err instanceof ApiError && err.status === 404) && count < 2,
  });
}

export type Range = "live" | "1h" | "6h" | "24h" | "7d" | "30d" | "90d";

export function useMetrics(uuid: string, range: Range) {
  return useQuery({
    queryKey: ["metrics", uuid, range],
    queryFn: () => api<Metric[]>(`/api/v1/nodes/${encodeURIComponent(uuid)}/metrics?range=${range}`),
    refetchInterval: range === "live" ? LIVE_INTERVAL : 60_000,
    placeholderData: keepPreviousData,
  });
}

export function usePing(uuid: string, range: Range, enabled: boolean) {
  return useQuery({
    queryKey: ["ping", uuid, range],
    queryFn: () => api<PingSeries[]>(`/api/v1/nodes/${encodeURIComponent(uuid)}/ping?range=${range}`),
    refetchInterval: 60_000,
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function useMe(enabled = true) {
  return useQuery({
    queryKey: ["me"],
    queryFn: () => api<Me>("/admin/api/me"),
    retry: false,
    enabled,
  });
}

export function useAdminNodes() {
  return useQuery({
    queryKey: ["admin", "nodes"],
    queryFn: () => api<AdminNode[]>("/admin/api/nodes"),
    refetchInterval: 10_000,
  });
}
