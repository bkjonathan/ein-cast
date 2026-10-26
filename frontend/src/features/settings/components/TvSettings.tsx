import { useState } from "react";
import { ShieldAlert, Tv } from "lucide-react";
import { useServerStore } from "@/features/server";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Switch } from "@/shared/ui/Switch";
import { inputFrom, smartTvsInsteadOfToken, tvNeedsRestart } from "../dirty";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";

/**
 * Smart TV (DLNA/UPnP) discovery. Off by default. A TV cannot send the access
 * token, so Go allows only one of the two; when the token is on, this screen
 * explains the trade and lets the person make it in one step (Go enforces it).
 */
export function TvSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const running = useServerStore((s) => s.status);
  const { save, pending, error } = useSettingsCommands();
  const [choosing, setChoosing] = useState(false);
  if (!saved) return null;

  const on = saved.dlna;
  const tradesToken = !on && saved.requireToken;

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
        <Switch
          checked={on}
          label="Let smart TVs find EinCast"
          disabled={pending}
          onChange={(next) => {
            if (next && tradesToken) return setChoosing(true);
            void save(inputFrom(saved, { dlna: next }));
          }}
        />
      </header>

      {tradesToken && !choosing && (
        <p className="text-sm text-muted-foreground">
          The access token is on, and TVs can't send it. Turning this on turns the token off.
        </p>
      )}

      {choosing && (
        <div role="alertdialog" aria-label="Turn off the access token?" className="glass space-y-3 rounded-glass-sm p-4">
          <p className="flex items-start gap-2 text-sm text-warning">
            <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
            Smart TVs can't send the access token, so EinCast would list your library to everyone on this network,
            without a password. Keep the token if you share your Wi-Fi with people you don't trust, and give the TV
            the address instead.
          </p>
          <div className="flex flex-wrap gap-2">
            <GlassButton
              size="sm"
              disabled={pending}
              onClick={async () => {
                if (await save(smartTvsInsteadOfToken(saved))) setChoosing(false);
              }}
            >
              Turn off the token and enable Smart TVs
            </GlassButton>
            <GlassButton size="sm" onClick={() => setChoosing(false)}>
              Keep the token
            </GlassButton>
          </div>
        </div>
      )}

      {(on || !tradesToken) && (
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
