import { describe, expect, it } from "vitest";
import { toLogLine, toServerStatus } from "./mappers";

const raw = (state: string) => ({
  state,
  name: "Den",
  port: 8765,
  localUrl: "http://127.0.0.1:8765",
  lanUrls: ["http://192.168.1.20:8765"],
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
      lanUrls: ["http://192.168.1.20:8765"],
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
