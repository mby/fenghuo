import { cn } from "@/lib/utils";

export type DotState = "ok" | "lit" | "pending";

/** Quiet jade when healthy; a flickering ember when the beacon is lit. */
export function StatusDot({ state, className }: { state: DotState; className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "inline-block size-2 shrink-0 rounded-full",
        state === "ok" && "bg-jade",
        state === "lit" && "flicker bg-ember shadow-[0_0_8px_var(--ember)]",
        state === "pending" && "border border-muted-foreground/60",
        className,
      )}
    />
  );
}
