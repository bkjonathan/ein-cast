/** The settings that are plain on/off switches on the startup panel. */
export type ToggleKey = "autoStart" | "keepAwake" | "openAtLogin" | "hideOnClose";

/** What closing the window does when it only hides the app, by platform. */
export function hideOnCloseHelp(platform: string): string {
  switch (platform) {
    case "darwin":
      return "EinCast stays in the Dock and keeps sharing. Quit it with ⌘Q (EinCast → Quit) to stop. Takes effect the next time EinCast opens.";
    case "windows":
      return "EinCast stays in the notification area and keeps sharing. Right-click its icon and choose Quit to stop. Takes effect the next time EinCast opens.";
    default:
      return "EinCast keeps sharing when the window is closed. Takes effect the next time EinCast opens.";
  }
}

/** What each way of starting does. Kept as data so the wording is testable. */
export const startupRows: readonly { key: ToggleKey; title: string; help: string }[] = [
  {
    key: "autoStart",
    title: "Start sharing when EinCast opens",
    help: "The server starts by itself, so there is nothing to click before a TV can connect.",
  },
  {
    key: "keepAwake",
    title: "Keep my computer awake while sharing",
    help: "A sleeping laptop can't be reached. The screen may still turn off. Applies the next time the server starts.",
  },
  {
    key: "openAtLogin",
    title: "Open EinCast when I sign in",
    help: "EinCast starts with the computer, in the background when closing the window keeps it sharing.",
  },
];
