import { useState } from "react";
import { Field } from "@/components/Field";
import { Textarea } from "@/components/ui/textarea";
import { useI18n } from "@/lib/i18n";

export function ProbePresetFields({
  targets,
  cards,
  onChange,
}: {
  targets: string;
  cards: string;
  onChange: (targets: string, cards: string) => void;
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
