import { motion, type HTMLMotionProps } from "motion/react";
import { cva, type VariantProps } from "class-variance-authority";
import { spring } from "@/shared/motion/presets";
import { cn } from "@/shared/lib/cn";

const panel = cva("glass", {
  variants: {
    radius: {
      sm: "rounded-glass-sm",
      md: "rounded-glass-md",
      lg: "rounded-glass-lg",
    },
  },
  defaultVariants: { radius: "lg" },
});

type GlassPanelProps = HTMLMotionProps<"div"> & VariantProps<typeof panel>;

/** A frosted surface that eases in on mount. */
export function GlassPanel({ radius, className, ...props }: GlassPanelProps) {
  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={spring.smooth}
      className={cn(panel({ radius }), className)}
      {...props}
    />
  );
}
