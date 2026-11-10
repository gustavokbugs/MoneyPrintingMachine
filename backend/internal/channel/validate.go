package channel

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	hexColorRe  = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
	localTimeRe = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
	weekdays    = map[string]bool{
		"Monday": true, "Tuesday": true, "Wednesday": true, "Thursday": true,
		"Friday": true, "Saturday": true, "Sunday": true,
	}
)

const (
	maxShortText = 300
	maxLongText  = 5000
	maxListItems = 50
)

func checkText(field, v string, max int, required bool) error {
	if required && strings.TrimSpace(v) == "" {
		return &ValidationError{Field: field, Message: "is required"}
	}
	if len(v) > max {
		return &ValidationError{Field: field, Message: fmt.Sprintf("must be at most %d characters", max)}
	}
	return nil
}

func checkList(field string, items []string, min, maxItems, maxLen int) error {
	if len(items) < min {
		return &ValidationError{Field: field, Message: fmt.Sprintf("must contain at least %d items", min)}
	}
	if len(items) > maxItems {
		return &ValidationError{Field: field, Message: fmt.Sprintf("must contain at most %d items", maxItems)}
	}
	for i, it := range items {
		if err := checkText(fmt.Sprintf("%s[%d]", field, i), it, maxLen, true); err != nil {
			return err
		}
	}
	return nil
}

// Validate checks a video format.
func (f VideoFormat) Validate(field string) error {
	if err := checkText(field+".name", f.Name, maxShortText, true); err != nil {
		return err
	}
	if err := checkText(field+".description", f.Description, maxLongText, false); err != nil {
		return err
	}
	if f.AspectRatio != "16:9" && f.AspectRatio != "9:16" {
		return &ValidationError{Field: field + ".aspectRatio", Message: "must be 16:9 or 9:16"}
	}
	if f.DurationSeconds < 5 || f.DurationSeconds > 4*3600 {
		return &ValidationError{Field: field + ".durationSeconds", Message: "must be between 5 and 14400"}
	}
	return nil
}

// Validate checks a publishing schedule.
func (p PublishingSchedule) Validate() error {
	if p.VideosPerWeek < 1 || p.VideosPerWeek > 21 {
		return &ValidationError{Field: "publishingFrequency.videosPerWeek", Message: "must be between 1 and 21"}
	}
	if len(p.Days) == 0 || len(p.Days) > 7 {
		return &ValidationError{Field: "publishingFrequency.days", Message: "must contain between 1 and 7 weekdays"}
	}
	for _, d := range p.Days {
		if !weekdays[d] {
			return &ValidationError{Field: "publishingFrequency.days", Message: fmt.Sprintf("unknown weekday %q", d)}
		}
	}
	if !localTimeRe.MatchString(p.LocalTime) {
		return &ValidationError{Field: "publishingFrequency.localTime", Message: "must be HH:MM"}
	}
	if _, err := time.LoadLocation(p.TimeZone); err != nil || p.TimeZone == "" {
		return &ValidationError{Field: "publishingFrequency.timeZone", Message: "must be an IANA time zone"}
	}
	return nil
}

// Validate checks a strategy.
func (s Strategy) Validate() error {
	for _, f := range []struct {
		name string
		v    string
	}{
		{"targetAudience", s.TargetAudience}, {"audienceGeography", s.AudienceGeography},
		{"positioning", s.Positioning}, {"editorialIdentity", s.EditorialIdentity},
	} {
		if err := checkText(f.name, f.v, maxLongText, true); err != nil {
			return err
		}
	}
	if err := checkList("contentPillars", s.ContentPillars, 1, 12, maxShortText); err != nil {
		return err
	}
	if len(s.VideoFormats) < 1 || len(s.VideoFormats) > 10 {
		return &ValidationError{Field: "videoFormats", Message: "must contain between 1 and 10 formats"}
	}
	for i, f := range s.VideoFormats {
		if err := f.Validate(fmt.Sprintf("videoFormats[%d]", i)); err != nil {
			return err
		}
	}
	if err := s.PublishingFrequency.Validate(); err != nil {
		return err
	}
	if err := checkList("competitorReferences", s.CompetitorReferences, 0, 20, maxShortText); err != nil {
		return err
	}
	return checkList("monetizationOptions", s.MonetizationOptions, 0, 20, maxShortText)
}

// Validate checks a palette.
func (p Palette) Validate() error {
	for name, v := range map[string]string{
		"primary": p.Primary, "secondary": p.Secondary, "accent": p.Accent,
		"background": p.Background, "text": p.Text,
	} {
		if !hexColorRe.MatchString(v) {
			return &ValidationError{Field: "palette." + name, Message: "must be a #RRGGBB color"}
		}
	}
	return nil
}

// Validate checks an identity.
func (i Identity) Validate() error {
	if len(i.NameSuggestions) == 0 || len(i.NameSuggestions) > maxListItems {
		return &ValidationError{Field: "nameSuggestions", Message: "must contain between 1 and 50 names"}
	}
	for idx, n := range i.NameSuggestions {
		if err := checkText(fmt.Sprintf("nameSuggestions[%d].name", idx), n.Name, MaxNameLength, true); err != nil {
			return err
		}
		switch n.Availability {
		case "", AvailabilityNotChecked, AvailabilityHandleTaken, AvailabilityNoChannel, AvailabilityInvalidHandle:
		default:
			return &ValidationError{Field: fmt.Sprintf("nameSuggestions[%d].availability", idx), Message: "is not a known value"}
		}
	}
	if err := checkText("shortBio", i.ShortBio, maxShortText, true); err != nil {
		return err
	}
	if err := checkText("fullDescription", i.FullDescription, maxLongText, true); err != nil {
		return err
	}
	for name, v := range map[string]string{
		"positioningStatement": i.PositioningStatement, "tagline": i.Tagline,
		"avatarConcept": i.AvatarConcept, "thumbnailStyle": i.ThumbnailStyle,
		"captionStyle": i.CaptionStyle, "motionGuidelines": i.MotionGuidelines,
		"introOutroGuidelines": i.IntroOutroGuidelines,
	} {
		if err := checkText(name, v, maxLongText, false); err != nil {
			return err
		}
	}
	if err := checkList("keywords", i.Keywords, 0, 30, 60); err != nil {
		return err
	}
	return i.Palette.Validate()
}

// Validate checks a blueprint.
func (b Blueprint) Validate() error {
	if len(b.Ideas) == 0 || len(b.Ideas) > 100 {
		return &ValidationError{Field: "ideas", Message: "must contain between 1 and 100 ideas"}
	}
	for idx, it := range b.Ideas {
		f := fmt.Sprintf("ideas[%d]", idx)
		if err := checkText(f+".workingTitle", it.WorkingTitle, maxShortText, true); err != nil {
			return err
		}
		if err := checkText(f+".topic", it.Topic, maxShortText, true); err != nil {
			return err
		}
	}
	for idx, e := range b.Calendar {
		if e.IdeaIndex < 0 || e.IdeaIndex >= len(b.Ideas) {
			return &ValidationError{Field: fmt.Sprintf("calendar[%d].ideaIndex", idx), Message: "is out of range"}
		}
		if _, err := time.Parse("2006-01-02", e.Date); err != nil {
			return &ValidationError{Field: fmt.Sprintf("calendar[%d].date", idx), Message: "must be YYYY-MM-DD"}
		}
	}
	if err := checkList("priorityTopics", b.PriorityTopics, 0, 30, maxShortText); err != nil {
		return err
	}
	return checkList("hooks", b.Hooks, 0, 50, maxShortText)
}
