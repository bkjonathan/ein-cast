import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { UpdateStatus } from "@/shared/bridge";

const mem = vi.hoisted(() => ({ status: null as UpdateStatus | null }));
vi.mock("../store", () => ({
  useUpdatesStore: <T,>(select: (s: typeof mem) => T): T => select(mem),
}));

import { UpdateNotice } from "./UpdateNotice";

const status = (over: Partial<UpdateStatus> = {}): UpdateStatus => ({
  current: "1.1.1", checking: false, available: false, latest: null, checkedAt: 0, error: "", ...over,
});

beforeEach(() => {
  mem.status = null;
});

describe("UpdateNotice", () => {
  it("is silent until Go reports, and when nothing newer exists", () => {
    expect(renderToStaticMarkup(<UpdateNotice onOpen={vi.fn()} />)).toBe("");
    mem.status = status({ checkedAt: 1 });
    expect(renderToStaticMarkup(<UpdateNotice onOpen={vi.fn()} />)).toBe("");
  });

  it("names the new version", () => {
    mem.status = status({
      available: true,
      latest: { version: "1.1.2", name: "x", notes: "", publishedAt: 0, assets: [] },
    });
    expect(renderToStaticMarkup(<UpdateNotice onOpen={vi.fn()} />)).toMatch(/Update available: 1\.1\.2/);
  });

  it("stays while a re-check runs or fails: the notice does not vanish with the Wi-Fi", () => {
    const latest = { version: "1.1.2", name: "x", notes: "", publishedAt: 0, assets: [] };
    mem.status = status({ available: true, latest, checking: true });
    expect(renderToStaticMarkup(<UpdateNotice onOpen={vi.fn()} />)).toMatch(/1\.1\.2/);
    mem.status = status({ available: true, latest, error: "offline" });
    expect(renderToStaticMarkup(<UpdateNotice onOpen={vi.fn()} />)).toMatch(/1\.1\.2/);
  });
});
