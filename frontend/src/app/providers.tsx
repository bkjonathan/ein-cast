import type { ReactNode } from "react";
import { MotionConfig } from "motion/react";

/**
 * App-wide providers. Theming is pure CSS (prefers-color-scheme), so the only
 * provider today is motion, set to honour the OS "reduce motion" setting.
 */
export function Providers({ children }: { children: ReactNode }) {
  return <MotionConfig reducedMotion="user">{children}</MotionConfig>;
}
