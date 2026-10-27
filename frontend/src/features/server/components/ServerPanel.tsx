import { LoaderCircle, OctagonX, Play, Square } from "lucide-react";
import { useSettingsStore } from "@/features/settings";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { allowedActions, isPortInUse } from "../describe";
import { useServerControls } from "../hooks/useServerControls";
import { useServerStore } from "../store";
import { StatusPill } from "./StatusPill";

interface ServerPanelProps {
  /** Opens the "free a port" tool for this port. Without it, the shortcut is not offered. */
  onFreePort?: (port: number) => void;
}

/** Server status with start and stop controls. */
export function ServerPanel({ onFreePort }: ServerPanelProps) {
  const status = useServerStore((s) => s.status);
  const port = useSettingsStore((s) => s.settings?.port ?? 0);
  const { start, stop, pending, commandError } = useServerControls();
  const { t } = useT();

  if (!status) return null;

  const { canStart, canStop } = allowedActions(status.state);
  const transitioning = status.state === "starting" || status.state === "stopping";
  // A failed start already shows its reason from state; show the command error
  // only when it adds something (for example, a refused request).
  const shownError = status.error || commandError;
  const portTaken = onFreePort !== undefined && port > 0 && isPortInUse(shownError ?? "");

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">{t("server.title")}</h2>
          <p className="text-sm text-muted-foreground">{status.name || "EinCast"}</p>
        </div>
        <StatusPill state={status.state} />
      </header>

      {status.state === "running" && (
        <p className="text-sm text-muted-foreground">
          {t("server.serving", { port: status.port })}
        </p>
      )}

      {shownError && (
        <p role="alert" className="text-sm text-destructive select-text">
          {shownError}
        </p>
      )}

      <div className="flex gap-2">
        {canStop || status.state === "stopping" ? (
          <GlassButton onClick={stop} disabled={!canStop || pending}>
            {status.state === "stopping" ? <LoaderCircle className="animate-spin" /> : <Square />}
            {t("server.stop")}
          </GlassButton>
        ) : (
          <GlassButton onClick={start} disabled={!canStart || pending}>
            {transitioning ? <LoaderCircle className="animate-spin" /> : <Play />}
            {status.state === "failed" ? t("server.retry") : t("server.start")}
          </GlassButton>
        )}
        {portTaken && (
          <GlassButton onClick={() => onFreePort(port)}>
            <OctagonX /> {t("server.findPort", { port })}
          </GlassButton>
        )}
      </div>
    </GlassPanel>
  );
}
