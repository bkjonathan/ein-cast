import type { MessageKey } from "@/shared/i18n";

/** The settings that are plain on/off switches on the startup panel. */
export type ToggleKey = "autoStart" | "keepAwake" | "openAtLogin" | "hideOnClose";

/** What closing the window does when it only hides the app, by platform (a translation key). */
export function hideOnCloseHelp(platform: string): MessageKey {
  switch (platform) {
    case "darwin":
      return "settings.startup.hideOnClose.darwin";
    case "windows":
      return "settings.startup.hideOnClose.windows";
    default:
      return "settings.startup.hideOnClose.fallback";
  }
}

/** What each way of starting does. Kept as data so the wording is testable. */
export const startupRows: readonly { key: ToggleKey; titleKey: MessageKey; helpKey: MessageKey }[] = [
  {
    key: "autoStart",
    titleKey: "settings.startup.autoStart.title",
    helpKey: "settings.startup.autoStart.help",
  },
  {
    key: "keepAwake",
    titleKey: "settings.startup.keepAwake.title",
    helpKey: "settings.startup.keepAwake.help",
  },
  {
    key: "openAtLogin",
    titleKey: "settings.startup.openAtLogin.title",
    helpKey: "settings.startup.openAtLogin.help",
  },
];
