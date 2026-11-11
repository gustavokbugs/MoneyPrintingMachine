import { type VisualStyle, visualStyles } from "./types";
// Runtime checks for API responses. Responses are untrusted until their
// essential shape is confirmed; deeper fields are validated by the server.

import type { Channel, ChannelStatus, Job, NicheReport } from "./types";

export class ResponseShapeError extends Error {
  constructor(what: string) {
    super(`Unexpected response shape: ${what}`);
    this.name = "ResponseShapeError";
  }
}

export function isRecord(v: unknown): v is Record<string, unknown> {
  return typeof v === "object" && v !== null && !Array.isArray(v);
}

const statuses: readonly ChannelStatus[] = [
  "draft",
  "branding",
  "awaiting_platform_setup",
  "ready",
  "active",
  "paused",
  "error",
  "archived",
];

export function isChannelStatus(v: unknown): v is ChannelStatus {
  return typeof v === "string" && (statuses as readonly string[]).includes(v);
}

export function asChannel(v: unknown): Channel {
  if (
    !isRecord(v) ||
    typeof v.id !== "string" ||
    typeof v.name !== "string" ||
    !isChannelStatus(v.status) ||
    !Array.isArray(v.allowedTransitions) ||
    typeof v.version !== "number"
  ) {
    throw new ResponseShapeError("channel");
  }
  return v as unknown as Channel;
}

export function asJob(v: unknown): Job {
  if (
    !isRecord(v) ||
    typeof v.id !== "string" ||
    typeof v.kind !== "string" ||
    !["queued", "running", "succeeded", "failed"].includes(String(v.status))
  ) {
    throw new ResponseShapeError("job");
  }
  return v as unknown as Job;
}

export function asNicheReport(v: unknown): NicheReport {
  if (!isRecord(v) || !Array.isArray(v.niches) || !Array.isArray(v.dataSources)) {
    throw new ResponseShapeError("niche report");
  }
  for (const n of v.niches) {
    if (!isRecord(n) || typeof n.name !== "string" || typeof n.heuristicScore !== "number") {
      throw new ResponseShapeError("niche");
    }
  }
  return v as unknown as NicheReport;
}

export function asItems<T>(v: unknown, check: (item: unknown) => T): T[] {
  if (!isRecord(v) || !Array.isArray(v.items)) {
    throw new ResponseShapeError("item list");
  }
  return v.items.map(check);
}

export function asRecordOf<T>(what: string, required: string[]) {
  return (v: unknown): T => {
    if (!isRecord(v) || required.some((k) => !(k in v))) {
      throw new ResponseShapeError(what);
    }
    return v as T;
  };
}

/**
 * asVisualStyles reads the server's style list ({id, name, description}
 * objects, or plain ids) and keeps only ids this client knows how to label.
 */
export function asVisualStyles(v: unknown): VisualStyle[] {
  if (!Array.isArray(v)) return ["auto"];
  const out: VisualStyle[] = [];
  for (const item of v) {
    const id: unknown = isRecord(item) ? item.id : item;
    const known = visualStyles.find((s) => s === id);
    if (known && !out.includes(known)) out.push(known);
  }
  return out.length > 0 ? out : ["auto"];
}

export interface AutopilotResult {
  channelId: string;
  generated: string[];
  notes: string[];
}

/** asAutopilotResult validates the result of an autopilot job. */
export function asAutopilotResult(v: unknown): AutopilotResult {
  if (!isRecord(v) || typeof v.channelId !== "string" || !Array.isArray(v.generated)) {
    throw new ResponseShapeError("autopilot result");
  }
  const strings = (a: unknown) => (Array.isArray(a) ? a.filter((x): x is string => typeof x === "string") : []);
  return { channelId: v.channelId, generated: strings(v.generated), notes: strings(v.notes) };
}
