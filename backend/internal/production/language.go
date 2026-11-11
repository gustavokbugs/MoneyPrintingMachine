package production

import (
	"fmt"
	"strings"
)

// Narration languages. English uses the Kokoro lexicon (with the
// pronunciation check and caption alignment); other languages are
// phonemized by espeak-ng through the Node TTS bridge (renderer/tts.mjs).
const langPortugueseBR = "pt-br"

// speechLang returns the espeak-ng voice for a channel language ("" for
// English), or ok=false when narration does not support the language.
func speechLang(channelLang string) (string, bool) {
	l := strings.ToLower(strings.TrimSpace(channelLang))
	switch {
	case l == "" || l == "en" || strings.HasPrefix(l, "en-"):
		return "", true
	case l == "pt" || l == "pt-br":
		return langPortugueseBR, true
	}
	return "", false
}

// UnsupportedLanguageError means the channel's language has no narration
// voice yet.
type UnsupportedLanguageError struct{ Language string }

func (e *UnsupportedLanguageError) Error() string {
	return fmt.Sprintf("narration is not available in %s yet (supported: English, Brazilian Portuguese)", e.Language)
}

// defaultVoices are the first and second voices for a channel language.
func defaultVoices(channelLang string) (int, int) {
	if l, _ := speechLang(channelLang); l == langPortugueseBR {
		return 42, 43 // Dora and Alex
	}
	return DefaultVoice, DefaultVoiceB
}

// voiceLang is the language a voice speaks ("en" or "pt-br").
func voiceLang(id int) string {
	for _, v := range Voices {
		if v.ID == id {
			return v.Lang
		}
	}
	return ""
}

// followText is the closing call to action on the gameplay story.
func followText(channelLang, channelName string) string {
	if l, _ := speechLang(channelLang); l == langPortugueseBR {
		return "Siga " + channelName + " para mais"
	}
	return "Follow " + channelName + " for more"
}
