import { LoaderCircle, Play, Square } from "lucide-react";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { allowedActions } from "../describe";
import { useServerControls } from "../hooks/useServerControls";
import { useServerStore } from "../store";
import { StatusPill } from "./StatusPill";

/** Server status with start and stop controls. */
export function ServerPanel() {
  const status = useServerStore((s) => s.status);
  const { start, stop, pending, commandError } = useServerControls();

  if (!status) return null;

  const { canStart, canStop } = allowedActions(status.state);
  const transitioning = status.state === "starting" || status.state === "stopping";
  // A failed start already shows its reason from state; show the command error
  // only when it adds something (for example, a refused request).
  const shownError = status.error || commandError;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-center justify-between gap-4">
        <div>
          <h2 className="text-lg font-semibold">Server</h2>
          <p className="text-sm text-muted-foreground">{status.name || "EinCast"}</p>
        </div>
        <StatusPill state={status.state} />
      </header>

      {status.state === "running" && (
        <div className="space-y-1">
          <p className="text-sm text-muted-foreground">Listening on</p>
          <p className="font-mono text-sm select-text">{status.addr}</p>
          <p className="text-xs text-muted-foreground">
            Reachable from this computer only for now. LAN access and the QR code
            arrive with the Connect phase.
          </p>
        </div>
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
            Stop server
          </GlassButton>
        ) : (
          <GlassButton onClick={start} disabled={!canStart || pending}>
            {transitioning ? <LoaderCircle className="animate-spin" /> : <Play />}
            {status.state === "failed" ? "Try again" : "Start server"}
          </GlassButton>
        )}
      </div>
    </GlassPanel>
  );
}
