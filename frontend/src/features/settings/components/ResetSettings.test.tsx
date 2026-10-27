import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";
import { ResetSettings } from "./ResetSettings";

describe("ResetSettings", () => {
  const html = renderToStaticMarkup(<ResetSettings />);

  it("says what is erased and that the media files are not", () => {
    expect(html).toMatch(/profiles/);
    expect(html).toMatch(/access token and PINs/);
    expect(html).toMatch(/media files are not touched/);
  });

  it("offers a button, and asks again before erasing anything", () => {
    expect(html).toMatch(/Reset EinCast…/);
    expect(html).not.toMatch(/Erase and restart/);
    expect(html).not.toMatch(/Confirm resetting/);
  });
});
