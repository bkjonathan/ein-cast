import { useState } from "react";
import { Globe, Info, LoaderCircle, Power, PowerOff, ShieldAlert } from "lucide-react";
import { useServerStore } from "@/features/server";
import { useSettingsStore } from "@/features/settings";
import { cn } from "@/shared/lib/cn";
import { GlassButton, GlassPanel, GlassPill } from "@/shared/ui/glass";
import { enableBlocker, remoteActions, summarize } from "../describe";
import { useNotice } from "../hooks/useNotice";
import { useRemoteCommands } from "../hooks/useRemoteCommands";
import { defaultProvider, describeProvider } from "../providers";
import { useRemoteStore } from "../store";
import { ProviderSetup } from "./ProviderSetup";

/** Choose how to reach the library from outside, and turn it on or off. */
export function ProviderPanel() {
  const status = useRemoteStore((s) => s.status);
  const providers = useRemoteStore((s) => s.providers);
  const serverRunning = useServerStore((s) => s.status?.state === "running");
  const port = useSettingsStore((s) => s.settings?.port);
  const [picked, setPicked] = useState<string | null>(null);
  const { enable, disable, busy, error } = useRemoteCommands();
  const selectedId = defaultProvider(providers, status?.provider ?? "", picked);
  const notice = useNotice(selectedId);

  if (!status) return null;

  const { canEnable, canDisable } = remoteActions(status);
  const selected = providers.find((p) => p.id === selectedId);
  const noticeText = selected ? describeProvider(selected).notice : null;
  const noticePending = noticeText !== null && !notice.acknowledged;
  const blocker = canEnable ? enableBlocker(selected, status, serverRunning, noticePending) : null;
  const { label, tone } = summarize(status);
  const shownError = status.error || error;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Globe className="size-5" aria-hidden /> Remote access
          </h2>
          <p className="text-sm text-muted-foreground">
            Watch your library away from home. Pick how devices outside this network connect.
          </p>
        </div>
        <GlassPill tone={tone} role="status" aria-live="polite">
          {label}
        </GlassPill>
      </header>

      {providers.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          No remote access options are available in this version of EinCast.
        </p>
      ) : (
        <div role="radiogroup" aria-label="How to connect" className="flex flex-col gap-2">
          {providers.map((p) => {
            const copy = describeProvider(p);
            const checked = p.id === selectedId;
            return (
              <button
                key={p.id}
                type="button"
                role="radio"
                aria-checked={checked}
                // The choice is fixed while a tunnel is on or coming up.
                disabled={!canEnable}
                onClick={() => setPicked(p.id)}
                className={cn(
                  "app-no-drag glass rounded-glass-sm p-4 text-left transition-colors disabled:cursor-default",
                  checked ? "ring-2 ring-primary" : "opacity-80 hover:opacity-100",
                  !checked && !canEnable && "opacity-40",
                )}
              >
                <span className="flex flex-wrap items-center gap-2">
                  <span className="font-medium">{copy.label}</span>
                  <span
                    className={cn(
                      "rounded-full px-2 py-0.5 text-xs",
                      p.public ? "bg-warning/20 text-warning" : "bg-success/20 text-success",
                    )}
                  >
                    {p.public ? "Public" : "Private"}
                  </span>
                  {status.needsSetup.includes(p.id) && (
                    <span className="rounded-full bg-(--glass-surface-hover) px-2 py-0.5 text-xs text-muted-foreground">
                      Needs setup
                    </span>
                  )}
                  {copy.recommended && (
                    <span className="rounded-full bg-primary/20 px-2 py-0.5 text-xs text-primary">
                      Recommended
                    </span>
                  )}
                </span>
                <span className="mt-1 block text-sm text-muted-foreground">{copy.summary}</span>
              </button>
            );
          })}
        </div>
      )}

      {selected && selected.fields.length > 0 && (
        <ProviderSetup
          provider={selected}
          // In use means on, or on and failing to stop: its settings are frozen.
          inUse={status.provider === selected.id && status.state !== "idle"}
          port={port}
          refreshOn={status.needsSetup.join(",") + status.state}
        />
      )}

      {canEnable && selected?.public && (
        <p className="flex items-start gap-2 text-sm text-warning">
          <ShieldAlert className="mt-0.5 size-4 shrink-0" aria-hidden />
          Anyone on the Internet who learns the address can reach its sign-in page. Only your PIN
          stands between them and your library.
        </p>
      )}

      {canEnable && noticeText && !notice.acknowledged && (
        <div role="note" className="glass flex flex-col gap-3 rounded-glass-sm p-4">
          <p className="flex items-start gap-2 text-sm">
            <Info className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden />
            {noticeText}
          </p>
          <GlassButton size="sm" className="self-start" onClick={notice.acknowledge}>
            I understand
          </GlassButton>
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        Playing away from home is limited by your home internet's upload speed. A film needs a few
        megabits per second to your viewer, all the time it plays.
      </p>

      {status.state === "running" && status.url && (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">Address</p>
          <code className="glass block truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text">
            {status.url}
          </code>
          <p className="text-xs text-muted-foreground">
            The Dashboard shows a QR code for it under “Connect a device”.
            {status.public && status.sessions > 0 && ` ${status.sessions} signed in.`}
          </p>
        </div>
      )}

      {shownError && (
        <p role="alert" className="text-sm text-destructive select-text">
          {shownError}
        </p>
      )}

      <div className="flex flex-wrap items-center gap-3">
        {canDisable ? (
          <GlassButton onClick={disable} disabled={busy.disable}>
            {busy.disable || status.state === "stopping" ? (
              <LoaderCircle className="animate-spin" />
            ) : (
              <PowerOff />
            )}
            {status.state === "starting" ? "Cancel" : status.state === "failed" ? "Try turning off again" : "Turn off"}
          </GlassButton>
        ) : (
          <GlassButton
            onClick={() => void enable(selectedId)}
            disabled={!canEnable || blocker !== null || busy.enable}
          >
            {busy.enable || status.state === "starting" ? <LoaderCircle className="animate-spin" /> : <Power />}
            {status.state === "failed" ? "Try again" : "Turn on"}
          </GlassButton>
        )}
        {blocker && <p className="text-sm text-muted-foreground">{blocker}</p>}
      </div>
    </GlassPanel>
  );
}
