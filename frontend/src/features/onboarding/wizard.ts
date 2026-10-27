import type { LogLine } from "@/shared/bridge";

import type { MessageKey } from "@/shared/i18n";

export type WizardStep = "folder" | "share" | "connect";

export const wizardSteps: readonly { id: WizardStep; titleKey: MessageKey }[] = [
  { id: "folder", titleKey: "wizard.step.folder" },
  { id: "share", titleKey: "wizard.step.share" },
  { id: "connect", titleKey: "wizard.step.connect" },
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
export function firewallNote(platform: string): MessageKey {
  switch (platform) {
    case "darwin":
      return "wizard.firewall.darwin";
    case "windows":
      return "wizard.firewall.windows";
    default:
      return "wizard.firewall.fallback";
  }
}
