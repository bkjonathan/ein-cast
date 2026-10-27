import { Component, type ErrorInfo, type ReactNode } from "react";
import { TriangleAlert } from "lucide-react";
import { translate, useLocaleStore } from "@/shared/i18n";
import { GlassButton, GlassPanel } from "@/shared/ui/glass";

interface State {
  error: Error | null;
}

/**
 * Catches a crash while rendering and shows a way out instead of a blank
 * window. The server keeps running: this only restarts the interface.
 */
export class ErrorBoundary extends Component<{ children: ReactNode }, State> {
  state: State = { error: null };

  static getDerivedStateFromError(error: Error): State {
    return { error };
  }

  componentDidCatch(error: Error, info: ErrorInfo): void {
    // The webview console is the only place a render crash can be reported.
    console.error("EinCast interface crashed:", error, info.componentStack);
  }

  render(): ReactNode {
    if (!this.state.error) return this.props.children;
    // A class component cannot use a hook, and this must work even if the rest of the tree is broken.
    const locale = useLocaleStore.getState().locale;
    return (
      <div className="flex h-full items-center justify-center p-6">
        <GlassPanel className="flex max-w-md flex-col items-center gap-4 p-10 text-center">
          <TriangleAlert className="size-12 text-warning" strokeWidth={1.5} aria-hidden />
          <h2 className="text-xl font-semibold">{translate(locale, "error.title")}</h2>
          <p className="text-sm text-muted-foreground">
            {translate(locale, "error.body")}
          </p>
          <p className="max-w-full truncate font-mono text-xs text-muted-foreground select-text" title={this.state.error.message}>
            {this.state.error.message}
          </p>
          <GlassButton onClick={() => window.location.reload()}>{translate(locale, "error.reload")}</GlassButton>
        </GlassPanel>
      </div>
    );
  }
}
