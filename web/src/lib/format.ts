import type { Lang } from "./i18n";

const UNITS = ["B", "KB", "MB", "GB", "TB", "PB"];

/** Binary (1024-based) sizes, labelled the way most VPS providers do. */
export function bytes(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return "0 B";
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), UNITS.length - 1);
  const v = n / 1024 ** i;
  return `${i === 0 || v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${UNITS[i]}`;
}

export const rate = (n: number) => `${bytes(n)}/s`;

export function percent(v: number): string {
  if (!Number.isFinite(v)) return "0%";
  return `${v >= 10 || v === 0 ? v.toFixed(0) : v.toFixed(1)}%`;
}

export const ratio = (used: number, total: number) => (total > 0 ? (used / total) * 100 : 0);

export const GB = 1024 ** 3;

export function duration(sec: number, lang: Lang): string {
  const s = Math.max(0, Math.floor(sec));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  if (lang === "zh") {
    if (d > 0) return `${d} 天 ${h} 小时`;
    if (h > 0) return `${h} 小时 ${m} 分`;
    if (m > 0) return `${m} 分钟`;
    return `${s} 秒`;
  }
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m`;
  return `${s}s`;
}

const locale = (lang: Lang) => (lang === "zh" ? "zh-CN" : "en");

export function date(ts: number, lang: Lang): string {
  return new Date(ts * 1000).toLocaleDateString(locale(lang), { year: "numeric", month: "short", day: "numeric" });
}

export function dateTime(ts: number, lang: Lang): string {
  return new Date(ts * 1000).toLocaleString(locale(lang), {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function fullDateTime(ts: number, lang: Lang): string {
  return new Date(ts * 1000).toLocaleString(locale(lang), {
    year: "numeric",
    month: "long",
    day: "numeric",
    weekday: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}

export function clock(ts: number, lang: Lang, withDate: boolean, withSeconds = false): string {
  return new Date(ts * 1000).toLocaleString(locale(lang), {
    ...(withDate ? { month: "numeric", day: "numeric" } : {}),
    hour: "2-digit",
    minute: "2-digit",
    ...(withSeconds ? { second: "2-digit" } : {}),
    hour12: false,
  });
}

/** Separator for joining short phrases in running text. */
export const listSep = (lang: Lang) => (lang === "zh" ? "，" : ", ");

export function daysUntil(ts: number, now = Date.now() / 1000): number {
  return Math.ceil((ts - now) / 86400);
}

/** Converts a unix timestamp to the value of an <input type="date">. */
export function toDateInput(ts: number): string {
  if (!ts) return "";
  const d = new Date(ts * 1000);
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`;
}

export function fromDateInput(v: string): number {
  if (!v) return 0;
  const [y, m, d] = v.split("-").map(Number);
  return Math.floor(new Date(y, m - 1, d, 23, 59, 59).getTime() / 1000);
}
