import { describe, expect, it } from "vitest";
import type { Channel, ChannelStatus } from "./api/types";
import { summarize } from "./pages/Dashboard";

function ch(id: string, status: ChannelStatus, connectedPlatforms?: string[]): Channel {
  return {
    id, name: id, status, statusReason: "", allowedTransitions: [], niche: "n", language: "en-US", geoTarget: "US",
    strategy: null, identity: null, blueprint: null, connectedPlatforms, version: 1, createdAt: "", updatedAt: "",
  };
}

describe("summarize", () => {
  it("counts portfolio groups and ignores archived channels", () => {
    const s = summarize([
      ch("a", "active", ["youtube"]),
      ch("b", "draft"),
      ch("c", "awaiting_platform_setup"),
      ch("d", "paused", ["youtube"]),
      ch("e", "archived", ["youtube"]),
    ]);
    expect(s).toEqual({ total: 4, active: 1, needsSetup: 2, connected: 2 });
  });

  it("handles an empty portfolio", () => {
    expect(summarize([])).toEqual({ total: 0, active: 0, needsSetup: 0, connected: 0 });
  });
});
