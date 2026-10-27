import { Power } from "lucide-react";
import { useT, type MessageKey } from "@/shared/i18n";
import { GlassPanel } from "@/shared/ui/glass";
import { Switch } from "@/shared/ui/Switch";
import { inputFrom } from "../dirty";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { usePlatform } from "../hooks/usePlatform";
import { useSettingsStore } from "../store";
import { hideOnCloseHelp, startupRows, type ToggleKey } from "../startup";

/**
 * How EinCast starts and stays on: opening, sleeping and closing the window. The
 * point is that the server is there when a TV looks for it, without a person
 * having to remember to open the app.
 */
export function StartupSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const platform = usePlatform();
  const { t } = useT();
  const { save, pending, error } = useSettingsCommands();
  if (!saved) return null;

  const row = (key: ToggleKey, titleKey: MessageKey, helpKey: MessageKey) => (
    <li key={key} className="flex items-start justify-between gap-4">
      <div className="space-y-0.5">
        <p className="text-sm font-medium">{t(titleKey)}</p>
        <p className="text-xs text-muted-foreground">{t(helpKey)}</p>
      </div>
      <Switch
        checked={saved[key]}
        label={t(titleKey)}
        disabled={pending}
        onChange={(next) => void save(inputFrom(saved, { [key]: next }))}
      />
    </li>
  );

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Power className="size-5" aria-hidden /> {t("settings.startup.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("settings.startup.intro")}</p>
      </header>
      <ul className="flex flex-col gap-4">
        {startupRows.map((r) => row(r.key, r.titleKey, r.helpKey))}
        {row("hideOnClose", "settings.startup.hideOnClose.title", hideOnCloseHelp(platform))}
      </ul>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
