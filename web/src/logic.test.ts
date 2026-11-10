import { describe, expect, it } from "vitest";
import { asChannel, asJob, asNicheReport, ResponseShapeError } from "./api/guards";
import type { Channel } from "./api/types";
import { formatCalendarDate, t } from "./i18n";
import { messages } from "./i18n/en-US";

// Import pure helpers only; page modules pull in React components.
import { completed } from "./pages/ChannelWizard";
import { nextPublication } from "./pages/Dashboard";

const baseChannel: Channel = {
  id: "c1",
  name: "",
  status: "draft",
  statusReason: "",
  allowedTransitions: ["branding", "archived"],
  niche: "Space",
  language: "en-US",
  geoTarget: "US",
  strategy: null,
  identity: null,
  blueprint: null,
  version: 1,
  createdAt: "2026-11-10T00:00:00Z",
  updatedAt: "2026-11-10T00:00:00Z",
};

describe("response guards", () => {
  it("accepts a valid channel", () => {
    expect(asChannel(baseChannel).id).toBe("c1");
  });

  it("rejects malformed channels", () => {
    expect(() => asChannel({ ...baseChannel, status: "deleted" })).toThrow(ResponseShapeError);
    expect(() => asChannel({ id: 1 })).toThrow(ResponseShapeError);
    expect(() => asChannel(null)).toThrow(ResponseShapeError);
  });

  it("validates jobs and niche reports", () => {
    expect(asJob({ id: "j", kind: "k", status: "running" }).status).toBe("running");
    expect(() => asJob({ id: "j", kind: "k", status: "done" })).toThrow(ResponseShapeError);
    expect(() => asNicheReport({ niches: [{ name: "x" }], dataSources: [] })).toThrow(ResponseShapeError);
    expect(asNicheReport({ niches: [{ name: "x", heuristicScore: 50 }], dataSources: [] }).niches).toHaveLength(1);
  });
});

describe("i18n", () => {
  it("substitutes placeholders and keeps unknown ones", () => {
    expect(t("dashboard.bulkResult", { ok: 2, failed: 1 })).toBe("2 updated, 1 skipped.");
    expect(t("dashboard.bulkResult", { ok: 2 })).toBe("2 updated, {failed} skipped.");
  });

  it("formats calendar dates without shifting the day", () => {
    expect(formatCalendarDate("2026-11-13")).toBe("Nov 13, 2026");
  });

  it("has no empty messages", () => {
    for (const [key, value] of Object.entries(messages)) {
      expect(value.trim(), key).not.toBe("");
    }
  });
});

describe("wizard progress", () => {
  it("never marks platform setup complete without a verified connection", () => {
    expect(completed("platforms", { ...baseChannel, status: "awaiting_platform_setup" })).toBe(false);
  });

  it("requires both identity and a selected name for branding", () => {
    const withIdentity = { ...baseChannel, identity: {} as Channel["identity"] };
    expect(completed("branding", withIdentity)).toBe(false);
    expect(completed("branding", { ...withIdentity, name: "Quiet Orbit" })).toBe(true);
  });
});

describe("dashboard", () => {
  it("finds the next publication on or after today", () => {
    const c: Channel = {
      ...baseChannel,
      blueprint: {
        priorityTopics: [],
        series: [],
        formats: [],
        hooks: [],
        ideas: [],
        calendar: [
          { date: "2026-11-01", localTime: "17:00", timeZone: "America/New_York", ideaIndex: 0 },
          { date: "2026-11-13", localTime: "17:00", timeZone: "America/New_York", ideaIndex: 1 },
        ],
        provenance: { generator: "ollama", model: "m", generatedAt: "", editedByUser: false },
      },
    };
    expect(nextPublication(c, "2026-11-10")).toBe("Nov 13, 2026 17:00");
    expect(nextPublication(c, "2026-12-01")).toBeNull();
    expect(nextPublication(baseChannel, "2026-11-10")).toBeNull();
  });
});
