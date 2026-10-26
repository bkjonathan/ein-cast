import { useState } from "react";
import { CircleCheck, OctagonX, Search } from "lucide-react";
import type { PortHolder } from "@/shared/bridge";
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
export function PortSettings() {
  const saved = useSettingsStore((s) => s.settings);
  if (!saved) return null;
  return <PortTools initialPort={saved.port} />;
}

function PortTools({ initialPort }: { initialPort: number }) {
  const [text, setText] = useState(String(initialPort));
  const [confirming, setConfirming] = useState<number | null>(null);
  const { holders, checkedPort, freedNote, pending, error, check, free, reset } = usePortTools();
  const port = parsePort(text);

  const onCheck = () => {
    setConfirming(null);
    void check(port);
  };

  return (
    <GlassPanel className="flex flex-col gap-4 p-6">
      <header className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <OctagonX className="size-5" aria-hidden /> Free a port
        </h2>
        <p className="text-sm text-muted-foreground">
          If the server won't start because the port is already in use, find the program holding it and end it.
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
          <span className="text-xs font-medium text-muted-foreground">Port</span>
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
            aria-label="Port to check"
          />
        </label>
        <GlassButton type="submit" disabled={pending}>
          <Search /> Check
        </GlassButton>
      </form>

      {holders !== null && holders.length === 0 && (
        <p role="status" className="flex items-center gap-2 text-sm text-success">
          <CircleCheck className="size-4 shrink-0" aria-hidden />
          {freedNote ?? `Port ${checkedPort} is free.`}
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
  const label = holder.name || "Unknown program";
  return (
    <li className="glass flex flex-wrap items-center justify-between gap-3 rounded-glass-sm px-3 py-2">
      <div className="min-w-0">
        <p className="truncate text-sm font-medium">
          {label} <span className="font-mono text-xs text-muted-foreground">pid {holder.pid}</span>
        </p>
        {holder.self && (
          <p className="text-xs text-muted-foreground">This is EinCast. Stop the server from the Dashboard to free the port.</p>
        )}
        {confirming && (
          <p className="text-xs text-warning">Unsaved work in this program may be lost.</p>
        )}
      </div>

      {!holder.self &&
        (confirming ? (
          <div className="flex gap-2">
            <GlassButton size="sm" disabled={pending} onClick={onEnd}>
              <OctagonX /> End {label}
            </GlassButton>
            <GlassButton size="sm" disabled={pending} onClick={onCancel}>
              Cancel
            </GlassButton>
          </div>
        ) : (
          <GlassButton size="sm" disabled={pending} onClick={onAsk}>
            End process…
          </GlassButton>
        ))}
    </li>
  );
}
