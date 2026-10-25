import { useState } from "react";
import { Check, Copy, Eye, EyeOff, KeyRound, ShieldAlert } from "lucide-react";
import { useCopy } from "@/shared/hooks/useCopy";
import { cn } from "@/shared/lib/cn";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { useSettingsCommands } from "../hooks/useSettingsCommands";
import { useSettingsStore } from "../store";

/** Turns the access token on or off, shows it, and replaces it. */
export function AccessSettings() {
  const saved = useSettingsStore((s) => s.settings);
  const { save, rotate, pending, error } = useSettingsCommands();
  const [reveal, setReveal] = useState(false);
  const [confirming, setConfirming] = useState(false);
  const { copied, copy } = useCopy();

  if (!saved) return null;
  const on = saved.requireToken;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="text-lg font-semibold">Access token</h2>
          <p className="text-sm text-muted-foreground">
            {on
              ? "Devices need the link or QR code shown on the Dashboard. Anyone without it is turned away."
              : "Off: anyone on your Wi-Fi who knows the address can open your library."}
          </p>
        </div>
        <button
          type="button"
          role="switch"
          aria-checked={on}
          aria-label="Require an access token"
          disabled={pending}
          onClick={() => save({ port: saved.port, name: saved.name, requireToken: !on })}
          className={cn(
            "app-no-drag relative h-7 w-12 shrink-0 rounded-full transition-colors disabled:opacity-50",
            on ? "bg-primary" : "bg-(--glass-surface-hover)",
          )}
        >
          <span
            className={cn(
              "absolute top-1 left-1 size-5 rounded-full bg-white shadow transition-transform",
              on && "translate-x-5",
            )}
          />
        </button>
      </header>

      {!on && (
        <p className="flex items-center gap-2 text-sm text-warning">
          <ShieldAlert className="size-4 shrink-0" aria-hidden />
          Recommended: turn this on if other people use your network.
        </p>
      )}

      {on && (
        <div className="space-y-3">
          <div className="flex items-center gap-2">
            <code
              className="glass min-w-0 flex-1 truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text"
              aria-label="Access token"
            >
              {reveal ? saved.accessToken : "•".repeat(24)}
            </code>
            <GlassButton
              size="icon"
              onClick={() => setReveal(!reveal)}
              aria-label={reveal ? "Hide token" : "Show token"}
            >
              {reveal ? <EyeOff /> : <Eye />}
            </GlassButton>
            <GlassButton size="sm" onClick={() => copy(saved.accessToken)}>
              {copied === saved.accessToken ? <Check /> : <Copy />}
              {copied === saved.accessToken ? "Copied" : "Copy"}
            </GlassButton>
          </div>

          {!saved.tokenPersisted && (
            <p role="status" className="text-sm text-warning">
              The system keychain isn't available, so this token will change the next time EinCast starts.
            </p>
          )}

          {confirming ? (
            <div className="flex flex-wrap items-center gap-2">
              <p className="text-sm text-muted-foreground">
                Devices using the old link will be locked out.
              </p>
              <GlassButton
                size="sm"
                disabled={pending}
                onClick={async () => {
                  await rotate();
                  setConfirming(false);
                }}
              >
                <KeyRound /> Generate new token
              </GlassButton>
              <GlassButton size="sm" onClick={() => setConfirming(false)}>
                Cancel
              </GlassButton>
            </div>
          ) : (
            <GlassButton size="sm" onClick={() => setConfirming(true)}>
              <KeyRound /> Replace token…
            </GlassButton>
          )}
        </div>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}
