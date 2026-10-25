import type { ServerState } from "@/shared/bridge";
import { GlassPill } from "@/shared/ui/glass";
import { describeState } from "../describe";

/** The server's state as a colored pill. Morphs in size when the label changes. */
export function StatusPill({ state }: { state: ServerState }) {
  const { label, tone } = describeState(state);
  return (
    <GlassPill tone={tone} role="status" aria-live="polite">
      {label}
    </GlassPill>
  );
}
