import { colorOf } from "../colors";

/** A round coloured badge with the first letter of the name. */
export function Avatar({ name, color, size = 36 }: { name: string; color: string; size?: number }) {
  return (
    <span
      aria-hidden
      className="grid shrink-0 place-items-center rounded-full font-semibold text-white"
      style={{ width: size, height: size, background: colorOf(color), fontSize: size * 0.45 }}
    >
      {Array.from(name.trim())[0]?.toUpperCase() ?? "?"}
    </span>
  );
}
