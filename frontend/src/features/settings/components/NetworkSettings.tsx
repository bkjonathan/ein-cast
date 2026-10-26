import { useState } from "react";
import { LoaderCircle, RefreshCw, Wifi } from "lucide-react";
import { useServerStore } from "@/features/server";
import { refreshAddresses } from "@/shared/bridge";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { networkHints } from "../network";

/**
 * For when a device can't find the server: take on the computer's new addresses
 * after the Wi-Fi changed, and the usual reasons a network hides a server.
 */
export function NetworkSettings() {
  const running = useServerStore((s) => s.status?.state === "running");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const refresh = async () => {
    setPending(true);
    setError(null);
    try {
      await refreshAddresses();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPending(false);
    }
  };

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Wifi className="size-5" aria-hidden /> Network
        </h2>
        <p className="text-sm text-muted-foreground">If a TV or phone can't see EinCast, start here.</p>
      </header>

      <div className="space-y-2">
        <GlassButton size="sm" onClick={() => void refresh()} disabled={!running || pending}>
          {pending ? <LoaderCircle className="animate-spin" /> : <RefreshCw />} Refresh addresses
        </GlassButton>
        <p className="text-xs text-muted-foreground">
          {running
            ? "Use this after the Wi-Fi or cable changed. It restarts the server, which also turns off remote access."
            : "Available while the server is running."}
        </p>
      </div>

      <ul className="flex flex-col gap-2">
        {networkHints.map((h) => (
          <li key={h.title} className="text-sm">
            <span className="font-medium">{h.title}</span>
            <span className="text-muted-foreground"> {h.text}</span>
          </li>
        ))}
      </ul>

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
