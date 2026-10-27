import { useEffect, useRef } from "react";
import { Check, CircleCheck, Copy, Download, ExternalLink, RefreshCw } from "lucide-react";
import { useUpdateCommands, useUpdatesStore } from "@/features/updates";
import { useCopy } from "@/shared/hooks/useCopy";
import { useT } from "@/shared/i18n";
import { formatBytes, formatDate, formatDateTime } from "@/shared/lib/format";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Switch } from "@/shared/ui/Switch";
import { inputFrom } from "../dirty";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";

/** What Go reports for a build made from source: it has no version to compare. */
const DEV_BUILD = "dev";

/**
 * Whether a newer EinCast exists. EinCast only tells: the person downloads and
 * installs it from the release page, and can check the download against the
 * published SHA-256. The automatic check is a setting; the button always works.
 */
export function UpdateSettings({ focus = 0 }: { focus?: number }) {
  const saved = useSettingsStore((s) => s.settings);
  const status = useUpdatesStore((s) => s.status);
  const panel = useRef<HTMLDivElement>(null);
  const shown = Boolean(saved && status);
  // The sidebar notice sends the person here (each click raises `focus`): bring the card into view once it exists.
  useEffect(() => {
    if (focus > 0 && shown) panel.current?.scrollIntoView?.({ block: "start" });
  }, [focus, shown]);
  const { check, openPage, error: commandError } = useUpdateCommands();
  const settingsCommands = useSettingsCommands();
  const { copied, copy } = useCopy();
  const { t, locale } = useT();
  if (!saved || !status) return null;

  const isDev = status.current === DEV_BUILD;
  const latest = status.latest;
  // Go's reason is English; for a build from source the card says it in the window's language.
  const failure = isDev ? "" : status.error;
  const error = failure || commandError;

  return (
    <GlassPanel ref={panel} className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Download className="size-5" aria-hidden /> {t("updates.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("updates.intro")}</p>
      </header>

      <p className="text-sm">{isDev ? t("updates.currentDev") : t("updates.current", { version: status.current })}</p>

      <div className="flex items-start justify-between gap-4">
        <div className="space-y-0.5">
          <p className="text-sm font-medium">{t("updates.auto.title")}</p>
          <p className="text-xs text-muted-foreground">{t("updates.auto.help")}</p>
        </div>
        <Switch
          checked={saved.checkForUpdates}
          label={t("updates.auto.title")}
          disabled={settingsCommands.pending}
          onChange={(next) => void settingsCommands.save(inputFrom(saved, { checkForUpdates: next }))}
        />
      </div>

      {status.available && latest ? (
        <section aria-label={t("updates.available", { version: latest.version })} className="glass flex flex-col gap-3 rounded-glass-sm p-4">
          <div className="space-y-0.5">
            <p role="status" className="text-sm font-semibold text-success">
              {t("updates.available", { version: latest.version })}
            </p>
            {latest.publishedAt > 0 && (
              <p className="text-xs text-muted-foreground">{t("updates.published", { date: formatDate(latest.publishedAt, locale) })}</p>
            )}
          </div>

          {latest.notes && (
            <div className="space-y-1">
              <p className="text-xs font-medium text-muted-foreground">{t("updates.notes")}</p>
              {/* Text from the network: shown as text, never as markup. */}
              <pre className="max-h-48 overflow-y-auto font-sans text-sm whitespace-pre-wrap select-text">{latest.notes}</pre>
            </div>
          )}

          {latest.assets.length > 0 && (
            <div className="space-y-2">
              <p className="text-xs font-medium text-muted-foreground">{t("updates.files")}</p>
              <ul className="flex flex-col gap-3">
                {latest.assets.map((a) => (
                  <li key={a.name} className="space-y-1">
                    <p className="flex items-baseline justify-between gap-3 text-sm">
                      <span className="min-w-0 font-medium break-all select-text">{a.name}</span>
                      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{formatBytes(a.size, locale)}</span>
                    </p>
                    {a.sha256 ? (
                      <div className="flex items-start gap-2">
                        <code className="min-w-0 flex-1 font-mono text-xs break-all text-muted-foreground select-text">SHA-256 {a.sha256}</code>
                        <GlassButton size="sm" aria-label={t("updates.copyAria", { name: a.name })} onClick={() => void copy(a.sha256)}>
                          {copied === a.sha256 ? <Check /> : <Copy />}
                          {copied === a.sha256 ? t("common.copied") : t("common.copy")}
                        </GlassButton>
                      </div>
                    ) : (
                      <p className="text-xs text-muted-foreground">{t("updates.noChecksum")}</p>
                    )}
                  </li>
                ))}
              </ul>
              <p className="text-xs text-muted-foreground">{t("updates.verify")}</p>
            </div>
          )}

          <div>
            <GlassButton size="sm" onClick={() => void openPage()}>
              <ExternalLink /> {t("updates.openPage")}
            </GlassButton>
          </div>
        </section>
      ) : (
        <p role="status" className="flex items-center gap-2 text-sm text-muted-foreground">
          {status.checking ? (
            t("updates.checking")
          ) : status.checkedAt > 0 ? (
            <>
              <CircleCheck className="size-4 shrink-0 text-success" aria-hidden />
              <span>
                {t("updates.upToDate")} {t("updates.checkedAt", { when: formatDateTime(status.checkedAt, locale) })}
              </span>
            </>
          ) : (
            t("updates.neverChecked")
          )}
        </p>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}

      <div className="flex flex-wrap gap-2">
        <GlassButton size="sm" onClick={() => void check()} disabled={status.checking}>
          <RefreshCw className={status.checking ? "animate-spin" : undefined} /> {status.checking ? t("updates.checking") : t("updates.checkNow")}
        </GlassButton>
      </div>

      {settingsCommands.error && (
        <p role="alert" className="text-sm text-destructive">
          {settingsCommands.error}
        </p>
      )}
    </GlassPanel>
  );
}
