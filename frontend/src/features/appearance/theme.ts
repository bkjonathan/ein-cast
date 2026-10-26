// The Appearance setting. It is a preference of this window, not of the server,
// so it lives in the browser's storage rather than in Go's settings file. All
// storage access is guarded: it can be missing or refuse writes (private
// windows, blocked site data), and the app then simply follows the OS.

/** "system" follows the OS; the others force a theme. */
export type ThemeMode = "system" | "light" | "dark";

export const themeModes: readonly ThemeMode[] = ["system", "light", "dark"];

const storageKey = "eincast.theme";

/** The part of Web Storage this needs, so tests can supply a stand-in. */
export interface ThemeStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

/** The browser's local storage, or null where there is none or access throws. */
export function browserStorage(): ThemeStorage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

/** Anything unrecognised (a hand-edited value, a future mode) means "system". */
export function parseTheme(raw: string | null | undefined): ThemeMode {
  return themeModes.find((m) => m === raw) ?? "system";
}

export function readTheme(storage: ThemeStorage | null): ThemeMode {
  try {
    return parseTheme(storage?.getItem(storageKey));
  } catch {
    return "system";
  }
}

/** Remembers the choice. Failure to remember is not an error. */
export function saveTheme(storage: ThemeStorage | null, mode: ThemeMode): void {
  try {
    storage?.setItem(storageKey, mode);
  } catch {
    // The choice lasts until the window closes.
  }
}

/** The part of <html> this touches. */
export interface ThemeTarget {
  dataset: Record<string, string | undefined>;
}

/**
 * Puts the mode on <html> where the stylesheet looks for it. "system" removes
 * the attribute, which hands control back to prefers-color-scheme.
 */
export function applyTheme(target: ThemeTarget, mode: ThemeMode): void {
  if (mode === "system") delete target.dataset["theme"];
  else target.dataset["theme"] = mode;
}
