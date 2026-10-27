// Translation without a library. The English file is the source of truth: its keys
// are the type of every other language, so a key missing from a translation, or a
// key used in code that English does not have, fails `tsc`, not a screenshot.

import en from "./locales/en.json";
import my from "./locales/my.json";

/** Every key English has. */
export type MessageKey = keyof typeof en;

/** The languages the window can show. */
export type Locale = "en" | "my";

export const locales: readonly Locale[] = ["en", "my"];

/** A language file: exactly English's keys, so none can be forgotten. */
type Messages = Record<MessageKey, string>;

const dictionaries: Record<Locale, Messages> = { en, my };

/** "x.one" and "x.other" are two forms of the plural "x". */
export type PluralKey = Extract<MessageKey, `${string}.other`> extends `${infer B}.other` ? B : never;

export type Params = Readonly<Record<string, string | number>>;

/** Fills `{name}` placeholders. A placeholder with no value stays visible, so a bug shows. */
function fill(template: string, params: Params | undefined): string {
  if (!params) return template;
  return template.replace(/\{(\w+)\}/g, (whole, name: string) => {
    const value = params[name];
    return value === undefined ? whole : String(value);
  });
}

/** The text for a key in a language. */
export function translate(locale: Locale, key: MessageKey, params?: Params): string {
  return fill(dictionaries[locale][key], params);
}

/**
 * The text for the right form of a plural, from the language's own rules:
 * English has "one" and "other", Burmese only "other". A form a file lacks falls
 * back to "other".
 */
export function translatePlural(locale: Locale, base: PluralKey, count: number, params?: Params): string {
  const form = new Intl.PluralRules(locale).select(count);
  const dict = dictionaries[locale] as Record<string, string>;
  const template = dict[`${base}.${form}`] ?? dict[`${base}.other`] ?? base;
  return fill(template, { count, ...params });
}

/** A translator bound to a language, which is what components and text helpers take. */
export interface Translator {
  locale: Locale;
  t: (key: MessageKey, params?: Params) => string;
  tn: (base: PluralKey, count: number, params?: Params) => string;
  /** "A, B and C" in the language's own way of joining. */
  list: (items: readonly string[]) => string;
}

export function translatorFor(locale: Locale): Translator {
  return {
    locale,
    t: (key, params) => translate(locale, key, params),
    tn: (base, count, params) => translatePlural(locale, base, count, params),
    list: (items) => new Intl.ListFormat(locale, { style: "long", type: "conjunction" }).format(items),
  };
}

/** The language preference: "system" follows the computer. */
export type LocalePreference = "system" | Locale;

export const localePreferences: readonly LocalePreference[] = ["system", ...locales];

/**
 * The language to show: the chosen one, or for "system" the first language the
 * computer lists that we have (by its primary part, so "my-MM" and "en-GB" match).
 */
export function resolveLocale(preference: LocalePreference, systemLanguages: readonly string[]): Locale {
  if (preference !== "system") return preference;
  for (const tag of systemLanguages) {
    const primary = tag.toLowerCase().split(/[-_]/)[0];
    const found = locales.find((l) => l === primary);
    if (found) return found;
  }
  return "en";
}
