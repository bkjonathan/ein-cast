import { describe, expect, it } from "vitest";
import type { ServerState } from "@/shared/bridge";
import { translate } from "@/shared/i18n";
import { allowedActions, describeState, isPortInUse } from "./describe";

describe("describeState", () => {
  it.each<[ServerState, string, string]>([
    ["idle", "Idle", "neutral"],
    ["starting", "Starting…", "warning"],
    ["running", "Running", "success"],
    ["stopping", "Stopping…", "warning"],
    ["failed", "Failed", "danger"],
  ])("%s → %s (%s)", (state, label, tone) => {
    const { labelKey, ...rest } = describeState(state);
    expect({ label: translate("en", labelKey), ...rest }).toEqual({ label, tone });
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

describe("isPortInUse", () => {
  it.each([
    "start server: listen tcp 127.0.0.1:8765: bind: address already in use",
    "listen tcp 0.0.0.0:8765: bind: Only one usage of each socket address (protocol/network address/port) is normally permitted.",
  ])("recognises the system's wording: %s", (error) => {
    expect(isPortInUse(error)).toBe(true);
  });

  it.each(["", "permission denied", "name is empty", "bind: cannot assign requested address"])(
    "does not offer to free a port for %j",
    (error) => {
      expect(isPortInUse(error)).toBe(false);
    },
  );
});
