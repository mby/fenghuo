import type { Lang } from "./i18n";
import type { Traffic } from "./types";

/** Render the server's billing clock, regardless of the viewer's browser timezone. */
export function trafficResetTime(traffic: Traffic, lang: Lang): { iso: string; date: string; zone: string } | null {
  const at = traffic.next_reset_at;
  const offset = traffic.reset_utc_offset;
  if (!Number.isFinite(at) || at <= 0 || !Number.isInteger(offset) || Math.abs(offset) > 14 * 3600) return null;
  const instant = new Date(at * 1000);
  const clock = new Date((at + offset) * 1000);
  if (!Number.isFinite(instant.getTime()) || !Number.isFinite(clock.getTime())) return null;
  const hours = String(Math.floor(Math.abs(offset) / 3600)).padStart(2, "0");
  const minutes = String(Math.floor((Math.abs(offset) % 3600) / 60)).padStart(2, "0");
  return {
    iso: instant.toISOString(),
    date: clock.toLocaleString(lang === "zh" ? "zh-CN" : "en", {
      timeZone: "UTC",
      year: "numeric",
      month: "2-digit",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }),
    zone: `UTC${offset < 0 ? "−" : "+"}${hours}:${minutes}`,
  };
}
