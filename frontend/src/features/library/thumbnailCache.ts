import type { LibraryItem } from "@/shared/bridge";

/** Pictures kept in memory. A data URL is tens of kilobytes, so this is a few MB. */
export const MAX_CACHED = 300;

/** Fetches a picture by item ID: a data URL, or "" for none. */
export type FetchThumbnail = (id: string) => Promise<string>;

export interface ThumbnailLoader {
  /** Resolves with a data URL, or "" when there is no picture (or it failed). */
  load: (item: LibraryItem) => Promise<string>;
}

/**
 * A picture belongs to one version of a file: if the file changes, the old
 * picture must not be shown, so size and modification time are part of the key.
 */
export function thumbnailKey(item: LibraryItem): string {
  return `${item.id}:${item.size}:${item.modTime}`;
}

/**
 * Remembers pictures so scrolling a row out of view and back does not ask Go
 * again, and shares one request between rows that want the same picture. The
 * oldest-used pictures are dropped past `limit`. "No picture" is not remembered:
 * Go answers it cheaply, and it may change (ffmpeg gets installed).
 */
export function createThumbnailLoader(
  fetchThumbnail: FetchThumbnail,
  limit: number = MAX_CACHED,
): ThumbnailLoader {
  const cache = new Map<string, Promise<string>>();

  function load(item: LibraryItem): Promise<string> {
    const key = thumbnailKey(item);
    const known = cache.get(key);
    if (known) {
      // Re-insert so it counts as recently used.
      cache.delete(key);
      cache.set(key, known);
      return known;
    }

    const request = fetchThumbnail(item.id).catch(() => "");
    cache.set(key, request);
    void request.then((url) => {
      if (url === "" && cache.get(key) === request) cache.delete(key);
    });
    while (cache.size > limit) {
      const oldest = cache.keys().next();
      if (oldest.done) break;
      cache.delete(oldest.value);
    }
    return request;
  }

  return { load };
}
