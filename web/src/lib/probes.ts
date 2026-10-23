import type { Settings } from "@/lib/types";

export const configureScript = "https://raw.githubusercontent.com/mby/fenghuo/main/deploy/configure-probes.sh";

export function shellQuote(value: string) {
  return `'${value.replaceAll("'", "'\\''")}'`;
}

export function probeInstallFlags(settings: Settings) {
  return settings.probe_targets
    ? ` --ping ${shellQuote(settings.probe_targets)} --ping-interval ${settings.probe_interval}s`
    : "";
}

export function probeConfigureCommand(settings: Settings) {
  const flags = settings.probe_targets
    ? `--ping ${shellQuote(settings.probe_targets)} --ping-interval ${settings.probe_interval}s`
    : "--disable";
  return `curl -fsSL ${configureScript} | sudo bash -s -- ${flags}`;
}
