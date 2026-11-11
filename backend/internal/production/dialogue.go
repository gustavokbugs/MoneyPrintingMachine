package production

import (
	"context"

	"contentfactory/internal/channel"
)

// Dialogue modes: how many voices a script uses and how they relate.
const (
	DialogueNarration = "narration"          // one narrator
	DialogueQA        = "qa"                 // a curious host asks, an expert answers
	DialogueDebate    = "debate"             // two sides argue, then a fair summary
	DialogueStory     = "narrator_character" // a narrator tells, a character speaks
)

// DefaultVoiceB is Michael, a male voice that contrasts with the default.
const DefaultVoiceB = 16

// DialogueModeInfo describes a mode and its speaker roles.
type DialogueModeInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	SpeakerA string `json:"speakerA"`
	SpeakerB string `json:"speakerB"`
	Guide    string `json:"-"`
}

// DialogueModes lists modes with speaker roles for the UI and prompts.
var DialogueModes = []DialogueModeInfo{
	{DialogueNarration, "Single narrator", "Narrator", "", ""},
	{DialogueQA, "Q&A", "Host", "Expert",
		"Speaker A is a curious host who asks short, natural questions a viewer would ask. Speaker B is the expert who answers clearly. Alternate A and B; A's turns are short, B's carry the facts."},
	{DialogueDebate, "Debate", "Pro", "Con",
		"Speaker A argues for one side and speaker B argues the other, each with real evidence and no strawmen. Alternate A and B. The outro (speaker A) gives a fair summary of both sides without declaring a winner unless the evidence is clear."},
	{DialogueStory, "Narrator and character", "Narrator", "Character",
		"Speaker A narrates the story. Speaker B is a character inside the story who speaks only in short first-person lines at key moments, never quoting a real person's words as fact. Most sections are A."},
}

// dialogueMode returns the mode definition, or ok=false.
func dialogueMode(id string) (DialogueModeInfo, bool) {
	for _, m := range DialogueModes {
		if m.ID == id {
			return m, true
		}
	}
	return DialogueModeInfo{}, false
}

// IsDialogue reports whether a mode uses two voices.
func IsDialogue(mode string) bool { return mode != "" && mode != DialogueNarration }

// voiceFor returns the voice of a section's speaker.
func (v *Video) voiceFor(speaker string) int {
	if IsDialogue(v.DialogueMode) && speaker == "B" {
		if v.VoiceB == 0 {
			return DefaultVoiceB
		}
		return v.VoiceB
	}
	return v.VoiceID
}

// SetDialogue chooses the dialogue mode and second voice. A new mode
// takes effect on the next generated script.
func (s *Service) SetDialogue(ctx context.Context, ownerID, id, mode string, voiceB int) (*Video, error) {
	if _, ok := dialogueMode(mode); !ok {
		return nil, &channel.ValidationError{Field: "dialogueMode", Message: "is not a known mode"}
	}
	if !ValidVoice(voiceB) {
		return nil, &channel.ValidationError{Field: "voiceB", Message: "is not a known voice"}
	}
	return s.mutate(ctx, ownerID, id, func(v *Video) error {
		if v.Status == StatusProducing {
			return &StateError{Status: v.Status, Action: "change the dialogue", Reason: "production is running"}
		}
		v.DialogueMode, v.VoiceB = mode, voiceB
		return nil
	})
}
