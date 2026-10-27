import { useState } from "react";
import { RotateCcw } from "lucide-react";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { useResetApp } from "../hooks/useResetApp";

/**
 * Start over: the last card on the Settings screen, behind a second click. It
 * erases what EinCast saved and restarts as a first run; media files stay.
 */
export function ResetSettings() {
  const { t } = useT();
  const { reset, pending, error } = useResetApp();
  const [confirming, setConfirming] = useState(false);

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <RotateCcw className="size-5" aria-hidden /> {t("settings.reset.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("settings.reset.intro")}</p>
        <p className="text-sm text-muted-foreground">{t("settings.reset.safe")}</p>
      </header>

      {pending ? (
        <p role="status" className="text-sm text-muted-foreground">
          {t("settings.reset.working")}
        </p>
      ) : confirming ? (
        <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("settings.reset.confirmGroup")}>
          <p className="text-sm text-destructive">{t("settings.reset.confirm")}</p>
          <GlassButton size="sm" className="text-destructive" onClick={() => void reset()}>
            <RotateCcw /> {t("settings.reset.confirmButton")}
          </GlassButton>
          <GlassButton size="sm" onClick={() => setConfirming(false)}>
            {t("common.cancel")}
          </GlassButton>
        </div>
      ) : (
        <div>
          <GlassButton size="sm" onClick={() => setConfirming(true)}>
            <RotateCcw /> {t("settings.reset.button")}
          </GlassButton>
        </div>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
