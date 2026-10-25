import type { ComponentProps } from "react";
import { Button } from "@/shared/ui/button";

/** shadcn Button pre-set to the glass variant. */
export function GlassButton(props: ComponentProps<typeof Button>) {
  return <Button variant="glass" {...props} />;
}
