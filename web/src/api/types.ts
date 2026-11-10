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

export interface NameSuggestion {
  name: string;
  rationale: string;
  availability: string;
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
    competitionScore: number | null;
    searchInterest: number | null;
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
