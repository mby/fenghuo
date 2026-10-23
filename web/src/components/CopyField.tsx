import { Check, Copy } from "lucide-react";
import { useState } from "react";
import { Button } from "@/components/ui/button";
import { useI18n } from "@/lib/i18n";

export function CopyField({ value, label }: { value: string; label?: string }) {
  const { t } = useI18n();
  const [copied, setCopied] = useState(false);

  async function copy() {
    await navigator.clipboard.writeText(value);
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  }

  return (
    <div className="grid gap-1.5">
      {label && <span className="text-sm font-medium">{label}</span>}
      <div className="flex items-start gap-2 rounded-lg border bg-muted/50 p-2.5">
        <code className="min-w-0 flex-1 break-all font-mono text-xs leading-relaxed">{value}</code>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="size-7 shrink-0"
          onClick={copy}
          aria-label={copied ? t("common.copied") : t("common.copy")}
        >
          {copied ? <Check className="size-3.5" /> : <Copy className="size-3.5" />}
        </Button>
      </div>
    </div>
  );
}
