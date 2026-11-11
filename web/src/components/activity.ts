import type { Job } from "../api/types";
import type { MessageKey } from "../i18n";

const knownKinds = [
  "launchpad.autopilot",
  "launchpad.discover_niches",
  "launchpad.generate_strategy",
  "launchpad.generate_identity",
  "launchpad.generate_blueprint",
  "production.script",
  "production.produce",
  "publishing.youtube",
] as const;

/** isActive reports whether a job is still waiting or running. */
export function isActive(j: Job): boolean {
  return j.status === "queued" || j.status === "running";
}

/** kindLabelKey returns the message key naming a job kind. */
export function kindLabelKey(kind: string): MessageKey {
  return (knownKinds as readonly string[]).includes(kind) ? (`activity.kind.${kind}` as MessageKey) : "activity.kind.other";
}

function inputVideoId(j: Job): string | null {
  const input = j.input;
  if (typeof input === "object" && input !== null && "videoId" in input) {
    const id = (input as { videoId: unknown }).videoId;
    if (typeof id === "string" && id !== "") return id;
  }
  return null;
}

/** activityLink is the page where a job's outcome can be seen. */
export function activityLink(j: Job): string {
  const video = inputVideoId(j);
  if (video) return `/videos/${encodeURIComponent(video)}`;
  if (j.kind === "launchpad.discover_niches") return "/launchpad";
  if (j.channelId) {
    const id = encodeURIComponent(j.channelId);
    switch (j.kind) {
      case "launchpad.autopilot":
        return `/channels/${id}?step=review&autopilot=${encodeURIComponent(j.id)}`;
      case "launchpad.generate_strategy":
        return `/channels/${id}?step=strategy`;
      case "launchpad.generate_identity":
        return `/channels/${id}?step=branding`;
      case "launchpad.generate_blueprint":
        return `/channels/${id}?step=blueprint`;
      default:
        return `/channels/${id}`;
    }
  }
  return "/";
}
