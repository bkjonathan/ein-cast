import { describe, expect, it } from "vitest";
import { hostOf, playlistUrl, withToken } from "./urls";

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

describe("withToken", () => {
  it("returns the URL unchanged when there is no token", () => {
    expect(withToken("http://192.168.1.20:8765", "")).toBe("http://192.168.1.20:8765");
  });

  it("adds the token as ?t=", () => {
    expect(withToken("http://192.168.1.20:8765", "abc_DEF-123")).toBe("http://192.168.1.20:8765/?t=abc_DEF-123");
  });

  it("keeps the path of a playlist URL", () => {
    expect(withToken(playlistUrl("http://192.168.1.20:8765"), "tok")).toBe(
      "http://192.168.1.20:8765/playlist.m3u?t=tok",
    );
  });

  it("escapes awkward characters so the server reads back the same token", () => {
    const url = withToken("http://h:1", "a+b/c=d e&f#g");
    expect(new URL(url).searchParams.get("t")).toBe("a+b/c=d e&f#g");
  });

  it("replaces an existing t parameter instead of adding a second", () => {
    expect(withToken("http://h:1/?t=old", "new")).toBe("http://h:1/?t=new");
  });
});
