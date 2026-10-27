import { Download } from "lucide-react";
import { useT } from "@/shared/i18n";
import { useUpdatesStore } from "../store";

/**
 * A quiet line in the sidebar when a newer version is out. It sits there until
 * the person looks; nothing pops up, and nothing is downloaded.
 */
export function UpdateNotice({ onOpen }: { onOpen: () => void }) {
  const status = useUpdatesStore((s) => s.status);
  const { t } = useT();
  if (!status?.available || !status.latest) return null;

  return (
    <button
      type="button"
      onClick={onOpen}
      className="app-no-drag mx-1 mb-1 flex items-center gap-2 rounded-glass-sm bg-(--glass-surface-hover) px-3 py-2 text-left text-xs font-medium text-success transition-colors hover:text-foreground"
    >
      <Download className="size-3.5 shrink-0" aria-hidden />
      <span className="min-w-0">{t("updates.notice", { version: status.latest.version })}</span>
    </button>
  );
}
