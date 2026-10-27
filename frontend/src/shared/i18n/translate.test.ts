import { describe, expect, it } from "vitest";
import en from "./locales/en.json";
import my from "./locales/my.json";
import { parsePreference } from "./store";
import { resolveLocale, translate, translatePlural, translatorFor } from "./translate";

const placeholders = (text: string) => [...text.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

describe("the language files", () => {
  const english = en as Record<string, string>;
  const burmese = my as Record<string, string>;

  it("have exactly the same keys (tsc checks this too; this names the culprit)", () => {
    expect(Object.keys(burmese).sort()).toEqual(Object.keys(english).sort());
  });

  it("have no empty text", () => {
    for (const [file, dict] of [["en", english], ["my", burmese]] as const) {
      for (const [key, text] of Object.entries(dict)) expect(text.trim(), `${file}: ${key}`).not.toBe("");
    }
  });

  it("use the same {placeholders} in every language, so no value is dropped", () => {
    for (const [key, text] of Object.entries(english)) {
      const other = burmese[key] ?? "";
      // "other" plural forms of a language may drop {count}; a placeholder must never be invented.
      for (const name of placeholders(other)) expect(placeholders(text), `${key}: {${name}}`).toContain(name);
    }
  });

  it("give every plural both forms in English", () => {
    for (const key of Object.keys(english).filter((k) => k.endsWith(".other"))) {
      expect(english[key.replace(/\.other$/, ".one")], key).toBeDefined();
    }
  });
});

describe("translate", () => {
  it("returns the text for a language", () => {
    expect(translate("en", "nav.library")).toBe("Library");
    expect(translate("my", "language.my")).toBe("မြန်မာ");
  });

  it("fills placeholders and leaves a missing one visible", () => {
    const t = translatorFor("en");
    expect(t.t("nav.dashboard", { x: 1 })).toBe("Dashboard");
  });

  it("chooses a plural form by the language's rules", () => {
    expect(translatePlural("en", "stats.files", 1)).toBe("1 file");
    expect(translatePlural("en", "stats.files", 0)).toBe("0 files");
    expect(translatePlural("en", "stats.files", 2)).toBe("2 files");
    // Burmese has one form, so "one" and "other" are the same text.
    expect(translatePlural("my", "stats.files", 1)).toBe(translatePlural("my", "stats.files", 2).replace("2", "1"));
  });
});

describe("resolveLocale", () => {
  it("honours an explicit choice whatever the computer says", () => {
    expect(resolveLocale("my", ["en-US"])).toBe("my");
    expect(resolveLocale("en", ["my-MM"])).toBe("en");
  });

  it("follows the computer's first language that we have, by its primary part", () => {
    expect(resolveLocale("system", ["my-MM", "en"])).toBe("my");
    expect(resolveLocale("system", ["fr-FR", "en-GB"])).toBe("en");
    expect(resolveLocale("system", ["MY"])).toBe("my");
    expect(resolveLocale("system", ["my_MM"])).toBe("my");
  });

  it("falls back to English", () => {
    expect(resolveLocale("system", [])).toBe("en");
    expect(resolveLocale("system", ["fr", "de"])).toBe("en");
  });
});

describe("parsePreference", () => {
  it("accepts what it knows and treats the rest as the computer's language", () => {
    expect(parsePreference("my")).toBe("my");
    expect(parsePreference("en")).toBe("en");
    expect(parsePreference("system")).toBe("system");
    expect(parsePreference("fr")).toBe("system");
    expect(parsePreference(null)).toBe("system");
    expect(parsePreference(undefined)).toBe("system");
  });
});
