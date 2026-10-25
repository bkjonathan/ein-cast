/**
 * Frontend-owned view of what Go sends. We map the generated wailsjs classes
 * into these plain types in commands.ts so the rest of the UI never depends on
 * generated code (the JS-side anti-corruption layer).
 */
export interface AppInfo {
  name: string;
  version: string;
  platform: string;
}

export interface KindCounts {
  video: number;
  audio: number;
  image: number;
}

/** The library Go currently has loaded. File paths never cross the bridge. */
export interface LibrarySummary {
  root: string;
  total: number;
  totalSize: number;
  counts: KindCounts;
}
