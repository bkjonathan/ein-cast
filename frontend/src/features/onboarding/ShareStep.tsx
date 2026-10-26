import { LoaderCircle, Play, WifiOff } from "lucide-react";
import { connectSources, QrCode, UrlRow, withToken } from "@/features/connect";
import { useServerControls, useServerStore } from "@/features/server";
import { useSettingsStore } from "@/features/settings";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";

/** Step 2: the server runs, and here is the code that opens it. */
export function ShareStep({ onNext }: { onNext: () => void }) {
  const status = useServerStore((s) => s.status);
  const token = useSettingsStore((s) => (s.settings?.requireToken ? s.settings.accessToken : ""));
  const { start, pending, commandError } = useServerControls();
  if (!status) return null;

  const running = status.state === "running";
  const busy = pending || status.state === "starting";
  const source = running ? connectSources(status, null, token, "")[0] : undefined;
  const url = source ? withToken(source.base, source.token) : "";
  const error = status.error || commandError;

  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-lg flex-col items-center gap-5 p-10 text-center">
        <div className="space-y-1.5">
          <h2 className="text-xl font-semibold">{running ? "You're sharing" : "Start sharing"}</h2>
          <p className="text-sm text-muted-foreground">
            {running
              ? "Anything on your Wi-Fi with this code or address can open your library."
              : "EinCast serves your folder to the screens on your Wi-Fi. Nothing leaves your home."}
          </p>
        </div>

        {running && source && (
          <>
            <QrCode value={url} />
            <div className="w-full text-left">
              <UrlRow label="Address" url={url} />
            </div>
            {source.token && (
              <p className="text-xs text-muted-foreground">
                The code includes your access token, so only devices you show it to can open the library.
              </p>
            )}
          </>
        )}

        {running && !source && (
          <p role="status" className="flex items-center gap-2 text-sm text-warning">
            <WifiOff className="size-4 shrink-0" aria-hidden />
            No local network found. Connect to Wi-Fi or plug in a cable, then come back to Settings → Network.
          </p>
        )}

        {error && !running && (
          <p role="alert" className="text-sm text-destructive select-text">
            {error}
          </p>
        )}

        {running ? (
          <GlassButton size="lg" onClick={onNext}>
            Next
          </GlassButton>
        ) : (
          <GlassButton size="lg" onClick={() => void start()} disabled={busy}>
            {busy ? <LoaderCircle className="animate-spin" /> : <Play />}
            {status.state === "failed" ? "Try again" : "Start sharing"}
          </GlassButton>
        )}
      </GlassPanel>
    </div>
  );
}
