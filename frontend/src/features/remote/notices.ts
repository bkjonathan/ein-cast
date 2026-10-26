// "Read this once" notices, remembered per provider. It is a courtesy, not a
// security control, so the browser's own storage is enough; and since storage
// can be missing or refuse writes (private windows, blocked site data), every
// access is guarded and the app works without it, showing the notice again.

const prefix = "eincast.notice.";

/** The part of Web Storage this needs, so tests can supply a stand-in. */
export interface NoticeStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

/** The browser's local storage, or null where there is none or access throws. */
export function browserStorage(): NoticeStorage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

export function isAcknowledged(storage: NoticeStorage | null, providerId: string): boolean {
  try {
    return storage?.getItem(prefix + providerId) === "1";
  } catch {
    return false;
  }
}

/** Remembers that the notice was read. Failure to remember is not an error. */
export function acknowledge(storage: NoticeStorage | null, providerId: string): void {
  try {
    storage?.setItem(prefix + providerId, "1");
  } catch {
    // The notice will simply be shown again next time.
  }
}
