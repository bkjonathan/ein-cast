import { useState } from "react";
import { Check, Copy, Eye, EyeOff, KeyRound, ShieldAlert } from "lucide-react";
import { useCopy } from "@/shared/hooks/useCopy";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Switch } from "@/shared/ui/Switch";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { inputFrom } from "../dirty";
import { useSettingsStore } from "../store";

/** Turns the access token on or off, shows it, and replaces it. */
export function AccessSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const { save, rotate, pending, error } = useSettingsCommands();
  const [reveal, setReveal] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const { copied, copy } = useCopy();
  const { t } = useT();

  if (!saved) return null;
  const on = saved.requireToken;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">{t("settings.access.title")}</h2>
          <p className="text-sm text-muted-foreground">
            {on ? t("settings.access.on") : t("settings.access.off")}
          </p>
        </div>
        <Switch
          checked={on}
          label={t("settings.access.require")}
          disabled={pending || (!on && saved.dlna)}
          onChange={(next) => save(inputFrom(saved, { requireToken: next }))}
        />
      </header>

      {!on && saved.dlna && (
        <p className="text-sm text-muted-foreground">
          {t("settings.access.dlnaOn")}
        </p>
      )}

      {!on && !saved.dlna && (
        <p className="flex items-center gap-2 text-sm text-warning">
          <ShieldAlert className="size-4 shrink-0" aria-hidden />
          {t("settings.access.recommend")}
        </p>
      )}

      {on && (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <code
              className="glass min-w-0 flex-1 truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text"
              aria-label={t("settings.access.title")}
            >
              {reveal ? saved.accessToken : "•".repeat(24)}
            </code>
            <GlassButton
              size="icon"
              onClick={() => setReveal(!reveal)}
              aria-label={reveal ? t("settings.access.hide") : t("settings.access.show")}
            >
              {reveal ? <EyeOff /> : <Eye />}
            </GlassButton>
            <GlassButton size="sm" onClick={() => copy(saved.accessToken)}>
              {copied === saved.accessToken ? <Check /> : <Copy />}
              {copied === saved.accessToken ? t("common.copied") : t("common.copy")}
            </GlassButton>
          </div>

          {!saved.tokenPersisted && (
            <p role="status" className="text-sm text-warning">
              {t("settings.access.noKeychain")}
            </p>
          )}

          {confirming ? (
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-sm text-muted-foreground">
                {t("settings.access.lockout")}
              </p>
              <GlassButton
                size="sm"
                disabled={pending}
                onClick={async () => {
                  await rotate();
                  setConfirming(false);
                }}
              >
                <KeyRound /> {t("settings.access.generate")}
              </GlassButton>
              <GlassButton size="sm" onClick={() => setConfirming(false)}>
                {t("common.cancel")}
              </GlassButton>
            </div>
          ) : (
            <GlassButton size="sm" onClick={() => setConfirming(true)}>
              <KeyRound /> {t("settings.access.replace")}
            </GlassButton>
          )}
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
