import { Check, FolderX, LoaderCircle, Pencil, RefreshCw, Trash2 } from "lucide-react";
import { useState } from "react";
import type { LibraryState } from "@/shared/bridge";
import { useT } from "@/shared/i18n";
import { formatBytes } from "@/shared/lib/format";
import { GlassButton } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import { Switch } from "@/shared/ui/Switch";
import { useLibraryActions } from "../hooks/useLibraryActions";

/**
 * One library with everything you can do to it: see its folder and what is in
 * it, rename it, scan it again, take it out. A failed scan keeps the previous
 * contents, and says so.
 */
export function LibraryCard({ library }: { library: LibraryState }) {
  const { rescan, remove, rename, setHiddenFromTVs, busy, error } = useLibraryActions();
  const [editing, setEditing] = useState<string | null>(null);
  const [confirming, setConfirming] = useState(false);
  const { t, locale } = useT();

  async function commitRename() {
    if (editing === null) return;
    if (editing.trim() === library.name || (await rename(library.id, editing))) setEditing(null);
  }

  return (
    <div className="glass flex min-w-0 flex-col gap-3 rounded-glass-md p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        {editing === null ? (
          <h3 className="min-w-0 truncate text-base font-semibold">{library.name}</h3>
        ) : (
          <form
            className="flex min-w-0 flex-1 items-center gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              void commitRename();
            }}
          >
            <Input
              autoFocus
              value={editing}
              maxLength={64}
              aria-label={t("library.card.name")}
              onChange={(e) => setEditing(e.target.value)}
              onKeyDown={(e) => e.key === "Escape" && setEditing(null)}
              className="h-8 text-sm"
            />
            <GlassButton size="icon" type="submit" aria-label={t("library.card.saveName")} disabled={busy}>
              <Check />
            </GlassButton>
          </form>
        )}
        <div className="flex gap-2">
          <GlassButton size="icon" onClick={() => setEditing(library.name)} aria-label={t("library.card.rename", { name: library.name })}>
            <Pencil />
          </GlassButton>
          <GlassButton size="sm" onClick={() => void rescan(library.id)} disabled={busy}>
            {library.scanning ? <LoaderCircle className="animate-spin" /> : <RefreshCw />}
            {library.scanning ? t("library.card.scanning") : t("library.card.rescan")}
          </GlassButton>
          <GlassButton size="icon" onClick={() => setConfirming(true)} aria-label={t("library.card.remove", { name: library.name })}>
            <Trash2 />
          </GlassButton>
        </div>
      </div>

      <p className="min-w-0 truncate text-sm text-muted-foreground select-text" title={library.root}>
        {library.root}
      </p>
      <p className="text-xs text-muted-foreground tabular-nums">
        {library.total === 0
          ? t("library.card.none")
          : t("library.card.counts", { video: library.counts.video, audio: library.counts.audio, image: library.counts.image, size: formatBytes(library.totalSize, locale) })}
      </p>
      <p className="text-xs text-muted-foreground">
        {library.watching
          ? t("library.card.watching")
          : t("library.card.notWatching")}
      </p>

      <div className="flex items-start justify-between gap-4">
        <p className="text-xs text-muted-foreground">
          {library.hiddenFromTVs
            ? t("library.card.hiddenFromTVs")
            : t("library.card.shownToTVs")}
        </p>
        <Switch
          checked={!library.hiddenFromTVs}
          label={t("library.card.showToTVs", { name: library.name })}
          disabled={busy}
          onChange={(show) => void setHiddenFromTVs(library.id, !show)}
        />
      </div>

      {library.error && (
        <p role="alert" className="flex items-start gap-2 text-sm text-destructive">
          <FolderX className="mt-0.5 size-4 shrink-0" aria-hidden />
          {library.error} {library.total > 0 && t("library.card.lastScan")}
        </p>
      )}

      {confirming && (
        <div className="flex flex-wrap items-center gap-2" role="group" aria-label={t("library.card.confirmGroup")}>
          <p className="text-sm text-muted-foreground">
            {t("library.card.confirm", { name: library.name })}
          </p>
          <GlassButton
            size="sm"
            disabled={busy}
            onClick={async () => {
              await remove(library.id);
              setConfirming(false);
            }}
          >
            <Trash2 /> {t("library.card.removeButton")}
          </GlassButton>
          <GlassButton size="sm" onClick={() => setConfirming(false)}>
            {t("common.cancel")}
          </GlassButton>
        </div>
      )}

      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
    </div>
  );
}
