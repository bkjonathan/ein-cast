/** Named springs shared by every animation so the app moves with one feel. */
export const spring = {
  snappy: { type: "spring", stiffness: 500, damping: 32 },
  smooth: { type: "spring", stiffness: 260, damping: 26 },
  gentle: { type: "spring", stiffness: 140, damping: 20 },
} as const;
