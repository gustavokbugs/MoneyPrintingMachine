// API contract types. Keep in sync with backend/internal/httpapi and
// backend/internal/channel.

export type ChannelStatus =
  | "draft"
  | "branding"
  | "awaiting_platform_setup"
  | "ready"
  | "active"
  | "paused"
  | "error"
  | "archived";

export interface Provenance {
  generator: string;
  model: string;
  generatedAt: string;
  editedByUser: boolean;
}

export interface VideoFormat {
  name: string;
  description: string;
  aspectRatio: "16:9" | "9:16";
  durationSeconds: number;
}

export interface PublishingSchedule {
  videosPerWeek: number;
  days: string[];
  localTime: string;
  timeZone: string;
}

export interface Strategy {
  targetAudience: string;
  audienceGeography: string;
  preferredLanguage: string;
  positioning: string;
  editorialIdentity: string;
  contentPillars: string[];
  videoFormats: VideoFormat[];
  publishingFrequency: PublishingSchedule;
  competitorReferences: string[];
  competitorNote: string;
  monetizationOptions: string[];
  provenance: Provenance;
}

export type NameAvailability = "not_checked" | "handle_taken" | "no_channel_with_handle" | "invalid_handle";

export interface NameSuggestion {
  name: string;
  rationale: string;
  availability: NameAvailability | "";
  handle?: string;
  checkedAt?: string;
}

export interface Palette {
  primary: string;
  secondary: string;
  accent: string;
  background: string;
  text: string;
}

export interface Identity {
  nameSuggestions: NameSuggestion[];
  shortBio: string;
  fullDescription: string;
  positioningStatement: string;
  tagline: string;
  keywords: string[];
  palette: Palette;
  typography: { headingFont: string; bodyFont: string; notes: string };
  avatarConcept: string;
  thumbnailStyle: string;
  captionStyle: string;
  motionGuidelines: string;
  introOutroGuidelines: string;
  provenance: Provenance;
}

export interface ContentIdea {
  topic: string;
  workingTitle: string;
  targetAudience: string;
  angle: string;
  whyInteresting: string;
  researchSources: string[];
  originalityNotes: string;
  suggestedDurationSeconds: number;
  format: string;
}

export interface Blueprint {
  priorityTopics: string[];
  series: { name: string; description: string; cadence: string }[];
  formats: VideoFormat[];
  hooks: string[];
  ideas: ContentIdea[];
  calendar: { date: string; localTime: string; timeZone: string; ideaIndex: number }[];
  provenance: Provenance;
}

export interface Channel {
  id: string;
  name: string;
  status: ChannelStatus;
  statusReason: string;
  allowedTransitions: ChannelStatus[];
  niche: string;
  language: string;
  geoTarget: string;
  strategy: Strategy | null;
  identity: Identity | null;
  blueprint: Blueprint | null;
  /** Present only in list responses; omitted means no verified platform. */
  connectedPlatforms?: string[];
  version: number;
  createdAt: string;
  updatedAt: string;
}

export type JobStatus = "queued" | "running" | "succeeded" | "failed";

export interface Job {
  id: string;
  kind: string;
  channelId?: string;
  status: JobStatus;
  input?: unknown;
  result?: unknown;
  error?: string;
  attempts: number;
  maxAttempts: number;
  createdAt: string;
  updatedAt: string;
}

export type Rating = "low" | "medium" | "high";

export interface Niche {
  name: string;
  description: string;
  subNiches: string[];
  contentOpportunities: string[];
  suggestedFormats: string[];
  rationale: string;
  heuristics: {
    audiencePotential: Rating;
    monetizationPotential: Rating;
    competitionEstimate: Rating;
    productionDifficulty: Rating;
    sourceAvailability: Rating;
    originalityPotential: Rating;
  };
  heuristicScore: number;
  measured: {
    trendScore: number | null;
    momentumPercent: number | null;
    demandScore: number | null;
    competitionScore: number | null;
    searchInterest: number | null;
    metrics: TrendMetrics | null;
    collectedAt: string | null;
    news: MentionStats | null;
    sources: string[];
  };
}

export interface DataSourceStatus {
  name: string;
  status: "connected" | "not_configured" | "unavailable";
  note: string;
}

export interface NicheReport {
  query: string;
  generatedAt: string;
  model: string;
  basis: string;
  niches: Niche[];
  dataSources: DataSourceStatus[];
}

export interface SavedNiche {
  id: string;
  name: string;
  notes: string;
  data: unknown;
  createdAt: string;
}

export interface PlatformStep {
  id: string;
  title: string;
  description: string;
  automation: "automated" | "user_action" | "verified";
  url?: string;
}

export interface PlatformCapability {
  platform: string;
  displayName: string;
  accountCreation: string;
  oauthScopes: string[];
  limits: string[];
  steps: PlatformStep[];
  integrationReady: boolean;
  lastReviewed: string;
}

export interface PlatformConnection {
  platform: string;
  status: string;
  externalId?: string;
  externalName?: string;
  canPublish: boolean;
  canReadAnalytics: boolean;
  statusReason?: string;
  updatedAt: string;
}

export interface LaunchReview {
  channel: Channel;
  connections: PlatformConnection[];
  checks: { id: string; label: string; ok: boolean; detail: string }[];
  readyToLaunch: boolean;
}

export interface Health {
  status: "ok" | "degraded";
  model: string;
  dependencies: { name: string; status: string; error?: string }[];
}

export interface TrendMetrics {
  sampleSize: number;
  windowDays: number;
  medianViews: number;
  medianViewsPerDay: number;
  medianEngagementRate: number;
  distinctChannels: number;
  topChannelShare: number;
  medianSubscribers: number | null;
  hiddenSubscriberChannels: number;
}

export interface TrendSnapshot {
  id: number;
  source: string;
  query: string;
  region: string;
  language: string;
  metrics: TrendMetrics;
  collectedAt: string;
}

export interface TrendResult {
  snapshot: TrendSnapshot;
  scores: {
    demand: number | null;
    competition: number | null;
    momentumPercent: number | null;
    trend: number | null;
  };
  previous: TrendSnapshot | null;
  cached: boolean;
  notes: string[];
}

export interface Integrations {
  youtubeDataApi: { configured: boolean; quota: { usedToday: number; limit: number } | null };
  youtubeOAuth: { configured: boolean };
}

export interface MentionStats {
  recent7d: number;
  previous7d: number;
  momentumPercent: number | null;
  activeFeeds: number;
}

export interface Watch {
  id: number;
  query: string;
  queryKey: string;
  region: string;
  language: string;
  source: "channel" | "manual";
  active: boolean;
  nextDueAt: string;
  lastMeasuredAt: string | null;
  lastError: string;
  createdAt: string;
}

export interface TrendPoint {
  collectedAt: string;
  medianViewsPerDay: number;
  sampleSize: number;
  scores: { demand: number | null; competition: number | null; momentumPercent: number | null; trend: number | null };
}

export type FeedStatus = "pending" | "ok" | "not_modified" | "error";

export interface Feed {
  id: number;
  url: string;
  title: string;
  active: boolean;
  nextFetchAt: string;
  lastFetchedAt: string | null;
  lastStatus: FeedStatus;
  lastError: string;
  itemCount: number;
  createdAt: string;
}

export interface RefreshResult {
  measured: number;
  failed: number;
  stopped?: string;
  feedsFetched: number;
  feedsFailed: number;
  itemsPruned: number;
}

export type VideoStatus = "planned" | "scripting" | "script_ready" | "approved" | "producing" | "produced" | "failed";
export type ClaimStatus = "unverified" | "verified" | "removed";

export interface ScriptSection {
  heading: string;
  narration: string;
  onScreenText: string;
  /** "A" or "B" in two-voice scripts. */
  speaker?: string;
}

export interface Claim {
  text: string;
  sourceHint: string;
  status: ClaimStatus;
  note: string;
}

export interface VideoScript {
  hook: string;
  sections: ScriptSection[];
  outro: string;
  claims: Claim[];
  estimatedSeconds: number;
  provenance: Provenance;
}

export interface VideoOutput {
  videoFile: string;
  captionsFile: string;
  durationSeconds: number;
  width: number;
  height: number;
  renderedAt: string;
  syntheticVoice: boolean;
  /** "animated:<style>" or "solid". */
  visuals?: string;
  visualsNote?: string;
  /** Provenance of the soundtrack (generated in code). */
  music?: string;
  soundEffects?: number;
  footage?: FootageCredit[];
}

export interface Video {
  id: string;
  channelId: string;
  ideaIndex: number;
  title: string;
  format: string;
  aspectRatio: "16:9" | "9:16";
  targetSeconds: number;
  status: VideoStatus;
  statusReason: string;
  voiceId: number;
  visualStyle: VisualStyle;
  dialogueMode: string;
  voiceB: number;
  /** How the script tells the story (explainer or a short viral format). */
  storyFormat: string;
  script: VideoScript | null;
  output: VideoOutput | null;
  version: number;
  createdAt: string;
  updatedAt: string;
}

export interface ProductionStatus {
  ready: boolean;
  missing: string[];
  voices: { id: number; name: string }[];
  styles: VisualStyle[];
  animatedScenes: boolean;
  stockFootage: boolean;
  dialogueModes: DialogueModeInfo[];
  storyFormats: StoryFormatInfo[];
}

/** StoryFormatInfo is one way to tell a video's story. */
export interface StoryFormatInfo {
  id: string;
  name: string;
  example: string;
  /** Length in seconds the format sets (0 keeps the current one). */
  seconds: number;
}

/** DialogueModeInfo is one script format and its speaker roles. */
export interface DialogueModeInfo {
  id: string;
  name: string;
  speakerA: string;
  speakerB: string;
}

/** Visual styles the scene renderer supports; keep in sync with production.Styles. */
export const visualStyles = ["auto", "minimal", "documentary", "bold", "tech", "gameplay", "footage", "build3d"] as const;
export type VisualStyle = (typeof visualStyles)[number];

export type Privacy = "private" | "unlisted" | "public";

export interface Publication {
  id: string;
  videoId: string;
  platform: string;
  status: "uploading" | "published" | "failed";
  privacy: Privacy;
  externalId: string;
  url: string;
  error: string;
  createdAt: string;
  updatedAt: string;
}

/** FootageCredit credits one stock clip used in a render. */
export interface FootageCredit {
  id: number;
  pageUrl: string;
  author: string;
  authorUrl: string;
  provider?: string;
  license?: string;
  title?: string;
}
