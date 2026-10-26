import type { RemoteProvider } from "@/shared/bridge";

/** What the UI says about one way of reaching the library from outside. */
export interface ProviderCopy {
  label: string;
  summary: string;
  recommended: boolean;
  /**
   * Something the person must read once before using the provider, or null.
   * Turning the provider on is held back until they have.
   */
  notice: string | null;
}

// Cloudflare's terms are the same for both of its tunnels.
const cloudflareNotice =
  "Cloudflare's service terms restrict serving large amounts of video through their proxy outside their paid video products. This is fine for light personal use; heavy streaming may violate Cloudflare's terms. Tailscale is recommended for regular remote viewing.";

// Go only says which providers exist and whether they are public. The names and
// advice are presentation, so they live here and Go never has to change for a
// wording tweak.
const known: Record<string, ProviderCopy> = {
  tailscale: {
    label: "Tailscale",
    summary:
      "Private. Only devices signed in to your own Tailscale account can connect, and nothing is opened to the Internet. Install Tailscale on this computer and on the device you want to watch on.",
    recommended: true,
    notice: null,
  },
  "cloudflare-quick": {
    label: "Cloudflare quick tunnel",
    summary:
      "Public. A random web address that works from anywhere, with no account. It changes every time you turn it on. Anyone who finds the address can reach its sign-in page, so a PIN is required. Needs the free cloudflared tool installed on this computer.",
    recommended: false,
    notice: cloudflareNotice,
  },
  "cloudflare-named": {
    label: "Cloudflare named tunnel",
    summary:
      "Public. A stable address on your own domain, through your own Cloudflare account. Anyone who finds the address can reach its sign-in page, so a PIN is required. You create the tunnel in Cloudflare first, and it needs the free cloudflared tool installed on this computer.",
    recommended: false,
    notice: cloudflareNotice,
  },
  ngrok: {
    label: "ngrok",
    summary:
      "Public. A web address from your ngrok account, with nothing else to install. The free plan has a very small data allowance and shows a warning page to browsers, so it suits trying remote access, not watching films. A PIN is required.",
    recommended: false,
    notice:
      "ngrok's free plan allows very little data and is meant for testing, not for streaming movies. If you go over, ngrok can stop the tunnel. Tailscale is recommended for regular remote viewing.",
  },
};

/**
 * Extra advice for setting a provider up that depends on this app's own state,
 * or null. The server's port is something only EinCast knows, and the Cloudflare
 * dashboard must be told it.
 */
export function setupNote(providerId: string, port: number | undefined): string | null {
  if (providerId !== "cloudflare-named") return null;
  const where = port ? `http://localhost:${port}` : "http://localhost and the port shown in Settings";
  return `In Cloudflare, the tunnel's public hostname must point at this server: set its service to ${where}. If you change the port in Settings, change it there too.`;
}

/** Copy for a provider; an unfamiliar one still gets an honest, generic description. */
export function describeProvider(p: Pick<RemoteProvider, "id" | "public">): ProviderCopy {
  return (
    known[p.id] ?? {
      label: p.id,
      summary: p.public
        ? "Public: anyone who learns the address can reach its sign-in page. Needs an access PIN."
        : "Private: only your own devices can reach it.",
      recommended: false,
      notice: null,
    }
  );
}

/**
 * The provider to show selected: the one in use, else the user's pick, else the
 * recommended one, else the first.
 */
export function defaultProvider(
  providers: readonly RemoteProvider[],
  inUse: string,
  picked: string | null,
): string {
  const exists = (id: string | null): id is string => id !== null && providers.some((p) => p.id === id);
  if (exists(inUse)) return inUse;
  if (exists(picked)) return picked;
  return (providers.find((p) => describeProvider(p).recommended) ?? providers[0])?.id ?? "";
}
