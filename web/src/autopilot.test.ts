import { describe, expect, it } from "vitest";
import { asAutopilotResult } from "./api/guards";

describe("asAutopilotResult", () => {
  it("keeps only string entries", () => {
    expect(asAutopilotResult({ channelId: "c1", generated: ["name", 3], notes: ["Picked X", null] })).toEqual({
      channelId: "c1",
      generated: ["name"],
      notes: ["Picked X"],
    });
  });

  it("defaults missing notes to an empty list", () => {
    expect(asAutopilotResult({ channelId: "c1", generated: [] }).notes).toEqual([]);
  });

  it("rejects malformed results", () => {
    expect(() => asAutopilotResult({ generated: [] })).toThrow();
    expect(() => asAutopilotResult(null)).toThrow();
  });
});
