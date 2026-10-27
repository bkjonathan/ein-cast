import type { Settings, SettingsInput } from "@/shared/bridge";

export interface ServerForm {
  /** Kept as text so a half-typed number is not rewritten under the cursor. */
  port: string;
  name: string;
}

export function formFrom(saved: Pick<Settings, "port" | "name">): ServerForm {
  return { port: String(saved.port), name: saved.name };
}

/** Has the user changed anything that is not saved yet? */
export function isDirty(form: ServerForm, saved: Pick<Settings, "port" | "name">): boolean {
  return form.port.trim() !== String(saved.port) || form.name.trim() !== saved.name;
}

/**
 * The server uses the port and name it was started with, so a saved change only
 * applies after a restart. True when the running server differs from the saved
 * settings.
 */
export function needsRestart(
  running: { state: string; port: number; name: string } | null,
  saved: Pick<Settings, "port" | "name">,
): boolean {
  return (
    running !== null &&
    running.state === "running" &&
    (running.port !== saved.port || running.name !== saved.name)
  );
}

/**
 * Smart TV discovery is announced when the server starts, so the running server
 * differs from the saved setting until it is restarted. `discovery` is what Go
 * reports as active ("mdns", "dlna").
 */
export function tvNeedsRestart(
  running: { state: string; discovery: string[] } | null,
  saved: Pick<Settings, "dlna">,
): boolean {
  return running !== null && running.state === "running" && running.discovery.includes("dlna") !== saved.dlna;
}

/** Port text to a number; anything unparsable becomes 0, which Go rejects with a clear message. */
export function parsePort(text: string): number {
  const n = Number(text.trim());
  return Number.isInteger(n) ? n : 0;
}

/**
 * The whole editable form, taken from what is saved with some fields changed.
 * Go replaces every field on each save, so a screen that changes one setting must
 * send the others back unchanged; building the input here keeps that in one place.
 */
export function inputFrom(saved: SettingsInput, change: Partial<SettingsInput> = {}): SettingsInput {
  return {
    port: saved.port,
    name: saved.name,
    requireToken: saved.requireToken,
    dlna: saved.dlna,
    autoStart: saved.autoStart,
    keepAwake: saved.keepAwake,
    openAtLogin: saved.openAtLogin,
    hideOnClose: saved.hideOnClose,
    hardwareEncoding: saved.hardwareEncoding,
    ...change,
  };
}

/**
 * Turning Smart TVs on while the access token is on is the person's choice to
 * make: a TV cannot send the token, so Go allows only one of the two. This is the
 * change that makes that choice, in one save.
 */
export function smartTvsInsteadOfToken(saved: SettingsInput): SettingsInput {
  return inputFrom(saved, { requireToken: false, dlna: true });
}
