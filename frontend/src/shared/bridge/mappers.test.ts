import { describe, expect, it } from "vitest";
import { toLibraryItem, toLibraryState, toLogLine, toRemoteProvider, toRemoteStatus, toServerStatus } from "./mappers";

const raw = (state: string) => ({
  state,
  name: "Den",
  port: 8765,
  localUrl: "http://127.0.0.1:8765",
  lanUrls: ["http://192.168.1.20:8765"], nameUrls: [], discovery: [],
  error: "",
});

describe("toServerStatus", () => {
  it.each(["idle", "starting", "running", "stopping", "failed"])("accepts %s", (state) => {
    expect(toServerStatus(raw(state)).state).toBe(state);
  });

  it("copies the other fields", () => {
    expect(toServerStatus(raw("running"))).toStrictEqual({
      state: "running",
      name: "Den",
      port: 8765,
      localUrl: "http://127.0.0.1:8765",
      lanUrls: ["http://192.168.1.20:8765"], nameUrls: [], discovery: [],
      error: "",
    });
  });

  it.each(["", "RUNNING", "paused", "unknown"])("rejects %j loudly", (state) => {
    expect(() => toServerStatus(raw(state))).toThrow(/unknown server state/);
  });
});

describe("toLogLine", () => {
  const raw = (level: string) => ({
    seq: 7,
    time: 1_700_000_000_000,
    level,
    source: "http",
    message: "request",
    attrs: [{ key: "status", value: "200" }],
  });

  it.each(["debug", "info", "warn", "error"])("keeps level %s", (level) => {
    expect(toLogLine(raw(level)).level).toBe(level);
  });

  it("shows an unfamiliar level as info instead of dropping the line", () => {
    expect(toLogLine(raw("trace")).level).toBe("info");
  });

  it("copies the other fields", () => {
    expect(toLogLine(raw("warn"))).toStrictEqual({
      seq: 7,
      time: 1_700_000_000_000,
      level: "warn",
      source: "http",
      message: "request",
      attrs: [{ key: "status", value: "200" }],
    });
  });
});

describe("toRemoteStatus", () => {
  const remote = (over: Partial<Parameters<typeof toRemoteStatus>[0]> = {}) => ({
    state: "running", provider: "tailscale", public: false, url: "http://den.ts.net:8765",
    error: "", live: "running", pinSet: false, sessions: 0, needsSetup: [] as string[], ...over,
  });

  it.each(["idle", "starting", "running", "stopping", "failed"])("accepts %s for both states", (state) => {
    const got = toRemoteStatus(remote({ state, live: state }));
    expect(got.state).toBe(state);
    expect(got.live).toBe(state);
  });

  it("copies the other fields", () => {
    expect(toRemoteStatus(remote({ public: true, pinSet: true, sessions: 3, error: "x" }))).toStrictEqual({
      state: "running", provider: "tailscale", public: true, url: "http://den.ts.net:8765",
      error: "x", live: "running", pinSet: true, sessions: 3, needsSetup: [],
    });
  });

  it("throws on an unknown state, naming it", () => {
    expect(() => toRemoteStatus(remote({ state: "paused" }))).toThrow(/paused/);
    expect(() => toRemoteStatus(remote({ live: "" }))).toThrow();
  });
});

describe("toRemoteProvider", () => {
  const field = (kind: string) => ({ key: "k", label: "L", help: "h", placeholder: "p", kind, required: true });

  it.each(["text", "secret", "file"])("keeps the %s kind", (kind) => {
    expect(toRemoteProvider({ id: "x", public: true, fields: [field(kind)] }).fields[0]?.kind).toBe(kind);
  });

  it("treats a kind it does not know as a secret: write-only is the safe reading", () => {
    expect(toRemoteProvider({ id: "x", public: true, fields: [field("password-manager")] }).fields[0]?.kind).toBe("secret");
    expect(toRemoteProvider({ id: "x", public: true, fields: [field("")] }).fields[0]?.kind).toBe("secret");
  });

  it("copies the rest and allows no fields", () => {
    expect(toRemoteProvider({ id: "tailscale", public: false, fields: [] })).toStrictEqual({
      id: "tailscale", public: false, fields: [],
    });
  });
});

describe("toRemoteStatus needsSetup", () => {
  it("is copied, not shared", () => {
    const raw = { state: "idle", provider: "", public: false, url: "", error: "", live: "idle", pinSet: false, sessions: 0, needsSetup: ["ngrok"] };
    const got = toRemoteStatus(raw);
    expect(got.needsSetup).toEqual(["ngrok"]);
    expect(got.needsSetup).not.toBe(raw.needsSetup);
  });
});

describe("toLibraryItem", () => {
  const raw = (kind: string) => ({
    id: "ab12",
    libraryId: "L1",
    name: "a.mp3",
    relPath: "music/a.mp3",
    kind,
    size: 3,
    modTime: 7,
  });

  it.each(["video", "audio", "image"])("keeps kind %s", (kind) => {
    expect(toLibraryItem(raw(kind)).kind).toBe(kind);
  });

  it("fails loudly on a kind it does not know", () => {
    expect(() => toLibraryItem(raw("document"))).toThrow(/unknown media kind/);
  });
});

describe("toLibraryState", () => {
  const raw = {
    id: "f1", name: "Films", root: "/m/films", total: 3, totalSize: 99,
    counts: { video: 2, audio: 0, image: 1 }, scanning: false, error: "gone", watching: true,
  };

  it("copies every field", () => {
    expect(toLibraryState(raw)).toStrictEqual(raw);
  });

  it("does not share the counts object with the wire value", () => {
    const got = toLibraryState(raw);
    got.counts.video = 99;
    expect(raw.counts.video).toBe(2);
  });
});
