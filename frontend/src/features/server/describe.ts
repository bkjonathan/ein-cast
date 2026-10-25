import type { ServerState } from "@/shared/bridge";

export type Tone = "neutral" | "success" | "warning" | "danger";

/** Label and color for each lifecycle state. */
export function describeState(state: ServerState): { label: string; tone: Tone } {
  switch (state) {
    case "idle":
      return { label: "Idle", tone: "neutral" };
    case "starting":
      return { label: "Starting…", tone: "warning" };
    case "running":
      return { label: "Running", tone: "success" };
    case "stopping":
      return { label: "Stopping…", tone: "warning" };
    case "failed":
      return { label: "Failed", tone: "danger" };
  }
}

/** What the user can do right now. Starting from "failed" is a retry. */
export function allowedActions(state: ServerState): { canStart: boolean; canStop: boolean } {
  return {
    canStart: state === "idle" || state === "failed",
    canStop: state === "running",
  };
}
