import { cn } from "@/shared/lib/cn";

interface SwitchProps {
  checked: boolean;
  onChange: (next: boolean) => void;
  /** Read out by screen readers; the visible text sits beside the switch. */
  label: string;
  disabled?: boolean;
}

/** An on/off switch. It reports the wish; the caller decides what the new state is. */
export function Switch({ checked, onChange, label, disabled }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={cn(
        "app-no-drag relative h-7 w-12 shrink-0 rounded-full transition-colors disabled:opacity-50",
        checked ? "bg-primary" : "bg-(--glass-surface-hover)",
      )}
    >
      <span
        className={cn(
          "absolute top-1 left-1 size-5 rounded-full bg-white shadow transition-transform",
          checked && "translate-x-5",
        )}
      />
    </button>
  );
}
