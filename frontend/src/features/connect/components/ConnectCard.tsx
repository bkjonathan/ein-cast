import { useState } from "react";
import { WifiOff } from "lucide-react";
import { useServerStore } from "@/features/server";
import { useSettingsStore } from "@/features/settings";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";
import { hostOf, playlistUrl, withToken } from "../urls";
import { QrCode } from "./QrCode";
import { UrlRow } from "./UrlRow";

/** How to open the library from another device. Visible only while the server runs. */
export function ConnectCard() {
  const status = useServerStore((s) => s.status);
  if (!status || status.state !== "running") return null;
  return <ConnectBody lanUrls={status.lanUrls} localUrl={status.localUrl} />;
}

function ConnectBody({ lanUrls, localUrl }: { lanUrls: string[]; localUrl: string }) {
  const [selected, setSelected] = useState(0);
  // The token rides in the link so a scan signs the device in. Go keeps it out of
  // the server status on purpose (it can change while the server runs).
  const token = useSettingsStore((s) => (s.settings?.requireToken ? s.settings.accessToken : ""));

  const base = lanUrls[Math.min(selected, lanUrls.length - 1)];
  if (!base) return <NoNetwork localUrl={localUrl} />;
  const url = withToken(base, token);

  return (
    <GlassPanel className="flex flex-col gap-6 p-6 sm:flex-row">
      <div className="flex min-w-0 flex-1 flex-col gap-4">
        <header>
          <h2 className="text-lg font-semibold">Connect a device</h2>
          <p className="text-sm text-muted-foreground">
            On a TV, iPad or phone on the same Wi-Fi, scan the code or open this address.
          </p>
        </header>

        {lanUrls.length > 1 && (
          <div role="group" aria-label="Network" className="flex flex-wrap gap-1">
            {lanUrls.map((u, i) => (
              <button
                key={u}
                type="button"
                aria-pressed={u === base}
                onClick={() => setSelected(i)}
                className={cn(
                  "app-no-drag rounded-full px-3 py-1 font-mono text-xs transition-colors",
                  u === base
                    ? "bg-(--glass-surface-hover) text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {hostOf(u)}
              </button>
            ))}
          </div>
        )}

        {token && (
          <p className="text-xs text-muted-foreground">
            These links include your access token. Share them only with people you trust.
          </p>
        )}
        <UrlRow label="Web player" url={url} />
        <UrlRow label="Playlist for VLC / Infuse" url={withToken(playlistUrl(base), token)} />
      </div>
      <QrCode value={url} />
    </GlassPanel>
  );
}

function NoNetwork({ localUrl }: { localUrl: string }) {
  return (
    <GlassPanel className="flex flex-col gap-3 p-6">
      <header className="flex items-center gap-2">
        <WifiOff className="size-5 text-warning" aria-hidden />
        <h2 className="text-lg font-semibold">No local network found</h2>
      </header>
      <p className="text-sm text-muted-foreground">
        Connect this computer to Wi-Fi or Ethernet, then stop and start the server so it can pick
        up the new address. Until then it can only be opened from this computer.
      </p>
      <UrlRow label="This computer only" url={localUrl} />
    </GlassPanel>
  );
}
