import type { LogLine } from "@/shared/bridge";

export type WizardStep = "folder" | "share" | "connect";

export const wizardSteps: readonly { id: WizardStep; title: string }[] = [
  { id: "folder", title: "Choose your media" },
  { id: "share", title: "Start sharing" },
  { id: "connect", title: "Open it on a screen" },
];

/** 1-based position of a step, for "Step 2 of 3". */
export function stepNumber(step: WizardStep): number {
  return wizardSteps.findIndex((s) => s.id === step) + 1;
}

const loopback = new Set(["127.0.0.1", "::1", "localhost"]);

/**
 * The first request from another device after `afterSeq`, or null. The access
 * log already records every request, so "a device connected" needs nothing new in
 * Go: it is a request from an address that is not this computer, that was
 * answered (not turned away), for something other than the browser's icon.
 */
export function firstDevice(lines: readonly LogLine[], afterSeq: number): { client: string } | null {
  for (const line of lines) {
    if (line.seq <= afterSeq || line.source !== "http" || line.message !== "request") continue;
    const attr = (key: string) => line.attrs.find((a) => a.key === key)?.value ?? "";
    const client = attr("client_ip");
    const status = Number(attr("status"));
    if (client === "" || loopback.has(client)) continue;
    if (!(status >= 200 && status < 400)) continue;
    if (attr("path") === "/favicon.ico") continue;
    return { client };
  }
  return null;
}

/** What each system asks the first time something listens for other computers. */
export function firewallNote(platform: string): string {
  switch (platform) {
    case "darwin":
      return "If macOS asks whether EinCast may accept incoming network connections, choose Allow. Without it, other devices can't connect.";
    case "windows":
      return "If Windows asks whether to let EinCast through the firewall, tick Private networks and choose Allow access. Without it, other devices can't connect.";
    default:
      return "If your system asks whether EinCast may accept incoming connections, allow it.";
  }
}
