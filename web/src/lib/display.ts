import { daysUntil, duration, listSep } from "./format";
import { issues } from "./health";
import type { Lang, MessageKey } from "./i18n";
import type { NodeInfo, PublicNode } from "./types";

type T = (key: MessageKey, vars?: Record<string, string | number>) => string;

/** "Debian GNU/Linux 13 (trixie)" -> "Debian 13"; the full name is shown on the detail page. */
export function shortOS(os: string): string {
  return os
    .replace(/ GNU\/Linux/, "")
    .replace(/\s*\(.*\)\s*$/, "")
    .replace(/ LTS$/, "")
    .trim();
}

export function specs(info: NodeInfo): string {
  return [info.arch, info.cpu_cores ? `${info.cpu_cores}C` : "", info.virt]
    .filter((v) => v && v !== "none")
    .join(" / ");
}

export function summaryLine(info: NodeInfo, capacity?: string): string {
  const hardware =
    capacity === undefined
      ? specs(info)
      : [info.arch, capacity, info.virt].filter((v) => v && v !== "none").join(" / ");
  return [shortOS(info.os), hardware].filter(Boolean).join(" / ");
}

export function issueText(node: PublicNode, now: number, t: T, lang: Lang): string {
  return issues(node)
    .map((p) =>
      p === "offline" && node.last_seen
        ? t("node.offlineFor", { d: duration(now - node.last_seen, lang) })
        : t(`issue.${p}`),
    )
    .join(listSep(lang));
}

export function expiryText(node: PublicNode, now: number, t: T): string {
  if (node.expire_at <= 0) return "";
  const days = daysUntil(node.expire_at, now);
  if (days < 0) return t("node.expired");
  if (days === 0) return t("node.expiresToday");
  return t("node.expiresIn", { n: days });
}
