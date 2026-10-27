import type { MessageKey } from "@/shared/i18n";

/** The usual reasons a device can't see the server, in the order worth checking. */
export const networkHints: readonly { id: string; titleKey: MessageKey; textKey: MessageKey }[] = [
  { id: "same", titleKey: "settings.network.hint.same.title", textKey: "settings.network.hint.same.text" },
  { id: "isolation", titleKey: "settings.network.hint.isolation.title", textKey: "settings.network.hint.isolation.text" },
  { id: "discovery", titleKey: "settings.network.hint.discovery.title", textKey: "settings.network.hint.discovery.text" },
  { id: "firewall", titleKey: "settings.network.hint.firewall.title", textKey: "settings.network.hint.firewall.text" },
];
