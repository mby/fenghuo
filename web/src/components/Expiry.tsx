import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { expiryText } from "@/lib/display";
import { daysUntil, fullDateTime } from "@/lib/format";
import { LIMITS } from "@/lib/health";
import { useI18n } from "@/lib/i18n";
import type { PublicNode } from "@/lib/types";
import { cn } from "@/lib/utils";

/** "Expires in 12 days", with the exact date and time on hover. */
export function Expiry({
  node,
  now,
  className,
  compact = false,
}: {
  node: PublicNode;
  now: number;
  className?: string;
  compact?: boolean;
}) {
  const { t, lang } = useI18n();
  const days = daysUntil(node.expire_at, now);
  const fullText = expiryText(node, now, t);
  const text = compact ? expiryText(node, now, t, true) : fullText;
  if (!text) return null;
  const soon = days <= LIMITS.expireDays;
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <time
          dateTime={new Date(node.expire_at * 1000).toISOString()}
          className={cn(
            "relative z-10 cursor-help whitespace-nowrap underline decoration-muted-foreground/40 decoration-dotted underline-offset-2",
            soon && "text-ember",
            className,
          )}
        >
          {compact ? (
            <>
              <span className="sr-only">{fullText}</span>
              <span aria-hidden>{text}</span>
            </>
          ) : (
            text
          )}
        </time>
      </TooltipTrigger>
      <TooltipContent>{t("expire.at", { date: fullDateTime(node.expire_at, lang) })}</TooltipContent>
    </Tooltip>
  );
}
