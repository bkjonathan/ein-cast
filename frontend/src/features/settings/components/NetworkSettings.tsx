import { useState } from "react";
import { LoaderCircle, RefreshCw, Wifi } from "lucide-react";
import { useServerStore } from "@/features/server";
import { refreshAddresses } from "@/shared/bridge";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { networkHints } from "../network";

/**
 * For when a device can't find the server: take on the computer's new addresses
 * after the Wi-Fi changed, and the usual reasons a network hides a server.
 */
export function NetworkSettings() {
  const running = useServerStore((s) => s.status?.state === "running");
  const { t } = useT();
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
          <Wifi className="size-5" aria-hidden /> {t("settings.network.title")}
        </h2>
        <p className="text-sm text-muted-foreground">{t("settings.network.intro")}</p>
      </header>

      <div className="space-y-2">
        <GlassButton size="sm" onClick={() => void refresh()} disabled={!running || pending}>
          {pending ? <LoaderCircle className="animate-spin" /> : <RefreshCw />} {t("settings.network.refresh")}
        </GlassButton>
        <p className="text-xs text-muted-foreground">
          {running
            ? t("settings.network.refreshHelp")
            : t("settings.network.refreshUnavailable")}
        </p>
      </div>

      <ul className="flex flex-col gap-2">
        {networkHints.map((h) => (
          <li key={h.id} className="text-sm">
            <span className="font-medium">{t(h.titleKey)}</span>
            <span className="text-muted-foreground"> {t(h.textKey)}</span>
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
