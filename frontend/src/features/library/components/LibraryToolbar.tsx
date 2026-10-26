import { Search } from "lucide-react";
import { useMemo } from "react";
import type { LibraryItem, LibraryState } from "@/shared/bridge";
import { cn } from "@/shared/lib/cn";
import { GlassToolbar } from "@/shared/ui/glass";
import { Input } from "@/shared/ui/input";
import { effectiveLibrary } from "../derive";
import { countByKind, sortLabels, type KindFilter, type SortKey } from "../filter";
import { useLibraryStore } from "../store";

const chips: readonly { key: KindFilter; label: string }[] = [
  { key: "all", label: "All" },
  { key: "video", label: "Videos" },
  { key: "audio", label: "Audio" },
  { key: "image", label: "Images" },
];

const sortKeys = Object.keys(sortLabels) as SortKey[];

/** Kind chips (with counts), search box and sort order. */
export function LibraryToolbar({
  libraries,
  items,
  shown,
}: {
  libraries: LibraryState[];
  /** The files of the chosen library; the chips count these. */
  items: LibraryItem[];
  shown: number;
}) {
  const kind = useLibraryStore((s) => s.kind);
  const chosen = useLibraryStore((s) => s.library);
  const setLibrary = useLibraryStore((s) => s.setLibrary);
  const library = effectiveLibrary(chosen, libraries);
  const query = useLibraryStore((s) => s.query);
  const sort = useLibraryStore((s) => s.sort);
  const setKind = useLibraryStore((s) => s.setKind);
  const setQuery = useLibraryStore((s) => s.setQuery);
  const setSort = useLibraryStore((s) => s.setSort);
  const counts = useMemo(() => countByKind(items), [items]);

  return (
    <GlassToolbar className="flex-wrap">
      {libraries.length > 1 && (
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          Library
          <select
            value={library}
            onChange={(e) => setLibrary(e.target.value)}
            className="app-no-drag glass h-8 max-w-40 rounded-glass-sm bg-transparent px-2 text-sm text-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
          >
            <option value="all">All</option>
            {libraries.map((l) => (
              <option key={l.id} value={l.id}>
                {l.name}
              </option>
            ))}
          </select>
        </label>
      )}
      <div className="flex gap-1" role="group" aria-label="Kind of file">
        {chips.map(({ key, label }) => (
          <button
            key={key}
            type="button"
            aria-pressed={kind === key}
            onClick={() => setKind(key)}
            className={cn(
              "app-no-drag flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs font-medium transition-colors",
              kind === key
                ? "bg-(--glass-surface-hover) text-foreground"
                : "text-muted-foreground opacity-60 hover:opacity-100",
            )}
          >
            {label}
            <span className="tabular-nums text-muted-foreground">{counts[key]}</span>
          </button>
        ))}
      </div>

      <div className="relative min-w-40 flex-1">
        <Search
          className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
          aria-hidden
        />
        <Input
          type="search"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="Search files and folders"
          aria-label="Search the library"
          className="h-8 pl-8 text-sm"
        />
      </div>

      <label className="flex items-center gap-2 text-xs text-muted-foreground">
        Sort
        <select
          value={sort}
          onChange={(e) => setSort(e.target.value as SortKey)}
          className="app-no-drag glass h-8 rounded-glass-sm bg-transparent px-2 text-sm text-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
        >
          {sortKeys.map((key) => (
            <option key={key} value={key}>
              {sortLabels[key]}
            </option>
          ))}
        </select>
      </label>

      <span className="text-xs text-muted-foreground tabular-nums">
        {shown === items.length ? `${items.length} files` : `${shown} of ${items.length}`}
      </span>
    </GlassToolbar>
  );
}
