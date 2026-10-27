import type { ServerState } from "@/shared/bridge";
import { useT } from "@/shared/i18n";
import { GlassPill } from "@/shared/ui/glass";
import { describeState } from "../describe";

/** The server's state as a colored pill. Morphs in size when the label changes. */
export function StatusPill({ state }: { state: ServerState }) {
  const { t } = useT();
  const { labelKey, tone } = describeState(state);
  return (
    <GlassPill tone={tone} role="status" aria-live="polite">
      {t(labelKey)}
    </GlassPill>
  );
}
