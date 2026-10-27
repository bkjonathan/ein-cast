// Resetting EinCast also forgets what this window keeps for itself: the chosen
// theme and language, and the "read this once" notices. They live in the
// browser's storage under this prefix, so the reset can find them without a list.

const prefix = "eincast.";

/** The part of Web Storage the reset needs, so tests can supply a stand-in. */
export interface ClearableStorage {
  readonly length: number;
  key(index: number): string | null;
  removeItem(key: string): void;
}

/** The browser's local storage, or null where there is none or access throws. */
export function browserStorage(): ClearableStorage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

/**
 * Removes every value EinCast stored in this window and nothing else. It never
 * throws: storage can be missing or refuse writes, and the reset must go on
 * (the preferences then simply survive).
 */
export function forgetWindowPreferences(storage: ClearableStorage | null): void {
  if (!storage) return;
  try {
    // Collected first: removing while counting would skip the neighbour of each removal.
    const ours: string[] = [];
    for (let i = 0; i < storage.length; i++) {
      const key = storage.key(i);
      if (key?.startsWith(prefix)) ours.push(key);
    }
    for (const key of ours) storage.removeItem(key);
  } catch {
    // Left as they were.
  }
}
