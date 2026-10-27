import { Monitor, Moon, Sun, type LucideIcon } from "lucide-react";
import { useT, type MessageKey } from "@/shared/i18n";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";
import { useAppearanceStore } from "../store";
import type { ThemeMode } from "../theme";

const options: readonly { mode: ThemeMode; labelKey: MessageKey; icon: LucideIcon }[] = [
  { mode: "system", labelKey: "appearance.system", icon: Monitor },
  { mode: "light", labelKey: "appearance.light", icon: Sun },
  { mode: "dark", labelKey: "appearance.dark", icon: Moon },
];

/** Light, dark, or whatever the OS is using. */
export function AppearanceSettings() {
  const mode = useAppearanceStore((s) => s.mode);
  const setMode = useAppearanceStore((s) => s.setMode);
  const { t } = useT();

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header>
        <h2 className="text-lg font-semibold">{t("appearance.title")}</h2>
        <p className="text-sm text-muted-foreground">
          {t("appearance.help")}
        </p>
      </header>
      <div role="radiogroup" aria-label={t("appearance.theme")} className="flex flex-wrap gap-2">
        {options.map(({ mode: value, labelKey, icon: Icon }) => (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={mode === value}
            onClick={() => setMode(value)}
            className={cn(
              "app-no-drag glass flex items-center gap-2 rounded-glass-sm px-4 py-2 text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
              mode === value
                ? "ring-2 ring-primary"
                : "text-muted-foreground hover:text-foreground",
            )}
          >
            <Icon className="size-4" aria-hidden />
            {t(labelKey)}
          </button>
        ))}
      </div>
    </GlassPanel>
  );
}
