import { useState } from "react";
import { ShieldAlert, Tv } from "lucide-react";
import { useServerStore } from "@/features/server";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Switch } from "@/shared/ui/Switch";
import { inputFrom, smartTvsInsteadOfToken, tvNeedsRestart } from "../dirty";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";

/**
 * Smart TV (DLNA/UPnP) discovery. Off by default. A TV cannot send the access
 * token, so Go allows only one of the two; when the token is on, this screen
 * explains the trade and lets the person make it in one step (Go enforces it).
 */
export function TvSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const running = useServerStore((s) => s.status);
  const { save, pending, error } = useSettingsCommands();
  const [choosing, setChoosing] = useState(false);
  const { t } = useT();
  if (!saved) return null;

  const on = saved.dlna;
  const tradesToken = !on && saved.requireToken;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Tv className="size-5" aria-hidden /> {t("settings.tv.title")}
          </h2>
          <p className="text-sm text-muted-foreground">
            {on ? t("settings.tv.on") : t("settings.tv.off")}
          </p>
        </div>
        <Switch
          checked={on}
          label={t("settings.tv.switch")}
          disabled={pending}
          onChange={(next) => {
            if (next && tradesToken) return setChoosing(true);
            void save(inputFrom(saved, { dlna: next }));
          }}
        />
      </header>

      {tradesToken && !choosing && (
        <p className="text-sm text-muted-foreground">
          {t("settings.tv.tradesToken")}
        </p>
      )}

      {choosing && (
        <div role="alertdialog" aria-label={t("settings.tv.dialogLabel")} className="glass space-y-3 rounded-glass-sm p-4">
          <p className="flex items-start gap-2 text-sm text-warning">
            <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            {t("settings.tv.dialogWarning")}
          </p>
          <div className="flex flex-wrap gap-2">
            <GlassButton
              size="sm"
              disabled={pending}
              onClick={async () => {
                if (await save(smartTvsInsteadOfToken(saved))) setChoosing(false);
              }}
            >
              {t("settings.tv.disableToken")}
            </GlassButton>
            <GlassButton size="sm" onClick={() => setChoosing(false)}>
              {t("settings.tv.keepToken")}
            </GlassButton>
          </div>
        </div>
      )}

      {(on || !tradesToken) && (
        <p className="text-xs text-muted-foreground">
          {t("settings.tv.openNote")}
        </p>
      )}
      {tvNeedsRestart(running, saved) && (
        <p className="text-sm text-warning">{t("settings.tv.restart")}</p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
