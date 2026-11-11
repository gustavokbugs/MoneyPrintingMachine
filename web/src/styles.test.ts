import { describe, expect, it } from "vitest";
import { asVisualStyles } from "./api/guards";

describe("asVisualStyles", () => {
  it("reads style objects from the server", () => {
    const raw = [
      { id: "auto", name: "Automatic", description: "x" },
      { id: "tech", name: "Tech", description: "y" },
    ];
    expect(asVisualStyles(raw)).toEqual(["auto", "tech"]);
  });

  it("accepts plain ids and drops unknown or duplicate entries", () => {
    expect(asVisualStyles(["bold", "neon", "bold", 42, null])).toEqual(["bold"]);
  });

  it("falls back to auto for invalid payloads", () => {
    expect(asVisualStyles(undefined)).toEqual(["auto"]);
    expect(asVisualStyles([{ id: "unknown" }])).toEqual(["auto"]);
  });
});
