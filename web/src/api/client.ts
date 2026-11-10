import {
  asChannel,
  asItems,
  asJob,
  asRecordOf,
  isRecord,
} from "./guards";
import type {
  Blueprint,
  Channel,
  ChannelStatus,
  Health,
  Identity,
  Job,
  LaunchReview,
  PlatformCapability,
  PlatformConnection,
  SavedNiche,
  Strategy,
} from "./types";

/** ApiError carries the server's stable error code and message. */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
    readonly field?: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function request(method: string, path: string, body?: unknown, headers?: Record<string, string>): Promise<unknown> {
  const init: RequestInit = { method, headers: { Accept: "application/json", ...headers } };
  if (body !== undefined) {
    init.body = JSON.stringify(body);
    (init.headers as Record<string, string>)["Content-Type"] = "application/json";
  }
  let res: Response;
  try {
    res = await fetch(path, init);
  } catch {
    throw new ApiError(0, "network_error", "Cannot reach the server. Check that the backend is running.");
  }
  if (res.status === 204) return null;
  let data: unknown = null;
  const text = await res.text();
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      throw new ApiError(res.status, "invalid_response", "The server returned an invalid response.");
    }
  }
  if (!res.ok) {
    const err = isRecord(data) && isRecord(data.error) ? data.error : {};
    throw new ApiError(
      res.status,
      typeof err.code === "string" ? err.code : "http_error",
      typeof err.message === "string" ? err.message : `Request failed with status ${res.status}.`,
      typeof err.field === "string" ? err.field : undefined,
    );
  }
  return data;
}

/** newIdempotencyKey returns a key for one user action. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}

const idem = (key: string) => ({ "Idempotency-Key": key });

export const api = {
  health: async () => asRecordOf<Health>("health", ["status", "dependencies"])(await request("GET", "/api/health")),

  listChannels: async (cursor = "") => {
    const data = await request("GET", `/api/channels?limit=100${cursor ? `&cursor=${encodeURIComponent(cursor)}` : ""}`);
    const items = asItems(data, asChannel);
    const next = isRecord(data) && typeof data.nextCursor === "string" ? data.nextCursor : "";
    return { items, nextCursor: next };
  },
  getChannel: async (id: string) => asChannel(await request("GET", `/api/channels/${encodeURIComponent(id)}`)),
  createChannel: async (input: { niche: string; language?: string; geoTarget?: string }) =>
    asChannel(await request("POST", "/api/channels", input)),
  updateChannel: async (id: string, input: Partial<Pick<Channel, "name" | "niche" | "language" | "geoTarget">>) =>
    asChannel(await request("PATCH", `/api/channels/${encodeURIComponent(id)}`, input)),
  changeStatus: async (id: string, status: ChannelStatus, reason = "") =>
    asChannel(await request("POST", `/api/channels/${encodeURIComponent(id)}/status`, { status, reason })),
  bulkStatus: async (ids: string[], status: "paused" | "active") =>
    asRecordOf<{ results: { id: string; ok: boolean; status?: ChannelStatus; error?: string }[] }>("bulk result", ["results"])(
      await request("POST", "/api/channels/bulk/status", { ids, status, reason: "" }),
    ),
  putStrategy: async (id: string, s: Strategy) =>
    asChannel(await request("PUT", `/api/channels/${encodeURIComponent(id)}/strategy`, s)),
  putIdentity: async (id: string, i: Identity) =>
    asChannel(await request("PUT", `/api/channels/${encodeURIComponent(id)}/identity`, i)),
  putBlueprint: async (id: string, b: Blueprint) =>
    asChannel(await request("PUT", `/api/channels/${encodeURIComponent(id)}/blueprint`, b)),
  generate: async (id: string, step: "strategy" | "identity" | "blueprint", key: string) =>
    asJob(await request("POST", `/api/channels/${encodeURIComponent(id)}/${step}/generate`, undefined, idem(key))),
  connections: async (id: string) =>
    asItems(
      await request("GET", `/api/channels/${encodeURIComponent(id)}/connections`),
      asRecordOf<PlatformConnection>("connection", ["platform", "status"]),
    ),
  launchReview: async (id: string) =>
    asRecordOf<LaunchReview>("launch review", ["channel", "checks", "readyToLaunch"])(
      await request("GET", `/api/channels/${encodeURIComponent(id)}/launch-review`),
    ),
  assetUrl: (id: string, asset: "avatar" | "banner", variant: number, version: number) =>
    `/api/channels/${encodeURIComponent(id)}/assets/${asset}.svg?variant=${variant}&v=${version}`,

  discoverNiches: async (query: string, key: string) =>
    asJob(await request("POST", "/api/launchpad/niches/discover", { query }, idem(key))),
  listSavedNiches: async () =>
    asItems(await request("GET", "/api/launchpad/saved-niches"), asRecordOf<SavedNiche>("saved niche", ["id", "name"])),
  saveNiche: async (name: string, notes: string, data: unknown) =>
    asRecordOf<SavedNiche>("saved niche", ["id", "name"])(
      await request("POST", "/api/launchpad/saved-niches", { name, notes, data }),
    ),
  deleteSavedNiche: async (id: string) => {
    await request("DELETE", `/api/launchpad/saved-niches/${encodeURIComponent(id)}`);
  },

  platforms: async () =>
    asItems(await request("GET", "/api/platforms"), asRecordOf<PlatformCapability>("platform", ["platform", "steps"])),
  getJob: async (id: string): Promise<Job> => asJob(await request("GET", `/api/jobs/${encodeURIComponent(id)}`)),
};
