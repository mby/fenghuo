import type { Settings } from "@/lib/types";

export const configureScript = "/configure-probes.sh";

export function shellQuote(value: string) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export function probeInstallFlags(settings: Settings) {
  const protocols = Object.fromEntries((settings.probe_protocols ?? "").split(",").map((v) => v.split("=")));
  const tcp: string[] = [];
  const icmp: string[] = [];
  for (const entry of settings.probe_targets.split(",").filter(Boolean)) {
    const [name, address] = entry.split("=");
    const kind = protocols[name] ?? "tcp";
    if (kind !== "icmp") tcp.push(entry);
    if (kind !== "tcp") icmp.push(`${name}=${address.split(":")[0]}`);
  }
  return `${tcp.length ? ` --ping ${shellQuote(tcp.join(","))} --ping-interval ${settings.probe_interval}s` : ""}${icmp.length ? ` --icmp ${shellQuote(icmp.join(","))} --icmp-interval ${settings.probe_interval}s` : ""}`;
}

export function probeConfigureCommand(settings: Settings) {
  const flags = probeInstallFlags(settings).trim() || "--disable";
  const origin =
    settings.public_url || (typeof window !== "undefined" ? window.location.origin : "https://probe.example.com");
  return `curl -fsSL ${origin.replace(/\/$/, "")}${configureScript} | sudo bash -s -- ${flags}`;
}
