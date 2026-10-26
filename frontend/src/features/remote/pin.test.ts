import { describe, expect, it } from "vitest";
import { pinProblem } from "./pin";

describe("pinProblem", () => {
  it.each([
    ["12345678", null],
    ["correct horse battery", null],
    ["x".repeat(64), null],
    ["สวัสดีครับผม", null], // 12 characters, though 36 bytes
    ["😀".repeat(8), null], // 8 characters, 16 UTF-16 units
  ])("accepts %j", (pin, want) => {
    expect(pinProblem(pin)).toBe(want);
  });

  it.each([
    ["", /at least 8/],
    ["1234567", /at least 8/],
    ["😀".repeat(7), /at least 8/],
    ["x".repeat(65), /at most 64/],
    ["abcdefgh\n", /Control/],
    ["abcd\tefgh", /Control/],
    ["abcdefgh\u007f", /Control/],
  ])("rejects %j", (pin, why) => {
    expect(pinProblem(pin)).toMatch(why);
  });
});
