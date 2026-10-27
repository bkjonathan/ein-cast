import type { ServerState } from "@/shared/bridge";
import type { MessageKey } from "@/shared/i18n";

export type Tone = "neutral" | "success" | "warning" | "danger";

/** The words (as a translation key) and color for each lifecycle state. */
export function describeState(state: ServerState): { labelKey: MessageKey; tone: Tone } {
  switch (state) {
    case "idle":
      return { labelKey: "server.state.idle", tone: "neutral" };
    case "starting":
      return { labelKey: "server.state.starting", tone: "warning" };
    case "running":
      return { labelKey: "server.state.running", tone: "success" };
    case "stopping":
      return { labelKey: "server.state.stopping", tone: "warning" };
    case "failed":
      return { labelKey: "server.state.failed", tone: "danger" };
  }
}

/** What the user can do right now. Starting from "failed" is a retry. */
export function allowedActions(state: ServerState): { canStart: boolean; canStop: boolean } {
  return {
    canStart: state === "idle" || state === "failed",
    canStop: state === "running",
  };
}

/**
 * Did the server fail because another program holds its port? Go passes the
 * operating system's words through, and they differ: macOS says "address already
 * in use", Windows says "Only one usage of each socket address".
 */
export function isPortInUse(error: string): boolean {
  const e = error.toLowerCase();
  return e.includes("address already in use") || e.includes("only one usage of each socket address");
}
