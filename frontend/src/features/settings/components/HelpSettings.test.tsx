import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { HelpSettings } from "./HelpSettings";

describe("HelpSettings", () => {
  const html = renderToStaticMarkup(<HelpSettings />);

  it("offers the report to copy and the three pages", () => {
    for (const label of ["Copy diagnostics", "Report a problem", "Suggest an idea", "Privacy statement"]) {
      expect(html).toContain(label);
    }
  });

  it("promises nothing is sent, and shows no result before anything was done", () => {
    expect(html).toMatch(/sends nothing about you/);
    expect(html).not.toMatch(/role="alert"/);
    expect(html).not.toMatch(/role="status"/);
  });
});
