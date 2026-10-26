import { ChevronRight, Film, Image, Music, Tv, type LucideIcon } from "lucide-react";
import type { CSSProperties } from "react";
import type { LibraryItem, MediaKind } from "@/shared/bridge";
import { cn } from "@/shared/lib/cn";
import { formatBytes } from "@/shared/lib/format";
import { useThumbnail } from "../hooks/useThumbnail";
import { episodeCode, itemTitle } from "../titles";

const icons: Record<MediaKind, LucideIcon> = { video: Film, audio: Music, image: Image };

/** Height every row is laid out at; the virtualizer relies on it being fixed. */
export const ROW_HEIGHT = 52;

/** Folder part of a slash path with its trailing slash, or "" at the top level. */
function folderOf(relPath: string): string {
  const cut = relPath.lastIndexOf("/");
  return cut < 0 ? "" : relPath.slice(0, cut);
}

function Picture({ item, className }: { item: LibraryItem; className?: string }) {
  const Icon = icons[item.kind];
  const picture = useThumbnail(item);
  return (
    <span className={cn("glass flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-glass-sm", className)}>
      {picture ? (
        <img src={picture} alt="" className="size-full object-cover" draggable={false} />
      ) : (
        <Icon className="size-4 text-muted-foreground" aria-hidden />
      )}
    </span>
  );
}

/**
 * One file: picture, title, the folder it lives in (with its library, when there
 * are several), and its size. For an episode the title is the episode's own and
 * the code (S02E05) leads. The file name is always in the tooltip, with its path.
 */
export function LibraryRow({
  item,
  style,
  libraryName,
  indent = false,
}: {
  item: LibraryItem;
  style: CSSProperties;
  libraryName?: string | undefined;
  /** An episode under its series' header. */
  indent?: boolean;
}) {
  const code = episodeCode(item);
  const caption = indent
    ? item.name
    : [libraryName, folderOf(item.relPath)].filter(Boolean).join(" · ");
  return (
    <div
      style={style}
      title={item.relPath}
      className={cn("flex items-center gap-3 px-4", indent && "pl-12")}
    >
      <Picture item={item} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm">
          {code && <span className="mr-2 font-mono text-xs text-muted-foreground">{code}</span>}
          {indent && code && item.title === code ? item.name : itemTitle(item)}
        </p>
        {caption && <p className="truncate text-xs text-muted-foreground">{caption}</p>}
      </div>
      <span className="shrink-0 text-xs text-muted-foreground tabular-nums">{formatBytes(item.size)}</span>
    </div>
  );
}

/** A series: its poster, its name, how much there is of it; opens and closes. */
export function ShowRow({
  name,
  episodes,
  seasons,
  cover,
  open,
  onToggle,
  style,
}: {
  name: string;
  episodes: number;
  seasons: number;
  cover: LibraryItem;
  open: boolean;
  onToggle: () => void;
  style: CSSProperties;
}) {
  const detail = `${seasons > 1 ? `${seasons} seasons · ` : ""}${episodes} ${episodes === 1 ? "episode" : "episodes"}`;
  return (
    <div style={style} className="flex items-center">
      <button
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="app-no-drag flex h-full w-full items-center gap-3 px-4 text-left hover:bg-(--glass-surface-hover)"
      >
        <ChevronRight className={cn("size-4 shrink-0 text-muted-foreground transition-transform", open && "rotate-90")} aria-hidden />
        <Picture item={cover} />
        <div className="min-w-0 flex-1">
          <p className="flex items-center gap-2 truncate text-sm font-medium">
            <Tv className="size-3.5 shrink-0 text-muted-foreground" aria-hidden />
            {name}
          </p>
          <p className="truncate text-xs text-muted-foreground">{detail}</p>
        </div>
      </button>
    </div>
  );
}

/** "Season 2" between a series' episodes. */
export function SeasonRow({ label, style }: { label: string; style: CSSProperties }) {
  return (
    <div style={style} className="flex items-end px-4 pb-1 pl-12">
      <p className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{label}</p>
    </div>
  );
}
