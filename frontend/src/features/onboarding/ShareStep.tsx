import { LoaderCircle, Play, WifiOff } from "lucide-react";
import { connectSources, QrCode, UrlRow, withToken } from "@/features/connect";
import { useServerControls, useServerStore } from "@/features/server";
import { useSettingsStore } from "@/features/settings";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";

/** Step 2: the server runs, and here is the code that opens it. */
export function ShareStep({ onNext }: { onNext: () => void }) {
  const status = useServerStore((s) => s.status);
  const token = useSettingsStore((s) => (s.settings?.requireToken ? s.settings.accessToken : ""));
  const { start, pending, commandError } = useServerControls();
  const { t } = useT();
  if (!status) return null;

  const running = status.state === "running";
  const busy = pending || status.state === "starting";
  const source = running ? connectSources(status, null, token, "")[0] : undefined;
  const url = source ? withToken(source.base, source.token) : "";
  const error = status.error || commandError;

  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-lg flex-col items-center gap-5 p-10 text-center">
        <div className="space-y-1.5">
          <h2 className="text-xl font-semibold">{running ? t("wizard.share.sharing") : t("wizard.share.start")}</h2>
          <p className="text-sm text-muted-foreground">
            {running ? t("wizard.share.running") : t("wizard.share.notRunning")}
          </p>
        </div>

        {running && source && (
          <>
            <QrCode value={url} />
            <div className="w-full text-left">
              <UrlRow label={t("wizard.share.address")} url={url} />
            </div>
            {source.token && (
              <p className="text-xs text-muted-foreground">
                {t("wizard.share.tokenNote")}
              </p>
            )}
          </>
        )}

        {running && !source && (
          <p role="status" className="flex items-center gap-2 text-sm text-warning">
            <WifiOff className="size-4 shrink-0" aria-hidden />
            {t("wizard.share.noNetwork")}
          </p>
        )}

        {error && !running && (
          <p role="alert" className="text-sm text-destructive select-text">
            {error}
          </p>
        )}

        {running ? (
          <GlassButton size="lg" onClick={onNext}>
            {t("wizard.next")}
          </GlassButton>
        ) : (
          <GlassButton size="lg" onClick={() => void start()} disabled={busy}>
            {busy ? <LoaderCircle className="animate-spin" /> : <Play />}
            {status.state === "failed" ? t("server.retry") : t("wizard.share.start")}
          </GlassButton>
        )}
      </GlassPanel>
    </div>
  );
}
