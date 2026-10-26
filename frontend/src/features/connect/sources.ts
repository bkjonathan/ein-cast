import type { RemoteStatus, ServerStatus } from "@/shared/bridge";
import { hostOf } from "./urls";

/**
 * One way of reaching the library that the Connect card can show:
 * - lan: another device on the same network;
 * - private: a tunnel only the user's own devices can reach (Tailscale);
 * - public: a tunnel anyone can reach, guarded by the PIN.
 */
export type SourceKind = "lan" | "private" | "public";

export interface ConnectSource {
  kind: SourceKind;
  /** Short name for the tab. */
  label: string;
  base: string;
  /**
   * The access token to put in links, or "". Never set for a public source:
   * it signs in with the PIN, and the LAN token is not accepted there.
   */
  token: string;
  /** Whether a bare playlist link works. Over a public address a player cannot sign in. */
  playlist: boolean;
}

/**
 * Everything the Connect card can offer right now: each LAN address, then the
 * remote address if remote access is up. Pure, so the rules are tested apart
 * from the component.
 */
export function connectSources(
  server: ServerStatus,
  remote: RemoteStatus | null,
  lanToken: string,
  remoteLabel: string,
): ConnectSource[] {
  const sources: ConnectSource[] = server.lanUrls.map((base) => ({
    kind: "lan",
    label: hostOf(base),
    base,
    token: lanToken,
    playlist: true,
  }));

  if (remote && remote.state === "running" && remote.live === "running" && remote.url) {
    sources.push(
      remote.public
        ? { kind: "public", label: remoteLabel, base: remote.url, token: "", playlist: false }
        : { kind: "private", label: remoteLabel, base: remote.url, token: lanToken, playlist: true },
    );
  }
  return sources;
}
