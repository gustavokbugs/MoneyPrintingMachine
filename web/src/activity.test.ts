import { describe, expect, it } from "vitest";
import type { Job } from "./api/types";
import { activityLink, isActive, kindLabelKey } from "./components/activity";

const job = (over: Partial<Job>): Job => ({
  id: "j1", kind: "launchpad.autopilot", status: "running", attempts: 1, maxAttempts: 3, createdAt: "", updatedAt: "", ...over,
});

describe("activity helpers", () => {
  it("links each job to where its result shows", () => {
    expect(activityLink(job({ channelId: "c1" }))).toBe("/channels/c1?step=review&autopilot=j1");
    expect(activityLink(job({ kind: "production.produce", input: { videoId: "v1" } }))).toBe("/videos/v1");
    expect(activityLink(job({ kind: "launchpad.discover_niches" }))).toBe("/launchpad");
    expect(activityLink(job({ kind: "launchpad.generate_identity", channelId: "c 2" }))).toBe("/channels/c%202?step=branding");
    expect(activityLink(job({ kind: "x", input: { videoId: 5 } }))).toBe("/");
  });

  it("labels unknown kinds generically", () => {
    expect(kindLabelKey("production.script")).toBe("activity.kind.production.script");
    expect(kindLabelKey("weird")).toBe("activity.kind.other");
  });

  it("treats queued and running as active", () => {
    expect(isActive(job({ status: "queued" }))).toBe(true);
    expect(isActive(job({ status: "failed" }))).toBe(false);
  });
});
