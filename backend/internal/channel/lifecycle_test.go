package channel

import (
	"errors"
	"testing"
)

func readyFixture(status Status) *Channel {
	return &Channel{
		OwnerID: "owner", Status: status, Name: "Deep Orbit", Niche: "Space",
		Language: DefaultLanguage, GeoTarget: DefaultGeoTarget,
		Strategy: &Strategy{}, Identity: &Identity{}, Blueprint: &Blueprint{},
	}
}

func TestTransitionTo(t *testing.T) {
	verified := Prerequisites{VerifiedPlatformConnections: 1}
	tests := []struct {
		name    string
		ch      func() *Channel
		to      Status
		reason  string
		pre     Prerequisites
		wantErr bool
	}{
		{"draft to branding with strategy", func() *Channel { return readyFixture(StatusDraft) }, StatusBranding, "", Prerequisites{}, false},
		{"draft to branding without strategy", func() *Channel {
			c := readyFixture(StatusDraft)
			c.Strategy = nil
			return c
		}, StatusBranding, "", Prerequisites{}, true},
		{"draft to branding without niche", func() *Channel {
			c := readyFixture(StatusDraft)
			c.Niche = ""
			return c
		}, StatusBranding, "", Prerequisites{}, true},
		{"branding to platform setup needs name", func() *Channel {
			c := readyFixture(StatusBranding)
			c.Name = ""
			return c
		}, StatusAwaitingPlatformSetup, "", Prerequisites{}, true},
		{"branding to platform setup", func() *Channel { return readyFixture(StatusBranding) }, StatusAwaitingPlatformSetup, "", Prerequisites{}, false},
		{"ready requires verified connection", func() *Channel { return readyFixture(StatusAwaitingPlatformSetup) }, StatusReady, "", Prerequisites{}, true},
		{"ready with verified connection", func() *Channel { return readyFixture(StatusAwaitingPlatformSetup) }, StatusReady, "", verified, false},
		{"ready requires blueprint", func() *Channel {
			c := readyFixture(StatusAwaitingPlatformSetup)
			c.Blueprint = nil
			return c
		}, StatusReady, "", verified, true},
		{"draft cannot jump to active", func() *Channel { return readyFixture(StatusDraft) }, StatusActive, "", verified, true},
		{"active to paused", func() *Channel { return readyFixture(StatusActive) }, StatusPaused, "", Prerequisites{}, false},
		{"paused to active needs connection", func() *Channel { return readyFixture(StatusPaused) }, StatusActive, "", Prerequisites{}, true},
		{"error requires reason", func() *Channel { return readyFixture(StatusActive) }, StatusError, "", Prerequisites{}, true},
		{"error with reason", func() *Channel { return readyFixture(StatusActive) }, StatusError, "upload token revoked", Prerequisites{}, false},
		{"same state rejected", func() *Channel { return readyFixture(StatusPaused) }, StatusPaused, "", Prerequisites{}, true},
		{"archived can only restore to draft", func() *Channel { return readyFixture(StatusArchived) }, StatusActive, "", verified, true},
		{"archived restore", func() *Channel { return readyFixture(StatusArchived) }, StatusDraft, "", Prerequisites{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.ch()
			from := c.Status
			ev, err := c.TransitionTo(tt.to, tt.reason, tt.pre)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				if c.Status != from {
					t.Fatalf("status changed on failed transition: %s", c.Status)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.Status != tt.to || ev.From != from || ev.To != tt.to {
				t.Fatalf("unexpected result: status=%s event=%+v", c.Status, ev)
			}
		})
	}
}

func TestEveryStatusHasTransitions(t *testing.T) {
	for _, s := range AllStatuses {
		if len(AllowedTransitions(s)) == 0 {
			t.Errorf("status %s has no outgoing transitions", s)
		}
	}
}

func TestParseStatus(t *testing.T) {
	if _, err := ParseStatus("active"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var ve *ValidationError
	if _, err := ParseStatus("deleted"); !errors.As(err, &ve) {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestNewDraftDefaultsAndValidation(t *testing.T) {
	c, err := NewDraft("owner", " History ", "", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Language != "en-US" || c.GeoTarget != "US" || c.Niche != "History" || c.Status != StatusDraft {
		t.Fatalf("unexpected defaults: %+v", c)
	}
	for _, tc := range []struct{ lang, geo string }{{"english", "US"}, {"en-US", "USA"}, {"en_us", "US"}} {
		if _, err := NewDraft("owner", "x", tc.lang, tc.geo); err == nil {
			t.Errorf("expected error for lang=%q geo=%q", tc.lang, tc.geo)
		}
	}
	if _, err := NewDraft("", "x", "", ""); err == nil {
		t.Error("expected error for missing owner")
	}
}

func TestApplySettingsRejectsArchived(t *testing.T) {
	c := readyFixture(StatusArchived)
	name := "New"
	if err := c.ApplySettings(SettingsUpdate{Name: &name}); err == nil {
		t.Fatal("expected error editing archived channel")
	}
	if c.Name != "Deep Orbit" {
		t.Fatalf("name changed: %s", c.Name)
	}
}

func TestPaletteValidate(t *testing.T) {
	ok := Palette{"#112233", "#AABBCC", "#abcdef", "#000000", "#FFFFFF"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	bad := ok
	bad.Accent = "red"
	if err := bad.Validate(); err == nil {
		t.Fatal("expected error for non-hex color")
	}
}

func TestPublishingScheduleValidate(t *testing.T) {
	ok := PublishingSchedule{VideosPerWeek: 2, Days: []string{"Tuesday", "Friday"}, LocalTime: "17:00", TimeZone: "America/New_York"}
	if err := ok.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, mut := range []func(*PublishingSchedule){
		func(p *PublishingSchedule) { p.Days = []string{"Funday"} },
		func(p *PublishingSchedule) { p.LocalTime = "25:00" },
		func(p *PublishingSchedule) { p.TimeZone = "Mars/Olympus" },
		func(p *PublishingSchedule) { p.VideosPerWeek = 0 },
	} {
		p := ok
		p.Days = append([]string(nil), ok.Days...)
		mut(&p)
		if err := p.Validate(); err == nil {
			t.Errorf("expected error for %+v", p)
		}
	}
}
