// A bridge module: allowed to import ../wailsjs/runtime.
import { ClipboardSetText } from "../../../wailsjs/runtime/runtime";

/**
 * Copies text to the system clipboard. The native Wails clipboard is tried
 * first because a desktop webview does not always grant the browser API; the
 * browser API covers plain-browser development.
 */
export async function copyText(text: string): Promise<void> {
  try {
    if (await ClipboardSetText(text)) return;
  } catch {
    // No Wails runtime (plain-browser dev): fall through to the browser API.
  }
  await navigator.clipboard.writeText(text);
}
