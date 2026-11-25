import { useState } from "react";
import { Field } from "@/components/Field";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { useI18n } from "@/lib/i18n";

export function ProbePresetFields({
  targets,
  cards,
  onChange,
  protocols = "",
  onProtocolsChange,
}: {
  targets: string;
  cards: string;
  onChange: (targets: string, cards: string) => void;
  protocols?: string;
  onProtocolsChange?: (value: string) => void;
}) {
  const { t } = useI18n();
  const [text, setText] = useState((targets ?? "").replaceAll(",", "\n"));
  const names = [
    ...new Set(
      (targets ?? "")
        .split(",")
        .map((entry) => entry.split("=")[0])
        .filter(Boolean),
    ),
  ];
  const selected = cards ? cards.split(",") : names.slice(0, 3);
  return (
    <div className="grid gap-3">
      <Field id="probe-targets" label={t("settings.probeTargets")} hint={t("settings.probeTargetsHint")}>
        <Textarea
          id="probe-targets"
          rows={6}
          value={text}
          placeholder={t("settings.probeExample")}
          onChange={(e) => {
            setText(e.target.value);
            const next = e.target.value
              .split(/\r?\n/)
              .map((line) => line.trim())
              .filter(Boolean)
              .join(",");
            const nextNames = next.split(",").map((entry) => entry.split("=")[0]);
            onChange(
              next,
              (cards ?? "")
                .split(",")
                .filter((name) => name && nextNames.includes(name))
                .join(","),
            );
          }}
        />
      </Field>
      <p className="text-xs text-muted-foreground">{t("settings.probeCapacity")}</p>
      {onProtocolsChange && names.length > 0 && (
        <fieldset className="grid gap-2">
          <legend className="mb-2 text-sm font-medium">{t("network.protocol")}</legend>
          {names.map((name) => {
            const kinds = Object.fromEntries(
              protocols
                .split(",")
                .filter(Boolean)
                .map((entry) => entry.split("=")),
            );
            return (
              <div key={name} className="flex min-w-0 items-center justify-between gap-3">
                <span className="break-all text-sm">{name}</span>
                <Select
                  value={kinds[name] ?? "tcp"}
                  onValueChange={(kind) => {
                    kinds[name] = kind;
                    onProtocolsChange(names.map((key) => `${key}=${kinds[key] ?? "tcp"}`).join(","));
                  }}
                >
                  <SelectTrigger className="w-36 shrink-0" aria-label={`${name} ${t("network.protocol")}`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="tcp">{t("network.tcp")}</SelectItem>
                    <SelectItem value="icmp">{t("network.icmp")}</SelectItem>
                    <SelectItem value="both">{t("network.both")}</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            );
          })}
          <p className="text-xs text-muted-foreground">{t("network.localConfigHint")}</p>
        </fieldset>
      )}
      {names.length > 0 && (
        <fieldset className="grid gap-2">
          <legend className="mb-2 text-sm font-medium">{t("settings.probeHomepage")}</legend>
          <p className="text-xs text-muted-foreground">{t("settings.probeHomepageHint")}</p>
          <div className="flex flex-wrap gap-x-4 gap-y-2">
            {names.map((name) => (
              <label key={name} className="flex min-h-9 items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={selected.includes(name)}
                  disabled={
                    (!selected.includes(name) && selected.length >= 3) ||
                    (selected.includes(name) && selected.length === 1)
                  }
                  onChange={(e) =>
                    onChange(
                      targets,
                      (e.target.checked ? [...selected, name] : selected.filter((item) => item !== name)).join(","),
                    )
                  }
                  className="size-4 accent-primary"
                />
                <span className="break-all">{name}</span>
              </label>
            ))}
          </div>
        </fieldset>
      )}
    </div>
  );
}
