import { useState } from "react";
import { Globe, Info, LoaderCircle, Power, PowerOff, ShieldAlert } from "lucide-react";
import { useServerStore } from "@/features/server";
import { useSettingsStore } from "@/features/settings";
import { useT } from "@/shared/i18n";
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
  const { t } = useT();

  if (!status) return null;

  const { canEnable, canDisable } = remoteActions(status);
  const selected = providers.find((p) => p.id === selectedId);
  const noticeKey = selected ? describeProvider(selected).noticeKey : null;
  const noticeText = noticeKey ? t(noticeKey) : null;
  const noticePending = noticeText !== null && !notice.acknowledged;
  const blockerKey = canEnable ? enableBlocker(selected, status, serverRunning, noticePending) : null;
  const { labelKey, tone } = summarize(status);
  const shownError = status.error || error;

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <h2 className="flex items-center gap-2 text-lg font-semibold">
            <Globe className="size-5" aria-hidden /> {t("remote.title")}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t("remote.intro")}
          </p>
        </div>
        <GlassPill tone={tone} role="status" aria-live="polite">
          {t(labelKey)}
        </GlassPill>
      </header>

      {providers.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t("remote.noProviders")}
        </p>
      ) : (
        <div role="radiogroup" aria-label={t("remote.howToConnect")} className="flex flex-col gap-2">
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
                    {p.public ? t("remote.public") : t("remote.private")}
                  </span>
                  {status.needsSetup.includes(p.id) && (
                    <span className="rounded-full bg-(--glass-surface-hover) px-2 py-0.5 text-xs text-muted-foreground">
                      {t("remote.needsSetup")}
                    </span>
                  )}
                  {copy.recommended && (
                    <span className="rounded-full bg-primary/20 px-2 py-0.5 text-xs text-primary">
                      {t("remote.recommended")}
                    </span>
                  )}
                </span>
                <span className="mt-1 block text-sm text-muted-foreground">{t(copy.summaryKey)}</span>
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
          {t("remote.publicWarning")}
        </p>
      )}

      {canEnable && noticeText && !notice.acknowledged && (
        <div role="note" className="glass flex flex-col gap-3 rounded-glass-sm p-4">
          <p className="flex items-start gap-2 text-sm">
            <Info className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden />
            {noticeText}
          </p>
          <GlassButton size="sm" className="self-start" onClick={notice.acknowledge}>
            {t("remote.understand")}
          </GlassButton>
        </div>
      )}

      <p className="text-xs text-muted-foreground">
        {t("remote.uploadNote")}
      </p>

      {status.state === "running" && status.url && (
        <div className="space-y-1.5">
          <p className="text-xs font-medium text-muted-foreground">{t("remote.address")}</p>
          <code className="glass block truncate rounded-glass-sm px-3 py-2 font-mono text-sm select-text">
            {status.url}
          </code>
          <p className="text-xs text-muted-foreground">
            {t("remote.qrNote")}
            {status.public && status.sessions > 0 && ` ${t("remote.signedIn", { count: status.sessions })}`}
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
            {status.state === "starting" ? t("remote.cancel") : status.state === "failed" ? t("remote.tryOffAgain") : t("remote.turnOff")}
          </GlassButton>
        ) : (
          <GlassButton
            onClick={() => void enable(selectedId)}
            disabled={!canEnable || blockerKey !== null || busy.enable}
          >
            {busy.enable || status.state === "starting" ? <LoaderCircle className="animate-spin" /> : <Power />}
            {status.state === "failed" ? t("server.retry") : t("remote.turnOn")}
          </GlassButton>
        )}
        {blockerKey && <p className="text-sm text-muted-foreground">{t(blockerKey)}</p>}
      </div>
    </GlassPanel>
  );
}
