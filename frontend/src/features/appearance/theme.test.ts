import { describe, expect, it } from "vitest";
import { applyTheme, parseTheme, readTheme, saveTheme, type ThemeStorage } from "./theme";

const memory = (initial: Record<string, string> = {}): ThemeStorage & { data: Record<string, string> } => {
  const data = { ...initial };
  return {
    data,
    getItem: (k) => data[k] ?? null,
    setItem: (k, v) => {
      data[k] = v;
    },
  };
};

const broken: ThemeStorage = {
  getItem: () => {
    throw new Error("blocked");
  },
  setItem: () => {
    throw new Error("blocked");
  },
};

describe("parseTheme", () => {
  it.each(["system", "light", "dark"])("accepts %s", (mode) => {
    expect(parseTheme(mode)).toBe(mode);
  });

  it.each([null, undefined, "", "sepia", "Dark"])("treats %j as system", (raw) => {
    expect(parseTheme(raw)).toBe("system");
  });
});

describe("readTheme and saveTheme", () => {
  it("round-trips through storage", () => {
    const storage = memory();
    saveTheme(storage, "dark");
    expect(readTheme(storage)).toBe("dark");
  });

  it("defaults to system with no storage or an empty one", () => {
    expect(readTheme(null)).toBe("system");
    expect(readTheme(memory())).toBe("system");
  });

  it("survives storage that throws", () => {
    expect(readTheme(broken)).toBe("system");
    expect(() => saveTheme(broken, "light")).not.toThrow();
    expect(() => saveTheme(null, "light")).not.toThrow();
  });
});

describe("applyTheme", () => {
  it("sets data-theme for a forced theme and removes it for system", () => {
    const target = { dataset: {} as Record<string, string | undefined> };
    applyTheme(target, "dark");
    expect(target.dataset["theme"]).toBe("dark");
    applyTheme(target, "light");
    expect(target.dataset["theme"]).toBe("light");
    applyTheme(target, "system");
    expect("theme" in target.dataset).toBe(false);
  });
});
