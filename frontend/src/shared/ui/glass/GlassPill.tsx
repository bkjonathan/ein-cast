import type { ReactNode } from "react";
import { motion, type HTMLMotionProps } from "motion/react";
import { cva, type VariantProps } from "class-variance-authority";
import { spring } from "@/shared/motion/presets";
import { cn } from "@/shared/lib/cn";

const pill = cva(
  "glass inline-flex items-center gap-2 rounded-full px-3 py-1 text-xs font-medium",
  {
    variants: {
      tone: {
        neutral: "[--dot:var(--muted-foreground)]",
        success: "[--dot:var(--success)]",
        warning: "[--dot:var(--warning)]",
        danger: "[--dot:var(--destructive)]",
      },
    },
    defaultVariants: { tone: "neutral" },
  },
);

type GlassPillProps = Omit<HTMLMotionProps<"span">, "children"> &
  VariantProps<typeof pill> & { children?: ReactNode };

/** A status chip with a colored dot. `layout` lets it morph between states. */
export function GlassPill({ tone, className, children, ...props }: GlassPillProps) {
  return (
    <motion.span
      layout
      transition={spring.snappy}
      className={cn(pill({ tone }), className)}
      {...props}
    >
      <span className="size-2 rounded-full bg-(--dot)" aria-hidden />
      {children}
    </motion.span>
  );
}
