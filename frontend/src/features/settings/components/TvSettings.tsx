import { Tv } from "lucide-react";
import { useServerStore } from "@/features/server";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";
import { tvNeedsRestart } from "../dirty";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";

/**
 * Smart TV (DLNA/UPnP) discovery. Off by default, and unavailable while the
 * access token is on: a TV cannot send a token, so the two exclude each other
 * (Go enforces it; this screen just explains it).
 */
export function TvSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const running = useServerStore((s) => s.status);
  const { save, pending, error } = useSettingsCommands();
  if (!saved) return null;

  const on = saved.dlna;
  const blocked = !on && saved.requireToken;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Tv className="size-5" aria-hidden /> Smart TVs
          </h2>
          <p className="text-sm text-muted-foreground">
            {on
              ? "Smart TVs and media players on your network find EinCast by themselves and can browse your library."
              : "Off: TVs have to be given the address, or play the playlist from VLC or Infuse."}
          </p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={on}
          aria-label="Let smart TVs find EinCast"
          disabled={pending || blocked}
          onClick={() =>
            save({ port: saved.port, name: saved.name, requireToken: saved.requireToken, dlna: !on })
          }
          className={cn(
            "app-no-drag relative h-7 w-12 shrink-0 rounded-full transition-colors disabled:opacity-50",
            on ? "bg-primary" : "bg-(--glass-surface-hover)",
          )}
        >
          <span
            className={cn(
              "absolute top-1 left-1 size-5 rounded-full bg-white shadow transition-transform",
              on && "translate-x-5",
            )}
          />
        </button>
      </header>

      {blocked && (
        <p className="text-sm text-muted-foreground">
          The access token is on, and TVs can't send it. Turn the token off above to use this.
        </p>
      )}
      {(on || !blocked) && (
        <p className="text-xs text-muted-foreground">
          This lists your library to everyone on the same network, without a password. Use it on a home network you trust.
        </p>
      )}
      {tvNeedsRestart(running, saved) && (
        <p className="text-sm text-warning">Saved. Stop and start the server for this to take effect.</p>
      )}
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
