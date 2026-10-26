import { useState } from "react";
import { WifiOff } from "lucide-react";
import { describeProvider, useRemoteStore } from "@/features/remote";
import { useServerStore } from "@/features/server";
import { useSettingsStore } from "@/features/settings";
import { cn } from "@/shared/lib/cn";
import { GlassPanel } from "@/shared/ui/glass";
import { connectSources, type ConnectSource, type SourceKind } from "../sources";
import { playlistUrl, withToken } from "../urls";
import { QrCode } from "./QrCode";
import { UrlRow } from "./UrlRow";

/**
 * How to open the library from another device: over the home network, and, while
 * remote access is on, from outside it. Visible only while the server runs.
 */
export function ConnectCard() {
  const status = useServerStore((s) => s.status);
  const remote = useRemoteStore((s) => s.status);
  // The token rides in the link so a scan signs the device in. Go keeps it out of
  // the server status on purpose (it can change while the server runs).
  const lanToken = useSettingsStore((s) => (s.settings?.requireToken ? s.settings.accessToken : ""));
  if (!status || status.state !== "running") return null;

  const label = remote ? describeProvider({ id: remote.provider, public: remote.public }).label : "";
  const sources = connectSources(status, remote, lanToken, label);
  if (sources.length === 0) return <NoNetwork localUrl={status.localUrl} />;
  return <ConnectBody sources={sources} />;
}

const sourceKey = (s: ConnectSource) => `${s.kind}|${s.base}`;

const intro: Record<SourceKind, string> = {
  lan: "On a TV, iPad or phone on the same Wi-Fi, scan the code or open this address.",
  private: "On a device signed in to your account there, scan the code or open this address. It works from anywhere.",
  public: "From any browser, open this address and enter your PIN. The code opens the sign-in page; it does not contain the PIN.",
};

function ConnectBody({ sources }: { sources: ConnectSource[] }) {
  // Chosen by identity, not position: a tunnel appearing or going away must not
  // silently switch which address is shown.
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const source = sources.find((s) => sourceKey(s) === selectedKey) ?? sources[0];
  if (!source) return null; // unreachable: the caller passes at least one

  const url = withToken(source.base, source.token);

  return (
    <GlassPanel className="flex flex-col gap-6 p-6 sm:flex-row">
      <div className="flex min-w-0 flex-1 flex-col gap-4">
        <header>
          <h2 className="text-lg font-semibold">Connect a device</h2>
          <p className="text-sm text-muted-foreground">{intro[source.kind]}</p>
        </header>

        {sources.length > 1 && (
          <div role="group" aria-label="Where from" className="flex flex-wrap gap-1">
            {sources.map((s) => (
              <button
                key={sourceKey(s)}
                type="button"
                aria-pressed={s === source}
                onClick={() => setSelectedKey(sourceKey(s))}
                className={cn(
                  "app-no-drag rounded-full px-3 py-1 text-xs transition-colors",
                  s.kind === "lan" && "font-mono",
                  s === source
                    ? "bg-(--glass-surface-hover) text-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {s.label}
              </button>
            ))}
          </div>
        )}

        {source.token && (
          <p className="text-xs text-muted-foreground">
            These links include your access token. Share them only with people you trust.
          </p>
        )}
        <UrlRow label="Web player" url={url} />
        {source.playlist ? (
          <UrlRow label="Playlist for VLC / Infuse" url={withToken(playlistUrl(source.base), source.token)} />
        ) : (
          <p className="text-xs text-muted-foreground">
            To play in VLC or Infuse, sign in on the player page first, then use “Playlist for VLC / Infuse”
            there. Its links stay valid for a few hours.
          </p>
        )}
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
