package channel

import "time"

// Provenance records how a piece of generated content was produced.
type Provenance struct {
	Generator    string    `json:"generator"` // e.g. "ollama".
	Model        string    `json:"model"`
	GeneratedAt  time.Time `json:"generatedAt"`
	EditedByUser bool      `json:"editedByUser"`
}

// Strategy is the editorial strategy of a channel.
type Strategy struct {
	TargetAudience       string             `json:"targetAudience"`
	AudienceGeography    string             `json:"audienceGeography"`
	PreferredLanguage    string             `json:"preferredLanguage"`
	Positioning          string             `json:"positioning"`
	EditorialIdentity    string             `json:"editorialIdentity"`
	ContentPillars       []string           `json:"contentPillars"`
	VideoFormats         []VideoFormat      `json:"videoFormats"`
	PublishingFrequency  PublishingSchedule `json:"publishingFrequency"`
	CompetitorReferences []string           `json:"competitorReferences"`
	CompetitorNote       string             `json:"competitorNote"`
	MonetizationOptions  []string           `json:"monetizationOptions"`
	Provenance           Provenance         `json:"provenance"`
}

// VideoFormat describes one recurring video format.
type VideoFormat struct {
	Name            string `json:"name"`
	Description     string `json:"description"`
	AspectRatio     string `json:"aspectRatio"` // "16:9" or "9:16".
	DurationSeconds int    `json:"durationSeconds"`
}

// PublishingSchedule describes how often and when to publish.
type PublishingSchedule struct {
	VideosPerWeek int      `json:"videosPerWeek"`
	Days          []string `json:"days"`      // e.g. "Tuesday".
	LocalTime     string   `json:"localTime"` // "HH:MM" in the target audience's main time zone.
	TimeZone      string   `json:"timeZone"`  // IANA name, e.g. "America/New_York".
}

// Identity is the brand identity of a channel.
type Identity struct {
	NameSuggestions      []NameSuggestion `json:"nameSuggestions"`
	ShortBio             string           `json:"shortBio"`
	FullDescription      string           `json:"fullDescription"`
	PositioningStatement string           `json:"positioningStatement"`
	Tagline              string           `json:"tagline"`
	Keywords             []string         `json:"keywords"`
	Palette              Palette          `json:"palette"`
	Typography           Typography       `json:"typography"`
	AvatarConcept        string           `json:"avatarConcept"`
	ThumbnailStyle       string           `json:"thumbnailStyle"`
	CaptionStyle         string           `json:"captionStyle"`
	MotionGuidelines     string           `json:"motionGuidelines"`
	IntroOutroGuidelines string           `json:"introOutroGuidelines"`
	Provenance           Provenance       `json:"provenance"`
}

// NameSuggestion is one candidate channel name. Availability is never
// claimed unless it was checked against a real data source.
type NameSuggestion struct {
	Name         string `json:"name"`
	Rationale    string `json:"rationale"`
	Availability string `json:"availability"` // "not_checked" until a checker is connected.
}

// Palette holds brand colors as #RRGGBB hex strings.
type Palette struct {
	Primary    string `json:"primary"`
	Secondary  string `json:"secondary"`
	Accent     string `json:"accent"`
	Background string `json:"background"`
	Text       string `json:"text"`
}

// Typography holds font recommendations. Fonts come from an allowlist of
// openly licensed families (see internal/launchpad/fonts.go).
type Typography struct {
	HeadingFont string `json:"headingFont"`
	BodyFont    string `json:"bodyFont"`
	Notes       string `json:"notes"`
}

// Blueprint is the initial content plan of a channel.
type Blueprint struct {
	PriorityTopics []string        `json:"priorityTopics"`
	Series         []Series        `json:"series"`
	Formats        []VideoFormat   `json:"formats"`
	Hooks          []string        `json:"hooks"`
	Ideas          []ContentIdea   `json:"ideas"`
	Calendar       []CalendarEntry `json:"calendar"`
	Provenance     Provenance      `json:"provenance"`
}

// Series is a recurring content series.
type Series struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Cadence     string `json:"cadence"`
}

// ContentIdea is a single video idea.
type ContentIdea struct {
	Topic                 string   `json:"topic"`
	WorkingTitle          string   `json:"workingTitle"`
	TargetAudience        string   `json:"targetAudience"`
	Angle                 string   `json:"angle"`
	WhyInteresting        string   `json:"whyInteresting"`
	ResearchSources       []string `json:"researchSources"`
	OriginalityNotes      string   `json:"originalityNotes"`
	SuggestedDurationSecs int      `json:"suggestedDurationSeconds"`
	Format                string   `json:"format"`
}

// CalendarEntry schedules one idea on a date.
type CalendarEntry struct {
	Date      string `json:"date"`      // YYYY-MM-DD.
	LocalTime string `json:"localTime"` // HH:MM.
	TimeZone  string `json:"timeZone"`
	IdeaIndex int    `json:"ideaIndex"` // Index into Blueprint.Ideas.
}
