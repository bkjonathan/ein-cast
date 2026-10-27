import { describe, expect, it } from "vitest";
import { forgetWindowPreferences, type ClearableStorage } from "./reset";

function memoryStorage(entries: Record<string, string>): ClearableStorage & { keys(): string[] } {
  const data = new Map(Object.entries(entries));
  return {
    get length() {
      return data.size;
    },
    key: (i) => [...data.keys()][i] ?? null,
    removeItem: (k) => void data.delete(k),
    keys: () => [...data.keys()],
  };
}

describe("forgetWindowPreferences", () => {
  it("removes every EinCast value, including ones next to each other, and keeps the rest", () => {
    const s = memoryStorage({
      "eincast.theme": "dark",
      "eincast.language": "my",
      "eincast.notice.cloudflare-quick": "1",
      "other-app.theme": "dark",
    });
    forgetWindowPreferences(s);
    expect(s.keys()).toEqual(["other-app.theme"]);
  });

  it("does nothing without storage", () => {
    expect(() => forgetWindowPreferences(null)).not.toThrow();
  });

  it("does not throw when storage refuses, so the reset still goes on", () => {
    const hostile: ClearableStorage = {
      length: 1,
      key: () => "eincast.theme",
      removeItem: () => {
        throw new Error("blocked");
      },
    };
    expect(() => forgetWindowPreferences(hostile)).not.toThrow();
  });
});
