import { useI18n } from "@/lib/i18n";
import { publicTags } from "@/lib/node-view";
import { cn } from "@/lib/utils";

export function NodeTags({ value, className }: { value?: string; className?: string }) {
  const { t } = useI18n();
  const tags = publicTags(value);
  if (!tags.length) return null;
  return (
    <ul aria-label={t("card.publicTags")} className={cn("flex min-w-0 flex-wrap gap-1.5", className)}>
      {tags.map((tag) => (
        <li
          key={tag}
          title={tag}
          className="max-w-full truncate rounded-md border px-1.5 py-0.5 text-xs text-muted-foreground"
        >
          {tag}
        </li>
      ))}
    </ul>
  );
}
