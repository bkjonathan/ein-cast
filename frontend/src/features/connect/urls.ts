/** The M3U playlist that VLC, Infuse and TV players can open. */
export function playlistUrl(base: string): string {
  return `${base.replace(/\/+$/, "")}/playlist.m3u`;
}

/** The host part of a URL, for a compact label: "http://192.168.1.20:8765" → "192.168.1.20". */
export function hostOf(url: string): string {
  try {
    return new URL(url).hostname;
  } catch {
    return url;
  }
}
