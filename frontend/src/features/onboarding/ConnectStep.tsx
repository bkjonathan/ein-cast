import { CircleCheck, LoaderCircle } from "lucide-react";
import { useRef } from "react";
import { useLogStore } from "@/features/logs";
import { usePlatform } from "@/features/settings";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { firewallNote, firstDevice } from "./wizard";

/**
 * Step 3: open it on a phone or TV. The indicator turns green when the first
 * device asks for the library, which the access log already shows.
 */
export function ConnectStep({ onDone }: { onDone: () => void }) {
  const platform = usePlatform();
  const { t } = useT();
  // Only requests made after this step opened count, so a device that connected
  // earlier does not make it look done before the person has tried.
  const opened = useRef(useLogStore.getState().lines.at(-1)?.seq ?? 0);
  // A string, not the object: a selector that returns a fresh object each time
  // makes the store re-render without end.
  const client = useLogStore((s) => firstDevice(s.lines, opened.current)?.client ?? "");

  return (
    <div className="flex h-full items-center justify-center">
      <GlassPanel className="flex w-full max-w-lg flex-col gap-5 p-10">
        <div className="space-y-1.5 text-center">
          <h2 className="text-xl font-semibold">{t("wizard.connect.title")}</h2>
          <p className="text-sm text-muted-foreground">{t("wizard.connect.intro")}</p>
        </div>

        <ul className="space-y-2 text-sm">
          <li>
            <span className="font-medium">{t("wizard.connect.phone")}</span>{" "}
            <span className="text-muted-foreground">{t("wizard.connect.phoneText")}</span>
          </li>
          <li>
            <span className="font-medium">{t("wizard.connect.tv")}</span>{" "}
            <span className="text-muted-foreground">{t("wizard.connect.tvText")}</span>
          </li>
          <li>
            <span className="font-medium">{t("wizard.connect.vlc")}</span>{" "}
            <span className="text-muted-foreground">{t("wizard.connect.vlcText")}</span>
          </li>
        </ul>

        <p role="status" aria-live="polite" className="glass flex items-center gap-2 rounded-glass-sm px-4 py-3 text-sm">
          {client ? (
            <>
              <CircleCheck className="size-4 shrink-0 text-success" aria-hidden />
              <span>
                {t("wizard.connect.connected")} <span className="font-mono text-muted-foreground">({client})</span>.{" "}
                {t("wizard.connect.allSet")}
              </span>
            </>
          ) : (
            <>
              <LoaderCircle className="size-4 shrink-0 animate-spin text-muted-foreground" aria-hidden />
              <span className="text-muted-foreground">{t("wizard.connect.waiting")}</span>
            </>
          )}
        </p>

        {!client && <p className="text-xs text-muted-foreground">{t(firewallNote(platform))}</p>}

        <div className="flex justify-center">
          <GlassButton size="lg" onClick={onDone}>
            {client ? t("wizard.connect.finish") : t("wizard.connect.finishNow")}
          </GlassButton>
        </div>
      </GlassPanel>
    </div>
  );
}
