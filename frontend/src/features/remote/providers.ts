import type { RemoteProvider } from "@/shared/bridge";
import type { MessageKey, Translator } from "@/shared/i18n";

/** What the UI says about one way of reaching the library from outside. */
export interface ProviderCopy {
  /** A product name, or for an unfamiliar provider its id: not translated. */
  label: string;
  /** Translation key of the description. */
  summaryKey: MessageKey;
  recommended: boolean;
  /**
   * Translation key of something the person must read once before using the
   * provider, or null. Turning the provider on is held back until they have.
   */
  noticeKey: MessageKey | null;
}

// Go only says which providers exist and whether they are public. The names and
// advice are presentation, so they live here and Go never has to change for a
// wording tweak. Cloudflare's terms are the same for both of its tunnels.
const known: Record<string, ProviderCopy> = {
  tailscale: {
    label: "Tailscale",
    summaryKey: "remote.provider.tailscale.summary",
    recommended: true,
    noticeKey: null,
  },
  "cloudflare-quick": {
    label: "Cloudflare quick tunnel",
    summaryKey: "remote.provider.cloudflareQuick.summary",
    recommended: false,
    noticeKey: "remote.provider.cloudflareNotice",
  },
  "cloudflare-named": {
    label: "Cloudflare named tunnel",
    summaryKey: "remote.provider.cloudflareNamed.summary",
    recommended: false,
    noticeKey: "remote.provider.cloudflareNotice",
  },
  ngrok: {
    label: "ngrok",
    summaryKey: "remote.provider.ngrok.summary",
    recommended: false,
    noticeKey: "remote.provider.ngrokNotice",
  },
};

/**
 * Extra advice for setting a provider up that depends on this app's own state,
 * or null. The server's port is something only EinCast knows, and the Cloudflare
 * dashboard must be told it.
 */
export function setupNote(providerId: string, port: number | undefined, tr: Translator): string | null {
  if (providerId !== "cloudflare-named") return null;
  const where = port ? `http://localhost:${port}` : tr.t("remote.provider.portWhere");
  return tr.t("remote.provider.cloudflareNamedNote", { where });
}

/** Copy for a provider; an unfamiliar one still gets an honest, generic description. */
export function describeProvider(p: Pick<RemoteProvider, "id" | "public">): ProviderCopy {
  return (
    known[p.id] ?? {
      label: p.id,
      summaryKey: p.public ? "remote.provider.unknownPublic" : "remote.provider.unknownPrivate",
      recommended: false,
      noticeKey: null,
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
