import { useState } from "react";
import { WifiOff } from "lucide-react";
import { useServerStore } from "@/features/server";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";
import { hostOf, playlistUrl } from "../urls";
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

  const url = lanUrls[Math.min(selected, lanUrls.length - 1)];
  if (!url) return <NoNetwork localUrl={localUrl} />;

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
                aria-pressed={u === url}
                onClick={() => setSelected(i)}
                className={cn(
                  "app-no-drag rounded-full px-3 py-1 font-mono text-xs transition-colors",
                  u === url
                    ? "bg-(--glass-surface-hover) text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {hostOf(u)}
              </button>
            ))}
          </div>
        )}

        <UrlRow label="Web player" url={url} />
        <UrlRow label="Playlist for VLC / Infuse" url={playlistUrl(url)} />
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
