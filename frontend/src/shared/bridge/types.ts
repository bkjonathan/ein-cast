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
