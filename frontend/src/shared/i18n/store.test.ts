import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useLocaleStore } from "./store";

// The tests run without a browser, so the two things the store touches are stood in for.
let saved: Map<string, string>;
let documentElement: { lang: string };

beforeEach(() => {
  saved = new Map();
  documentElement = { lang: "" };
  vi.stubGlobal("localStorage", {
    getItem: (k: string) => saved.get(k) ?? null,
    setItem: (k: string, v: string) => void saved.set(k, v),
  });
  vi.stubGlobal("document", { documentElement });
});

afterEach(() => {
  vi.unstubAllGlobals();
  useLocaleStore.getState().setPreference("system");
});

describe("the language store", () => {
  it("applies a choice at once: the language, the page's lang, and the saved preference", () => {
    useLocaleStore.getState().setPreference("my");
    expect(useLocaleStore.getState().locale).toBe("my");
    expect(useLocaleStore.getState().preference).toBe("my");
    expect(documentElement.lang).toBe("my");
    expect(saved.get("eincast.language")).toBe("my");
  });

  it("'system' resolves to a language we have even with no browser to ask", () => {
    useLocaleStore.getState().setPreference("my");
    useLocaleStore.getState().setPreference("system");
    expect(useLocaleStore.getState().locale).toBe("en");
    expect(documentElement.lang).toBe("en");
  });

  it("still works when the browser refuses to remember", () => {
    vi.stubGlobal("localStorage", {
      getItem: () => null,
      setItem: () => {
        throw new Error("blocked");
      },
    });
    expect(() => useLocaleStore.getState().setPreference("my")).not.toThrow();
    expect(useLocaleStore.getState().locale).toBe("my");
  });
});
