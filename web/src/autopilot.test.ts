import { describe, expect, it } from "vitest";
import { asAutopilotResult, asStoryFormats } from "./api/guards";

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

import { asDialogueModes } from "./api/guards";
import { speakerRole } from "./components/DialogueControls";
import type { Video } from "./api/types";

describe("dialogue modes", () => {
  it("keeps valid modes and always offers single narrator", () => {
    const modes = asDialogueModes([{ id: "qa", name: "Q&A", speakerA: "Host", speakerB: "Expert" }, { id: 3 }]);
    expect(modes.map((m) => m.id)).toEqual(["narration", "qa"]);
    expect(asDialogueModes(undefined).map((m) => m.id)).toEqual(["narration"]);
  });

  it("names each speaker's role", () => {
    const modes = asDialogueModes([{ id: "narration", name: "Single", speakerA: "Narrator", speakerB: "" }, { id: "qa", name: "Q&A", speakerA: "Host", speakerB: "Expert" }]);
    const v = { dialogueMode: "qa" } as Video;
    expect(speakerRole(modes, v, "B")).toBe("Expert");
    expect(speakerRole(modes, v, undefined)).toBe("Host");
    expect(speakerRole(modes, { dialogueMode: "narration" } as Video, "B")).toBe("");
  });
});

describe("asStoryFormats", () => {
  it("keeps well-formed formats and always offers the explainer", () => {
    const got = asStoryFormats([{ id: "escalation", name: "Absurd escalation", example: "You...", seconds: 35 }, { id: 3 }, "x"]);
    expect(got.map((f) => f.id)).toEqual(["explainer", "escalation"]);
    expect(got[1]?.seconds).toBe(35);
    expect(asStoryFormats(undefined).map((f) => f.id)).toEqual(["explainer"]);
  });
});
