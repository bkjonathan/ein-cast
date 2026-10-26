import type { RemoteProvider, RemoteStatus } from "@/shared/bridge";

export type Tone = "neutral" | "success" | "warning" | "danger";

/** How remote access looks in the status pill. */
export function summarize(status: RemoteStatus): { label: string; tone: Tone } {
  switch (status.state) {
    case "idle":
      return { label: "Off", tone: "neutral" };
    case "starting":
      return { label: "Connecting…", tone: "warning" };
    case "running":
      // The provider can lose its connection while we still intend it to be on.
      if (status.live !== "running") return { label: "Reconnecting…", tone: "warning" };
      // Amber, not green, while the Internet can reach it.
      return { label: "On", tone: status.public ? "warning" : "success" };
    case "stopping":
      return { label: "Turning off…", tone: "warning" };
    case "failed":
      return { label: "Failed", tone: "danger" };
  }
}

/**
 * What the user can do now. A failure with a provider still recorded means the
 * tunnel could not be confirmed stopped, so the only safe action is to turn it
 * off again; a failure without one means it never came up and can be retried.
 */
export function remoteActions(status: RemoteStatus): { canEnable: boolean; canDisable: boolean } {
  const neverCameUp = status.state === "failed" && status.provider === "";
  return {
    canEnable: status.state === "idle" || neverCameUp,
    // "Turn off" while starting is how a slow connection is cancelled.
    canDisable:
      status.state === "starting" ||
      status.state === "running" ||
      (status.state === "failed" && status.provider !== ""),
  };
}

/** Why a provider cannot be turned on yet, or null. Go enforces this too. */
export function enableBlocker(
  provider: RemoteProvider | undefined,
  status: RemoteStatus,
  serverRunning: boolean,
  noticePending = false,
): string | null {
  if (!serverRunning) return "Start the server first.";
  if (!provider) return "Choose a way to connect.";
  if (status.needsSetup.includes(provider.id)) return "Finish setting this up first.";
  if (provider.public && !status.pinSet) return "Set an access PIN first.";
  if (noticePending) return "Read the notice above first.";
  return null;
}

/**
 * The warning shown in the title bar, or null when nothing is public. It errs
 * on the side of alarm: while a stop is unconfirmed the library may still be
 * reachable, and the text says so.
 */
export function publicBadge(status: RemoteStatus | null): string | null {
  if (!status?.public) return null;
  switch (status.state) {
    case "stopping":
      return "Public access turning off…";
    case "failed":
      return "Public access may still be ON";
    default: {
      const n = status.sessions;
      return n > 0 ? `Public access ON · ${n} signed in` : "Public access ON";
    }
  }
}
