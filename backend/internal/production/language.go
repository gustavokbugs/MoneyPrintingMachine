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

// languageName names a channel language for prompts.
func languageName(tag string) string {
	if l, _ := speechLang(tag); l == langPortugueseBR {
		return "Brazilian Portuguese (português do Brasil)"
	}
	if t := strings.TrimSpace(tag); t != "" && !strings.EqualFold(t, "en-US") {
		return "the language with BCP 47 tag " + t
	}
	return "American English"
}

// englishMarkers are frequent English words that are not Portuguese words.
var englishMarkers = map[string]bool{"the": true, "you": true, "your": true, "and": true, "but": true, "now": true,
	"with": true, "it's": true, "is": true, "are": true, "this": true, "that": true, "just": true, "so": true}

// wrongLanguage reports whether a script for a Portuguese channel came
// back mostly in English (more than 1 in 12 words is an English marker).
func wrongLanguage(sc *Script, tag string) bool {
	if l, _ := speechLang(tag); l != langPortugueseBR {
		return false
	}
	words, english := 0, 0
	for _, seg := range sc.Segments() {
		for _, w := range strings.Fields(strings.ToLower(seg.Narration)) {
			words++
			if englishMarkers[strings.Trim(w, ".,!?;:\"()")] {
				english++
			}
		}
	}
	return words > 0 && english*12 > words
}
