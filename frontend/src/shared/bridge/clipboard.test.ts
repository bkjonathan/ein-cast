import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const runtime = vi.hoisted(() => ({ ClipboardSetText: vi.fn() }));
vi.mock("../../../wailsjs/runtime/runtime", () => runtime);

import { copyText } from "./clipboard";

const writeText = vi.fn();

beforeEach(() => {
  vi.resetAllMocks();
  vi.stubGlobal("navigator", { clipboard: { writeText } });
});
afterEach(() => vi.unstubAllGlobals());

describe("copyText", () => {
  it("uses the native clipboard when it works", async () => {
    runtime.ClipboardSetText.mockResolvedValue(true);
    await copyText("http://192.168.1.20:8765");
    expect(runtime.ClipboardSetText).toHaveBeenCalledWith("http://192.168.1.20:8765");
    expect(writeText).not.toHaveBeenCalled();
  });

  it("falls back to the browser clipboard when Wails reports failure", async () => {
    runtime.ClipboardSetText.mockResolvedValue(false);
    writeText.mockResolvedValue(undefined);
    await copyText("x");
    expect(writeText).toHaveBeenCalledWith("x");
  });

  it("falls back when there is no Wails runtime at all", async () => {
    runtime.ClipboardSetText.mockRejectedValue(new TypeError("window.runtime is undefined"));
    writeText.mockResolvedValue(undefined);
    await copyText("x");
    expect(writeText).toHaveBeenCalledWith("x");
  });

  it("rejects when nothing can copy", async () => {
    runtime.ClipboardSetText.mockResolvedValue(false);
    writeText.mockRejectedValue(new Error("denied"));
    await expect(copyText("x")).rejects.toThrow("denied");
  });
});
