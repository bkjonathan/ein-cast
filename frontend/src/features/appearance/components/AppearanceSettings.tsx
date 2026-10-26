import { Monitor, Moon, Sun, type LucideIcon } from "lucide-react";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";
import { useAppearanceStore } from "../store";
import type { ThemeMode } from "../theme";

const options: readonly { mode: ThemeMode; label: string; icon: LucideIcon }[] = [
  { mode: "system", label: "System", icon: Monitor },
  { mode: "light", label: "Light", icon: Sun },
  { mode: "dark", label: "Dark", icon: Moon },
];

/** Light, dark, or whatever the OS is using. */
export function AppearanceSettings() {
  const mode = useAppearanceStore((s) => s.mode);
  const setMode = useAppearanceStore((s) => s.setMode);

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header>
        <h2 className="text-lg font-semibold">Appearance</h2>
        <p className="text-sm text-muted-foreground">
          “System” follows your computer’s setting. This choice applies to this window only.
        </p>
      </header>
      <div role="radiogroup" aria-label="Theme" className="flex flex-wrap gap-2">
        {options.map(({ mode: value, label, icon: Icon }) => (
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
            {label}
          </button>
        ))}
      </div>
    </GlassPanel>
  );
}
