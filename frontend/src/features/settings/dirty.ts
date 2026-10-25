import type { Settings } from "@/shared/bridge";

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

/** Port text to a number; anything unparsable becomes 0, which Go rejects with a clear message. */
export function parsePort(text: string): number {
  const n = Number(text.trim());
  return Number.isInteger(n) ? n : 0;
}
