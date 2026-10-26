import { describe, expect, it, vi } from "vitest";
import type { LibraryItem } from "@/shared/bridge";
import { createThumbnailLoader, thumbnailKey } from "./thumbnailCache";

const item = (id: string, over: Partial<LibraryItem> = {}): LibraryItem => ({
  id,
  libraryId: "L1",
  name: `${id}.mp4`,
  relPath: `${id}.mp4`,
  kind: "video",
  size: 10,
  modTime: 100,
  ...over,
});

describe("thumbnail loader", () => {
  it("asks Go once per picture, however often it is wanted", async () => {
    const fetch = vi.fn().mockResolvedValue("data:a");
    const loader = createThumbnailLoader(fetch);

    await expect(loader.load(item("a"))).resolves.toBe("data:a");
    await expect(loader.load(item("a"))).resolves.toBe("data:a");
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("shares one request between rows that ask at the same time", async () => {
    let resolve: (url: string) => void = () => undefined;
    const fetch = vi.fn().mockReturnValue(new Promise<string>((r) => (resolve = r)));
    const loader = createThumbnailLoader(fetch);

    const first = loader.load(item("a"));
    const second = loader.load(item("a"));
    resolve("data:a");

    await expect(Promise.all([first, second])).resolves.toEqual(["data:a", "data:a"]);
    expect(fetch).toHaveBeenCalledTimes(1);
  });

  it("asks again for a file that changed", async () => {
    const fetch = vi.fn().mockResolvedValue("data:x");
    const loader = createThumbnailLoader(fetch);

    await loader.load(item("a"));
    await loader.load(item("a", { modTime: 200 }));
    await loader.load(item("a", { size: 11 }));
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it("does not remember 'no picture', so it can be asked again later", async () => {
    const fetch = vi.fn().mockResolvedValueOnce("").mockResolvedValueOnce("data:a");
    const loader = createThumbnailLoader(fetch);

    await expect(loader.load(item("a"))).resolves.toBe("");
    await Promise.resolve(); // let the cleanup run
    await expect(loader.load(item("a"))).resolves.toBe("data:a");
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it("treats a failed request as no picture", async () => {
    const fetch = vi.fn().mockRejectedValueOnce(new Error("bridge down")).mockResolvedValueOnce("data:a");
    const loader = createThumbnailLoader(fetch);

    await expect(loader.load(item("a"))).resolves.toBe("");
    await Promise.resolve();
    await expect(loader.load(item("a"))).resolves.toBe("data:a");
  });

  it("forgets the least recently used picture past its limit", async () => {
    const fetch = vi.fn((id: string) => Promise.resolve(`data:${id}`));
    const loader = createThumbnailLoader(fetch, 2);

    await loader.load(item("a"));
    await loader.load(item("b"));
    await loader.load(item("a")); // a is now newer than b
    await loader.load(item("c")); // pushes out b
    fetch.mockClear();

    await loader.load(item("a"));
    await loader.load(item("c"));
    expect(fetch).not.toHaveBeenCalled();
    await loader.load(item("b"));
    expect(fetch).toHaveBeenCalledTimes(1);
  });
});

describe("thumbnailKey", () => {
  it("differs when the file's version does", () => {
    expect(thumbnailKey(item("a"))).not.toBe(thumbnailKey(item("a", { modTime: 101 })));
    expect(thumbnailKey(item("a"))).toBe(thumbnailKey(item("a")));
  });
});
