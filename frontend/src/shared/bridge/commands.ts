// The only module in the frontend allowed to import ../wailsjs/go.
import {
  AppInfo as goAppInfo,
  ChooseFolder as goChooseFolder,
  GetLibrarySummary as goGetLibrarySummary,
} from "../../../wailsjs/go/bridge/App";
import type { AppInfo, LibrarySummary } from "./types";

/** Asks Go for the app name, version and host platform. */
export async function getAppInfo(): Promise<AppInfo> {
  const dto = await goAppInfo();
  return { name: dto.name, version: dto.version, platform: dto.platform };
}

/**
 * Opens the native folder picker and scans the choice. Resolves when Go is done
 * (including when the user cancels); the new library arrives as a
 * "library:scanned" event, not as a return value. Rejects with Go's message.
 */
export async function chooseFolder(): Promise<void> {
  await goChooseFolder();
}

/** The library Go has loaded right now, or null before the first scan. */
export async function getLibrarySummary(): Promise<LibrarySummary | null> {
  // The generated type claims a non-null DTO, but Go returns nil (→ null) before
  // the first scan.
  const dto: Awaited<ReturnType<typeof goGetLibrarySummary>> | null =
    await goGetLibrarySummary();
  if (!dto) return null;
  return {
    root: dto.root,
    total: dto.total,
    totalSize: dto.totalSize,
    counts: { ...dto.counts },
  };
}
