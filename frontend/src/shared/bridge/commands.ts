// The only module in the frontend allowed to import ../wailsjs.
import { AppInfo as goAppInfo } from "../../../wailsjs/go/bridge/App";
import type { AppInfo } from "./types";

/** Asks Go for the app name, version and host platform. */
export async function getAppInfo(): Promise<AppInfo> {
  const dto = await goAppInfo();
  return { name: dto.name, version: dto.version, platform: dto.platform };
}
