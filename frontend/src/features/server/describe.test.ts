import { describe, expect, it } from "vitest";
import type { ServerState } from "@/shared/bridge";
import { allowedActions, describeState } from "./describe";

describe("describeState", () => {
  it.each<[ServerState, string, string]>([
    ["idle", "Idle", "neutral"],
    ["starting", "Starting…", "warning"],
    ["running", "Running", "success"],
    ["stopping", "Stopping…", "warning"],
    ["failed", "Failed", "danger"],
  ])("%s → %s (%s)", (state, label, tone) => {
    expect(describeState(state)).toEqual({ label, tone });
  });
});

describe("allowedActions", () => {
  it.each<[ServerState, boolean, boolean]>([
    ["idle", true, false],
    ["starting", false, false],
    ["running", false, true],
    ["stopping", false, false],
    ["failed", true, false],
  ])("%s → start=%s stop=%s", (state, canStart, canStop) => {
    expect(allowedActions(state)).toEqual({ canStart, canStop });
  });
});
