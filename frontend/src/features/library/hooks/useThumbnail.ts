import { useEffect, useState } from "react";
import { getThumbnail, thumbnailsAvailable, type LibraryItem } from "@/shared/bridge";
import { createThumbnailLoader } from "../thumbnailCache";

/**
 * How long a row must stay on screen before its picture is requested. Scrolling
 * past a thousand files quickly would otherwise queue a thousand ffmpeg runs
 * for rows nobody looked at.
 */
const SETTLE_MS = 150;

// One loader for the app: pictures outlive the rows that show them.
const loader = createThumbnailLoader(getThumbnail);

/** The picture for a library row as a data URL, or null while there is none. */
export function useThumbnail(item: LibraryItem): string | null {
  // Keep the answer together with the item it belongs to: a recycled row that
  // now shows another file must not flash the previous file's picture.
  const [loaded, setLoaded] = useState<{ id: string; url: string } | null>(null);

  useEffect(() => {
    let live = true;
    const timer = setTimeout(() => {
      void loader.load(item).then((url) => {
        if (live && url) setLoaded({ id: item.id, url });
      });
    }, SETTLE_MS);
    return () => {
      live = false;
      clearTimeout(timer);
    };
    // The key parts, not the object: a rescan hands out new objects for the
    // same unchanged files.
  }, [item.id, item.size, item.modTime, item.poster]);

  return loaded?.id === item.id ? loaded.url : null;
}

/**
 * Whether thumbnails can be made on this computer. Null until Go answers. Asked
 * each time the screen opens, so installing ffmpeg takes effect without a restart.
 */
export function useThumbnailSupport(): boolean | null {
  const [available, setAvailable] = useState<boolean | null>(null);
  useEffect(() => {
    thumbnailsAvailable()
      .then(setAvailable)
      // No Go runtime (plain-browser dev): say nothing rather than nag.
      .catch(() => setAvailable(null));
  }, []);
  return available;
}
