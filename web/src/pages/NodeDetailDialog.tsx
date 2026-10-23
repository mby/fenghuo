import { ArrowUpRight, X } from "lucide-react";
import { useState } from "react";
import { Link, useLocation, useNavigate } from "react-router";
import { Button } from "@/components/ui/button";
import { Dialog, DialogClose, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { useI18n } from "@/lib/i18n";
import { NodeDetail } from "./NodeDetail";

export function NodeDetailDialog() {
  const { t } = useI18n();
  const navigate = useNavigate();
  const location = useLocation();
  const [returnFocus] = useState(() => (document.activeElement instanceof HTMLElement ? document.activeElement : null));
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open) navigate(-1);
      }}
    >
      <DialogContent
        showCloseButton={false}
        aria-describedby={undefined}
        className="h-[calc(100dvh-1rem)] max-w-[calc(100%-1rem)] grid-rows-[auto_minmax(0,1fr)] gap-0 overflow-hidden rounded-xl p-0 sm:h-[90dvh] sm:max-w-6xl motion-reduce:animate-none"
        onCloseAutoFocus={(e) => {
          e.preventDefault();
          if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
        }}
      >
        <div className="flex items-center justify-between gap-3 border-b px-4 py-3 sm:px-6">
          <DialogTitle className="text-base font-medium">{t("detail.quickView")}</DialogTitle>
          <div className="flex items-center gap-1">
            <Button asChild variant="ghost" size="sm">
              <Link to={`${location.pathname}${location.search}${location.hash}`} replace state={null}>
                {t("detail.fullPage")}
                <ArrowUpRight className="size-4" aria-hidden />
              </Link>
            </Button>
            <DialogClose asChild>
              <Button variant="ghost" size="icon" aria-label={t("common.close")}>
                <X className="size-4" />
              </Button>
            </DialogClose>
          </div>
        </div>
        <div className="min-w-0 overflow-y-auto overscroll-contain px-4 pb-6 sm:px-6">
          <NodeDetail embedded />
        </div>
      </DialogContent>
    </Dialog>
  );
}
