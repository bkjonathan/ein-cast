import { motion } from "motion/react";
import { QRCodeSVG } from "qrcode.react";
import { spring } from "@/shared/motion/presets";

/**
 * The QR code for a URL. Always dark on white, whatever the theme: scanners
 * need the contrast and the quiet zone, and a "glass" QR would not scan.
 */
export function QrCode({ value }: { value: string }) {
  return (
    <motion.div
      // A new URL (another network card picked) replays the reveal.
      key={value}
      initial={{ opacity: 0, scale: 0.92 }}
      animate={{ opacity: 1, scale: 1 }}
      transition={spring.gentle}
      role="img"
      aria-label={`QR code for ${value}`}
      className="shrink-0 self-center rounded-glass-md bg-white p-3 shadow-lg"
    >
      <QRCodeSVG value={value} size={168} level="M" marginSize={0} bgColor="#ffffff" fgColor="#000000" />
    </motion.div>
  );
}
