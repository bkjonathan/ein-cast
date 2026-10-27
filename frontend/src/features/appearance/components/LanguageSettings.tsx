import { Languages } from "lucide-react";
import { useLocaleStore, useT, type LocalePreference, type MessageKey } from "@/shared/i18n";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";

const options: readonly { value: LocalePreference; labelKey: MessageKey }[] = [
  { value: "system", labelKey: "language.system" },
  // A language is always named in itself, so a person who cannot read the current one can still find theirs.
  { value: "en", labelKey: "language.en" },
  { value: "my", labelKey: "language.my" },
];

/** The language of this window: the computer's, English or Burmese. */
export function LanguageSettings() {
  const preference = useLocaleStore((s) => s.preference);
  const setPreference = useLocaleStore((s) => s.setPreference);
  const { t } = useT();

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Languages className="size-5" aria-hidden /> {t("language.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("language.help")}</p>
      </header>
      <div role="radiogroup" aria-label={t("language.group")} className="flex flex-wrap gap-2">
        {options.map(({ value, labelKey }) => (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={preference === value}
            onClick={() => setPreference(value)}
            className={cn(
              "app-no-drag glass flex items-center gap-2 rounded-glass-sm px-4 py-2 text-sm font-medium transition-colors outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
              preference === value ? "ring-2 ring-primary" : "text-muted-foreground hover:text-foreground",
            )}
          >
            {t(labelKey)}
          </button>
        ))}
      </div>
    </GlassPanel>
  );
}
