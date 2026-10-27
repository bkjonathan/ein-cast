import { Check, CircleCheck, Clapperboard, Copy, FolderSearch, RefreshCw } from "lucide-react";
import { useCopy } from "@/shared/hooks/useCopy";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { useT } from "@/shared/i18n";
import { Switch } from "@/shared/ui/Switch";
import { inputFrom } from "../dirty";
import { useFFmpeg } from "../hooks/useFFmpeg";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";
import { hardwareEncodingRow } from "../conversion";

/**
 * ffmpeg makes the pictures in the library, converts videos that iPhones, iPads
 * and browsers can't play (MKV, HEVC) and reads the audio and subtitle tracks
 * inside them. EinCast doesn't ship it, so this says whether it
 * was found and, if not, what it would unlock and how to get it.
 */
export function FFmpegSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const chosen = saved?.ffmpegPath ?? null;
  const { status, pending, error, recheck, locate, useAutomatic } = useFFmpeg(chosen ?? "");
  const { copied, copy } = useCopy();
  const settingsCommands = useSettingsCommands();
  const { t } = useT();
  if (saved === null || chosen === null || !status) return null;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Clapperboard className="size-5" aria-hidden /> ffmpeg
        </h2>
        <p className="text-sm text-muted-foreground">
          {t("settings.ffmpeg.intro")}
        </p>
      </header>

      {status.available ? (
        <p role="status" className="flex items-start gap-2 text-sm text-success">
          <CircleCheck className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span className="min-w-0">
            {t("settings.ffmpeg.foundAt")} <span className="font-mono break-all text-foreground select-text">{status.path}</span>
          </span>
        </p>
      ) : (
        <div className="space-y-3">
          <p role="status" className="text-sm text-warning">
            {status.chosen ? t("settings.ffmpeg.badChoice") : t("settings.ffmpeg.missing")}
          </p>
          {status.installCommand && (
            <div className="space-y-1.5">
              <p className="text-xs font-medium text-muted-foreground">{t("settings.ffmpeg.installHint")}</p>
              <div className="flex items-center gap-2">
                <code className="glass min-w-0 flex-1 truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text">
                  {status.installCommand}
                </code>
                <GlassButton size="sm" onClick={() => copy(status.installCommand)}>
                  {copied === status.installCommand ? <Check /> : <Copy />}
                  {copied === status.installCommand ? t("common.copied") : t("common.copy")}
                </GlassButton>
              </div>
              <p className="text-xs text-muted-foreground">{t("settings.ffmpeg.installAfter")}</p>
            </div>
          )}
        </div>
      )}

      {status.available && (
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-0.5">
            <p className="text-sm font-medium">{t(hardwareEncodingRow.titleKey)}</p>
            <p className="text-xs text-muted-foreground">{t(hardwareEncodingRow.helpKey)}</p>
          </div>
          <Switch
            checked={saved.hardwareEncoding}
            label={t(hardwareEncodingRow.titleKey)}
            disabled={settingsCommands.pending}
            onChange={(next) => void settingsCommands.save(inputFrom(saved, { hardwareEncoding: next }))}
          />
        </div>
      )}

      {status.chosen && (
        <p className="text-xs text-muted-foreground">
          {t("settings.ffmpeg.yourChoice")} <span className="font-mono break-all select-text">{status.chosen}</span>
        </p>
      )}

      <div className="flex flex-wrap gap-2">
        <GlassButton size="sm" onClick={() => void locate()} disabled={pending}>
          <FolderSearch /> {t("settings.ffmpeg.locate")}
        </GlassButton>
        <GlassButton size="sm" onClick={() => void recheck()} disabled={pending}>
          <RefreshCw /> {t("settings.ffmpeg.recheck")}
        </GlassButton>
        {status.chosen && (
          <GlassButton size="sm" onClick={() => void useAutomatic()} disabled={pending}>
            {t("settings.ffmpeg.automatic")}
          </GlassButton>
        )}
      </div>

      {(error ?? settingsCommands.error) && (
        <p role="alert" className="text-sm text-destructive">
          {error ?? settingsCommands.error}
        </p>
      )}
    </GlassPanel>
  );
}
