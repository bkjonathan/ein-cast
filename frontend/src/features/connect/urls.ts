/**
 * Adds the access token to a URL as "?t=…", the form the server accepts from
 * QR codes, links and players. With no token the URL is returned unchanged.
 */
export function withToken(url: string, token: string): string {
  if (!token) return url;
  const u = new URL(url);
  u.searchParams.set("t", token);
  return u.toString();
}

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
