import { daysUntil, ratio } from "./format";
import type { PublicNode } from "./types";

export type Issue = "offline" | "cpu" | "mem" | "disk" | "traffic" | "expire";

/** Thresholds above which a node is shown as needing attention on the dashboard. */
export const LIMITS = { usage: 90, warn: 80, traffic: 90, expireDays: 3 };

export function isPending(n: PublicNode) {
  return !n.online && !n.has_reported;
}

export function issues(n: PublicNode): Issue[] {
  const out: Issue[] = [];
  if (isPending(n)) return out;
  if (!n.online) out.push("offline");
  const s = n.status;
  if (n.online && s) {
    if (s.cpu >= LIMITS.usage) out.push("cpu");
    if (ratio(s.mem_used, s.mem_total) >= LIMITS.usage) out.push("mem");
    if (ratio(s.disk_used, s.disk_total) >= LIMITS.usage) out.push("disk");
  }
  if (n.traffic.limit > 0 && ratio(n.traffic.used, n.traffic.limit) >= LIMITS.traffic) out.push("traffic");
  if (n.expire_at > 0 && daysUntil(n.expire_at) <= LIMITS.expireDays) out.push("expire");
  return out;
}

export type Level = "ok" | "warn" | "hot";

export function level(pct: number): Level {
  if (pct >= LIMITS.usage) return "hot";
  if (pct >= LIMITS.warn) return "warn";
  return "ok";
}
