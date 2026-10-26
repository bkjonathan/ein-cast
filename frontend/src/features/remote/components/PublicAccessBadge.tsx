import { TriangleAlert } from "lucide-react";
import { GlassPill } from "@/shared/ui/glass";
import { publicBadge } from "../describe";
import { useRemoteStore } from "../store";

/**
 * The persistent amber warning in the title bar while the library may be open
 * to the Internet. It is a button: clicking it opens the Remote screen, where
 * it can be turned off.
 */
export function PublicAccessBadge({ onOpen }: { onOpen: () => void }) {
  const text = useRemoteStore((s) => publicBadge(s.status));
  if (!text) return null;
  return (
    <button type="button" onClick={onOpen} className="app-no-drag ml-auto" aria-label={`${text}. Open remote access settings.`}>
      <GlassPill tone="warning" role="status" aria-live="polite">
        <TriangleAlert className="size-3.5" aria-hidden />
        {text}
      </GlassPill>
    </button>
  );
}
