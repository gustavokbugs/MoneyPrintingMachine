package production

import (
	"context"

	"contentfactory/internal/channel"
)

// Story formats: how a script tells its story.
const (
	FormatExplainer  = "explainer"  // calm, sourced explainer (the original style)
	FormatEscalation = "escalation" // absurd "you did X... congrats, you built Y"
	FormatPOV        = "pov"        // second-person story with dialogue lines
	FormatFixChain   = "fix_chain"  // each fix causes the next problem
)

// ShortFormatSeconds is the default length of the short viral formats.
const ShortFormatSeconds = 60

// StoryFormat describes one format for the UI and the script prompt.
type StoryFormat struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Example string `json:"example"`
	// Seconds is the length the format resets a video to (0 keeps it).
	Seconds int `json:"seconds"`
	// Pace is the narration speed (0 means normal); short formats talk
	// fast, like popular Shorts (about 3 words per second).
	Pace float64 `json:"-"`
	// NoMusic drops the music bed so word sound effects carry the mood.
	NoMusic bool   `json:"-"`
	Guide   string `json:"-"`
}

// StoryFormats lists the formats, explainer first.
var StoryFormats = []StoryFormat{
	{ID: FormatExplainer, Name: "Explainer", Example: "A calm, sourced story about the topic.", Seconds: 0},
	{ID: FormatEscalation, Name: "Absurd escalation", Seconds: ShortFormatSeconds, Pace: ShortPace,
		Example: "You leave your lunch in your bag for a month... congrats, you just discovered antibiotics.",
		Guide: `Format: absurd escalation. Talk to the viewer as "you". Start with an ordinary, harmless action. Each section escalates by one step, with deadpan, unhinged humor ("congrats", "at this point", "you are now legally a..."), until the last step lands on a surprising real outcome tied to the topic. The hook is the punchline teased in one line ("If you do this, you accidentally ___."). Every step stays absurd or harmless: never give real, usable instructions for anything dangerous (weapons, explosives, toxic mixtures, hacking). The real facts the joke rests on must be true and listed in claims. Each section is ONE new step, never a repeat of the last one.
Example beats on another topic (copy the rhythm, not the content): Hook: "If you skip the dishes before vacation, you accidentally save millions of lives." / "You leave a dirty plate in the lab. Classic." / "You go on vacation. The plate does not." / "You come back. Something blue and fuzzy is living on it." / "Congrats, it is killing the bacteria around it." / "You just found penicillin. That was Alexander Fleming, 1928." / Outro: "Do your dishes. Or don't. Follow for more."`},
	{ID: FormatPOV, Name: "POV story", Seconds: ShortFormatSeconds, Pace: ShortPace, NoMusic: true,
		Example: "Your girlfriend is in the shower. Steam pours out of the walls. You ask: \"Are you cooking yourself?\"",
		Guide: `Format: POV story. Narrate in second person, present tense ("You walk in. You see..."), like a chaotic short film about the viewer's life. Use quick, vivid beats and short spoken lines from the other character in quotes. Build to an absurd twist, then reveal the real fact or idea from the topic in one line near the end. Keep it playful, never cruel, sexual, or demeaning. Each section is ONE new beat.
Example beats on another topic (copy the rhythm, not the content): "You walk into the kitchen at 3 a.m." / "Your roommate is staring into the fridge." / "'It's humming at me,' she whispers." / "You open it. It's 4 degrees colder than the room. Every night." / "'It's not haunted,' you say. 'It's a heat pump.'" / "It moves heat out of the box and onto your floor."`},
	{ID: FormatFixChain, Name: "Problem-fix chain", Seconds: ShortFormatSeconds, Pace: ShortPace,
		Example: "You poke a pipe through the wall: ventilation. But now the heat escapes, so you build...",
		Guide: `Format: problem-fix chain. Talk to the viewer as "you". Start from one simple act that solves a problem. Every section names the new problem the last fix created ("but now..."), then the next fix, in a fast chain, until the viewer has reinvented a real invention from the topic and the last line names it. The engineering logic must be roughly real, with deadpan humor between steps. Each section is ONE new step, never a repeat.
Example beats on another topic (copy the rhythm, not the content): "You're cold. You light a fire inside. Cozy." / "But now the room is full of smoke." / "So you poke a hole in the roof. Ventilation." / "But now the rain comes in." / "So you put a little hat on the hole." / "Congrats, you just invented the chimney."`},
}

// ShortPace is the narration speed of the short formats.
const ShortPace = 1.1

// paceOf returns the narration speed for a video's format (1 = normal).
func paceOf(format string) float64 {
	if f, ok := storyFormat(format); ok && f.Pace > 0 {
		return f.Pace
	}
	return 1
}

// musicOff reports whether a format plays without music.
func musicOff(format string) bool {
	f, ok := storyFormat(format)
	return ok && f.NoMusic
}

// storyFormat returns the format definition, or ok=false.
func storyFormat(id string) (StoryFormat, bool) {
	for _, f := range StoryFormats {
		if f.ID == id {
			return f, true
		}
	}
	return StoryFormat{}, false
}

// SetStoryFormat chooses how the next script is told. Short formats also
// reset the target length, which sizes the script, and make the video
// vertical, since they are made for Shorts, Reels, and TikTok.
func (s *Service) SetStoryFormat(ctx context.Context, ownerID, id, format string) (*Video, error) {
	f, ok := storyFormat(format)
	if !ok {
		return nil, &channel.ValidationError{Field: "storyFormat", Message: "is not a known format"}
	}
	return s.mutate(ctx, ownerID, id, func(v *Video) error {
		if v.Status == StatusProducing {
			return &StateError{Status: v.Status, Action: "change the story format", Reason: "production is running"}
		}
		v.StoryFormat = f.ID
		if f.Seconds > 0 {
			v.TargetSeconds = f.Seconds
			v.AspectRatio = "9:16"
		}
		return nil
	})
}
