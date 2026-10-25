import { describe, expect, it } from "vitest";
import { formatBytes } from "./format";

describe("formatBytes", () => {
  it.each([
    [0, "0 B"],
    [1023, "1023 B"],
    [1024, "1.0 KB"],
    [1536, "1.5 KB"],
    [10 * 1024, "10 KB"],
    [5 * 1024 ** 3, "5.0 GB"],
    [3 * 1024 ** 4, "3.0 TB"],
    [5000 * 1024 ** 4, "5000 TB"],
  ])("formats %d as %s", (input, expected) => {
    expect(formatBytes(input)).toBe(expected);
  });

  it.each([-1, Number.NaN, Number.POSITIVE_INFINITY])("returns a dash for %s", (input) => {
    expect(formatBytes(input)).toBe("—");
  });
});
