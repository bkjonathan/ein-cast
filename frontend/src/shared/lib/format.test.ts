import { describe, expect, it } from "vitest";
import { formatBytes, formatDate, formatDateTime } from "./format";

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

describe("formatBytes in other languages", () => {
  it("uses the language's separators but Latin digits and the usual units", () => {
    expect(formatBytes(1536, "de")).toBe("1,5 KB");
    expect(formatBytes(1536, "my")).toBe("1.5 KB");
    expect(formatBytes(5 * 1024 ** 3, "my")).toBe("5.0 GB");
  });
});

describe("formatDate and formatDateTime", () => {
  const moment = Date.UTC(2026, 10, 1, 12, 30); // noon UTC: the same calendar day in every time zone

  it("names the day", () => {
    expect(formatDate(moment)).toMatch(/2026/);
    expect(formatDateTime(moment)).toMatch(/2026/);
    expect(formatDateTime(moment).length).toBeGreaterThan(formatDate(moment).length);
  });

  it("keeps Latin digits in Burmese", () => {
    expect(formatDate(moment, "my")).toMatch(/2026/);
    expect(formatDate(moment, "my")).not.toMatch(/[၀-၉]/);
  });

  it.each([0, -1, Number.NaN])("returns a dash for %s (unknown)", (input) => {
    expect(formatDate(input)).toBe("—");
    expect(formatDateTime(input)).toBe("—");
  });
});
