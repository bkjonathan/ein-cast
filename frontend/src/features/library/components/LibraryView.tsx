import { useMemo } from "react";
import { GlassPanel } from "@/shared/ui/glass";
import { browseItems } from "../filter";
import { useLibraryStore } from "../store";
import { useThumbnailSupport } from "../hooks/useThumbnail";
import { LibraryFolder } from "./LibraryFolder";
import { LibraryList } from "./LibraryList";
import { LibraryToolbar } from "./LibraryToolbar";

/** The Library screen: folder, filters, and the files themselves. */
export function LibraryView() {
  const summary = useLibraryStore((s) => s.summary);
  const items = useLibraryStore((s) => s.items);
  const kind = useLibraryStore((s) => s.kind);
  const query = useLibraryStore((s) => s.query);
  const sort = useLibraryStore((s) => s.sort);

  const thumbnailSupport = useThumbnailSupport();
  const visible = useMemo(() => browseItems(items, kind, query, sort), [items, kind, query, sort]);
  if (!summary) return null;

  return (
    <div className="flex h-full flex-col gap-3">
      <GlassPanel className="p-4">
        <LibraryFolder root={summary.root} />
      </GlassPanel>
      <LibraryToolbar items={items} shown={visible.length} />
      {thumbnailSupport === false && (
        <p className="px-1 text-xs text-muted-foreground">
          Install ffmpeg (for example “brew install ffmpeg”) to see thumbnails here and on your devices.
        </p>
      )}
      <GlassPanel className="min-h-0 flex-1 overflow-hidden">
        <LibraryList
          items={visible}
          emptyHint={
            items.length === 0
              ? "No supported media found in this folder."
              : "Nothing matches the filters."
          }
        />
      </GlassPanel>
    </div>
  );
}
