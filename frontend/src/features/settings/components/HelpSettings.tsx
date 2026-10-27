import { useEffect, useRef, useState } from "react";
import { Bug, Check, ClipboardCopy, Lightbulb, LifeBuoy, ShieldCheck } from "lucide-react";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { useHelp } from "../hooks/useHelp";

/** How long "Copied" stays before the button goes back to its label. */
const COPIED_MS = 4000;

/**
 * Where a person goes when something is wrong or missing: a report to paste, and
 * the project's issue pages. EinCast sends nothing by itself; the diagnostics go to
 * the clipboard and the person decides where they end up.
 */
export function HelpSettings() {
  const { t } = useT();
  const { copyDiagnostics, open, error } = useHelp();
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);

  const onCopy = async () => {
    clearTimeout(timer.current);
    const ok = await copyDiagnostics();
    setCopied(ok);
    if (ok) timer.current = setTimeout(() => setCopied(false), COPIED_MS);
  };

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <LifeBuoy className="size-5" aria-hidden /> {t("settings.help.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("settings.help.intro")}</p>
      </header>

      <div className="flex flex-wrap gap-2">
        <GlassButton size="sm" onClick={() => void onCopy()}>
          {copied ? <Check /> : <ClipboardCopy />} {t("settings.help.copy")}
        </GlassButton>
        <GlassButton size="sm" onClick={() => void open("bug")}>
          <Bug /> {t("settings.help.report")}
        </GlassButton>
        <GlassButton size="sm" onClick={() => void open("idea")}>
          <Lightbulb /> {t("settings.help.idea")}
        </GlassButton>
        <GlassButton size="sm" onClick={() => void open("privacy")}>
          <ShieldCheck /> {t("settings.help.privacy")}
        </GlassButton>
      </div>

      {copied && (
        <p role="status" className="text-sm text-success">
          {t("settings.help.copied")}
        </p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {t("settings.help.failed", { reason: error })}
        </p>
      )}
    </GlassPanel>
  );
}
