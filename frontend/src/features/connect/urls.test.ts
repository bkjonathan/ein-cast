import { describe, expect, it } from "vitest";
import { hostOf, playlistUrl } from "./urls";

describe("playlistUrl", () => {
  it.each([
    ["http://192.168.1.20:8765", "http://192.168.1.20:8765/playlist.m3u"],
    ["http://192.168.1.20:8765/", "http://192.168.1.20:8765/playlist.m3u"],
    ["http://192.168.1.20:8765///", "http://192.168.1.20:8765/playlist.m3u"],
  ])("%s → %s", (base, expected) => {
    expect(playlistUrl(base)).toBe(expected);
  });
});

describe("hostOf", () => {
  it("returns the hostname without scheme or port", () => {
    expect(hostOf("http://192.168.1.20:8765")).toBe("192.168.1.20");
    expect(hostOf("http://eincast.local:8765/x")).toBe("eincast.local");
  });

  it("returns the input unchanged when it is not a URL", () => {
    expect(hostOf("not a url")).toBe("not a url");
  });
});
