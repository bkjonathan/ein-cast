import { Check, CircleCheck, Clapperboard, Copy, FolderSearch, RefreshCw } from "lucide-react";
import { useCopy } from "@/shared/hooks/useCopy";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
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
  if (saved === null || chosen === null || !status) return null;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Clapperboard className="size-5" aria-hidden /> ffmpeg
        </h2>
        <p className="text-sm text-muted-foreground">
          Optional. Gives your library thumbnails and lets iPhones, iPads and TVs play MKV and HEVC videos.
        </p>
      </header>

      {status.available ? (
        <p role="status" className="flex items-start gap-2 text-sm text-success">
          <CircleCheck className="mt-0.5 size-4 shrink-0" aria-hidden />
          <span className="min-w-0">
            Found at <span className="font-mono break-all text-foreground select-text">{status.path}</span>
          </span>
        </p>
      ) : (
        <div className="space-y-3">
          <p role="status" className="text-sm text-warning">
            {status.chosen ? "The ffmpeg you chose can't be used. Pick another, or let EinCast look for it." : "ffmpeg wasn't found. Without it, videos show an icon instead of a picture, and files an iPhone can't play won't play."}
          </p>
          {status.installCommand && (
            <div className="space-y-1.5">
              <p className="text-xs font-medium text-muted-foreground">Install it by pasting this in a terminal:</p>
              <div className="flex items-center gap-2">
                <code className="glass min-w-0 flex-1 truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text">
                  {status.installCommand}
                </code>
                <GlassButton size="sm" onClick={() => copy(status.installCommand)}>
                  {copied === status.installCommand ? <Check /> : <Copy />}
                  {copied === status.installCommand ? "Copied" : "Copy"}
                </GlassButton>
              </div>
              <p className="text-xs text-muted-foreground">Then choose Check again. No restart needed.</p>
            </div>
          )}
        </div>
      )}

      {status.available && (
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-0.5">
            <p className="text-sm font-medium">{hardwareEncodingRow.title}</p>
            <p className="text-xs text-muted-foreground">{hardwareEncodingRow.help}</p>
          </div>
          <Switch
            checked={saved.hardwareEncoding}
            label={hardwareEncodingRow.title}
            disabled={settingsCommands.pending}
            onChange={(next) => void settingsCommands.save(inputFrom(saved, { hardwareEncoding: next }))}
          />
        </div>
      )}

      {status.chosen && (
        <p className="text-xs text-muted-foreground">
          Your choice: <span className="font-mono break-all select-text">{status.chosen}</span>
        </p>
      )}

      <div className="flex flex-wrap gap-2">
        <GlassButton size="sm" onClick={() => void locate()} disabled={pending}>
          <FolderSearch /> Locate ffmpeg…
        </GlassButton>
        <GlassButton size="sm" onClick={() => void recheck()} disabled={pending}>
          <RefreshCw /> Check again
        </GlassButton>
        {status.chosen && (
          <GlassButton size="sm" onClick={() => void useAutomatic()} disabled={pending}>
            Let EinCast look for it
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
