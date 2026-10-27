/** The switch for converting videos with the computer's video chip. Kept as data so the wording is testable. */
export const hardwareEncodingRow = {
  title: "Use hardware acceleration",
  help: "Converts videos with your computer's video chip: faster, and the fan stays quieter. If the chip can't do a video, EinCast uses the processor instead. Applies to the next video converted.",
} as const;
