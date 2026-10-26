import { LoaderCircle, RefreshCw } from "lucide-react";
import { useMemo, useState } from "react";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";
import { effectiveLibrary, nameById } from "../derive";
import { browseItems } from "../filter";
import { listRows } from "../group";
import { useLibraryActions } from "../hooks/useLibraryActions";
import { useThumbnailSupport } from "../hooks/useThumbnail";
import { useLibraryStore } from "../store";
import { LibraryList } from "./LibraryList";
import { LibraryToolbar } from "./LibraryToolbar";

/** The Library screen: filters, and the files themselves. */
export function LibraryView() {
  const libraries = useLibraryStore((s) => s.libraries);
  const items = useLibraryStore((s) => s.items);
  const kind = useLibraryStore((s) => s.kind);
  const chosen = useLibraryStore((s) => s.library);
  const query = useLibraryStore((s) => s.query);
  const sort = useLibraryStore((s) => s.sort);
  const { rescanAll, busy } = useLibraryActions();

  const thumbnailSupport = useThumbnailSupport();
  const library = effectiveLibrary(chosen, libraries);
  // What the kind chips count: the files of the chosen library, before the kind
  // and search narrow them.
  const inLibrary = useMemo(
    () => (library === "all" ? items : items.filter((it) => it.libraryId === library)),
    [items, library],
  );
  const visible = useMemo(
    () => browseItems(items, kind, query, sort, library),
    [items, kind, query, sort, library],
  );
  // Series that are open. View state, like the filters: Go knows nothing of it.
  const [opened, setOpened] = useState<ReadonlySet<string>>(new Set());
  const toggleShow = (key: string) =>
    setOpened((prev) => {
      const next = new Set(prev);
      if (!next.delete(key)) next.add(key);
      return next;
    });
  // A search opens every series, so its matches are not hidden behind closed headers.
  const rows = useMemo(() => listRows(visible, opened, query.trim() !== ""), [visible, opened, query]);
  const names = useMemo(() => nameById(libraries), [libraries]);
  const problems = libraries.filter((l) => l.error);
  const scanning = libraries.some((l) => l.scanning);
  if (libraries.length === 0) return null;

  return (
    <div className="flex h-full flex-col gap-3">
      {(problems.length > 0 || libraries.length > 1) && (
        <GlassPanel className="flex flex-col gap-2 p-4">
          {libraries.length > 1 && (
            <div className="flex items-center justify-between gap-3">
              <p className="text-sm text-muted-foreground">{libraries.length} libraries</p>
              <GlassButton size="sm" onClick={() => void rescanAll()} disabled={busy}>
                {scanning ? <LoaderCircle className="animate-spin" /> : <RefreshCw />} Rescan all
              </GlassButton>
            </div>
          )}
          {problems.map((l) => (
            <p key={l.id} role="alert" className="text-sm text-destructive">
              {l.name}: {l.error} {l.total > 0 && "Showing the last successful scan."}
            </p>
          ))}
        </GlassPanel>
      )}
      <LibraryToolbar libraries={libraries} items={inLibrary} shown={visible.length} />
      {thumbnailSupport === false && (
        <p className="px-1 text-xs text-muted-foreground">
          Install ffmpeg (for example “brew install ffmpeg”) to see thumbnails here and on your devices.
        </p>
      )}
      <GlassPanel className="min-h-0 flex-1 overflow-hidden">
        <LibraryList
          rows={rows}
          onToggleShow={toggleShow}
          libraryNames={libraries.length > 1 ? names : undefined}
          emptyHint={
            items.length === 0
              ? "No supported media found yet."
              : "Nothing matches the filters."
          }
        />
      </GlassPanel>
    </div>
  );
}
