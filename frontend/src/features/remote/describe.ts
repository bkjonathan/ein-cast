import type { RemoteProvider, RemoteStatus } from "@/shared/bridge";

import type { MessageKey, Translator } from "@/shared/i18n";

export type Tone = "neutral" | "success" | "warning" | "danger";

/** How remote access looks in the status pill. */
export function summarize(status: RemoteStatus): { labelKey: MessageKey; tone: Tone } {
  switch (status.state) {
    case "idle":
      return { labelKey: "remote.state.idle", tone: "neutral" };
    case "starting":
      return { labelKey: "remote.state.starting", tone: "warning" };
    case "running":
      // The provider can lose its connection while we still intend it to be on.
      if (status.live !== "running") return { labelKey: "remote.state.reconnecting", tone: "warning" };
      // Amber, not green, while the Internet can reach it.
      return { labelKey: "remote.state.on", tone: status.public ? "warning" : "success" };
    case "stopping":
      return { labelKey: "remote.state.stopping", tone: "warning" };
    case "failed":
      return { labelKey: "remote.state.failed", tone: "danger" };
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
): MessageKey | null {
  if (!serverRunning) return "remote.blocker.server";
  if (!provider) return "remote.blocker.choose";
  if (status.needsSetup.includes(provider.id)) return "remote.blocker.setup";
  if (provider.public && !status.pinSet) return "remote.blocker.pin";
  if (noticePending) return "remote.blocker.notice";
  return null;
}

/**
 * The warning shown in the title bar, or null when nothing is public. It errs
 * on the side of alarm: while a stop is unconfirmed the library may still be
 * reachable, and the text says so.
 */
export function publicBadge(status: RemoteStatus | null, tr: Translator): string | null {
  if (!status?.public) return null;
  switch (status.state) {
    case "stopping":
      return tr.t("remote.badge.stopping");
    case "failed":
      return tr.t("remote.badge.failed");
    default: {
      const n = status.sessions;
      return n > 0 ? tr.t("remote.badge.onSessions", { count: n }) : tr.t("remote.badge.on");
    }
  }
}
