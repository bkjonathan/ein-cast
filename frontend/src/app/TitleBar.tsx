import { PublicAccessBadge } from "@/features/remote";
import { cn } from "@/shared/lib/cn";
import { Logo } from "@/shared/ui/Logo";

// The macOS traffic lights float over our content (hidden-inset title bar),
// so the bar must leave room for them: they end about 70px from the edge, and
// the extra top padding puts the logo and name on the lights' centre line.
// WKWebView always reports "Macintosh".
const isMac = navigator.userAgent.includes("Macintosh");

/** Draggable window title bar; the right side is reserved for status badges. */
export function TitleBar({ name, onOpenRemote }: { name: string; onOpenRemote: () => void }) {
  return (
    <header
      className={cn(
        "app-drag flex h-11 shrink-0 items-center gap-2.5 pr-4",
        isMac ? "pt-1.5 pl-[5.5rem]" : "pl-4",
      )}
    >
      <Logo className="size-5" />
      <h1 className="text-sm font-semibold tracking-tight">{name}</h1>
      <PublicAccessBadge onOpen={onOpenRemote} />
    </header>
  );
}
