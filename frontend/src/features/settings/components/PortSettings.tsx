import { useEffect, useRef, useState } from "react";
import { CircleCheck, OctagonX, Search } from "lucide-react";
import type { PortHolder } from "@/shared/bridge";
import { useT } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import { parsePort } from "../dirty";
import { usePortTools } from "../hooks/usePortTools";
import { useSettingsStore } from "../store";

/**
 * For when the server cannot start because another program already holds its
 * port. It first shows which program that is, and ends it only on a second,
 * explicit click: killing a process is not something to do from a guess.
 */
export function PortSettings({ checkPort }: { checkPort?: number }) {
  const saved = useSettingsStore((s) => s.settings);
  if (!saved) return null;
  return <PortTools initialPort={checkPort ?? saved.port} lookNow={checkPort !== undefined} />;
}

/**
 * `lookNow` is set when the Dashboard sent the person here because the server
 * could not bind: the answer to "who has it?" is then shown without another click.
 */
function PortTools({ initialPort, lookNow }: { initialPort: number; lookNow: boolean }) {
  const [text, setText] = useState(String(initialPort));
  const panel = useRef<HTMLDivElement>(null);
  const [confirming, setConfirming] = useState<number | null>(null);
  const { holders, checkedPort, freed, pending, error, check, free, reset } = usePortTools();
  const { t } = useT();
  const port = parsePort(text);

  useEffect(() => {
    if (!lookNow) return;
    panel.current?.scrollIntoView?.({ block: "start" });
    void check(initialPort);
    // Once, when the screen opens. `check` is stable, and a later edit of the
    // text must not search again by itself.
  }, []);

  const onCheck = () => {
    setConfirming(null);
    void check(port);
  };

  return (
    <GlassPanel ref={panel} className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <OctagonX className="size-5" aria-hidden /> {t("settings.port.title")}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t("settings.port.intro")}
        </p>
      </header>

      <form
        className="flex items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          onCheck();
        }}
      >
        <label className="w-36 space-y-1.5">
          <span className="text-xs font-medium text-muted-foreground">{t("settings.server.port")}</span>
          <Input
            value={text}
            onChange={(e) => {
              setText(e.target.value);
              setConfirming(null);
              reset(); // the old answer is about a different port now
            }}
            inputMode="numeric"
            autoComplete="off"
            className="font-mono"
            aria-label={t("settings.port.toCheck")}
          />
        </label>
        <GlassButton type="submit" disabled={pending}>
          <Search /> {t("settings.port.check")}
        </GlassButton>
      </form>

      {holders !== null && holders.length === 0 && (
        <p role="status" className="flex items-center gap-2 text-sm text-success">
          <CircleCheck className="size-4 shrink-0" aria-hidden />
          {freed
            ? t("settings.port.freed", { who: typeof freed.who === "number" ? t("settings.port.process", { pid: freed.who }) : freed.who, port: freed.port })
            : t("settings.port.free", { port: checkedPort ?? 0 })}
        </p>
      )}

      {holders !== null && holders.length > 0 && (
        <ul className="flex flex-col gap-2">
          {holders.map((h) => (
            <HolderRow
              key={h.pid}
              holder={h}
              confirming={confirming === h.pid}
              pending={pending}
              onAsk={() => setConfirming(h.pid)}
              onCancel={() => setConfirming(null)}
              onEnd={async () => {
                await free(checkedPort ?? port, h);
                setConfirming(null);
              }}
            />
          ))}
        </ul>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </GlassPanel>
  );
}

interface HolderRowProps {
  holder: PortHolder;
  confirming: boolean;
  pending: boolean;
  onAsk: () => void;
  onCancel: () => void;
  onEnd: () => void;
}

function HolderRow({ holder, confirming, pending, onAsk, onCancel, onEnd }: HolderRowProps) {
  const { t } = useT();
  const label = holder.name || t("settings.port.unknown");
  return (
    <li className="glass flex flex-wrap items-center justify-between gap-3 rounded-glass-sm px-3 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">
          {label} <span className="font-mono text-xs text-muted-foreground">{t("settings.port.pid", { pid: holder.pid })}</span>
        </p>
        {holder.self && (
          <p className="text-xs text-muted-foreground">{t("settings.port.self")}</p>
        )}
        {confirming && (
          <p className="text-xs text-warning">{t("settings.port.unsaved")}</p>
        )}
      </div>

      {!holder.self &&
        (confirming ? (
          <div className="flex gap-2">
            <GlassButton size="sm" disabled={pending} onClick={onEnd}>
              <OctagonX /> {t("settings.port.end", { name: label })}
            </GlassButton>
            <GlassButton size="sm" disabled={pending} onClick={onCancel}>
              {t("common.cancel")}
            </GlassButton>
          </div>
        ) : (
          <GlassButton size="sm" disabled={pending} onClick={onAsk}>
            {t("settings.port.endAsk")}
          </GlassButton>
        ))}
    </li>
  );
}
