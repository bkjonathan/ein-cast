import { useMemo } from "react";
import { create } from "zustand";
import { localePreferences, resolveLocale, translatorFor, type Locale, type LocalePreference, type Translator } from "./translate";

// Like the theme, the language is a preference of this window, kept in the
// browser's storage; every access is guarded because storage can be missing or
// refuse writes, and the window then follows the computer.
const storageKey = "eincast.language";

interface LanguageStorage {
  getItem(key: string): string | null;
  setItem(key: string, value: string): void;
}

function browserStorage(): LanguageStorage | null {
  try {
    return typeof localStorage === "undefined" ? null : localStorage;
  } catch {
    return null;
  }
}

/** Anything unrecognised (a hand-edited value, a language from a newer version) means "system". */
export function parsePreference(raw: string | null | undefined): LocalePreference {
  return localePreferences.find((p) => p === raw) ?? "system";
}

function systemLanguages(): readonly string[] {
  if (typeof navigator === "undefined") return [];
  return navigator.languages?.length ? navigator.languages : [navigator.language];
}

function readPreference(): LocalePreference {
  try {
    return parsePreference(browserStorage()?.getItem(storageKey));
  } catch {
    return "system";
  }
}

/** The window's language bookkeeping. `locale` is always a language we have. */
interface LocaleState {
  preference: LocalePreference;
  locale: Locale;
  setPreference: (preference: LocalePreference) => void;
}

const initial = readPreference();

export const useLocaleStore = create<LocaleState>()((set) => ({
  preference: initial,
  locale: resolveLocale(initial, systemLanguages()),
  setPreference: (preference) => {
    try {
      browserStorage()?.setItem(storageKey, preference);
    } catch {
      // The choice lasts until the window closes.
    }
    const locale = resolveLocale(preference, systemLanguages());
    applyDocumentLanguage(locale);
    set({ preference, locale });
  },
}));

/** Tells the browser which language the page is in (spelling, fonts, screen readers). */
export function applyDocumentLanguage(locale: Locale): void {
  if (typeof document !== "undefined") document.documentElement.lang = locale;
}

/** Sets <html lang> from the saved preference. Call before the first render. */
export function initLocale(): void {
  applyDocumentLanguage(useLocaleStore.getState().locale);
}

/** The translator for the window's language; components re-render when it changes. */
export function useT(): Translator {
  const locale = useLocaleStore((s) => s.locale);
  return useMemo(() => translatorFor(locale), [locale]);
}
