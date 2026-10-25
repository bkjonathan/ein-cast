import type { ComponentProps } from "react";
import { cn } from "@/shared/lib/cn";

/** A horizontal glass strip for grouped controls (filters, actions). */
export function GlassToolbar({ className, ...props }: ComponentProps<"div">) {
  return (
    <div
      role="toolbar"
      className={cn(
        "glass flex items-center gap-2 rounded-glass-md px-3 py-2",
        className,
      )}
      {...props}
    />
  );
}
