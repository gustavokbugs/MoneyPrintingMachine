package production

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpeechLang(t *testing.T) {
	for in, want := range map[string]string{"en-US": "", "en": "", "": "", "pt-BR": "pt-br", "pt": "pt-br"} {
		if got, ok := speechLang(in); !ok || got != want {
			t.Errorf("speechLang(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := speechLang("ja-JP"); ok {
		t.Error("Japanese has no voice yet")
	}
	if a, b := defaultVoices("pt-BR"); a != 42 || b != 43 || voiceLang(a) != "pt-br" || !ValidVoice(44) {
		t.Errorf("Portuguese voices = %d, %d", a, b)
	}
	if a, b := defaultVoices("en-US"); a != DefaultVoice || b != DefaultVoiceB {
		t.Errorf("English voices = %d, %d", a, b)
	}
	if got := followText("pt-BR", "ObraFeita"); got != "Siga ObraFeita para mais" {
		t.Errorf("follow = %q", got)
	}
	if got := followText("en-US", "SciWise"); got != "Follow SciWise for more" {
		t.Errorf("follow = %q", got)
	}
	var ue *UnsupportedLanguageError
	if !errors.As(error(&UnsupportedLanguageError{Language: "ja-JP"}), &ue) || !strings.Contains(ue.Error(), "ja-JP") {
		t.Error("unsupported language error")
	}
}

// bridgeRunner records the Node bridge call and writes the output WAV.
type bridgeRunner struct {
	name string
	args []string
	req  map[string]any
}

func (b *bridgeRunner) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	b.name, b.args = name, args
	raw, err := os.ReadFile(args[len(args)-1])
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &b.req); err != nil {
		return nil, err
	}
	return nil, os.WriteFile(b.req["out"].(string), make([]byte, 100), 0o644)
}

func TestNarratorUsesBridgeForPortuguese(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model")
	_ = os.MkdirAll(model, 0o755)
	_ = os.WriteFile(filepath.Join(model, "model.onnx"), []byte("x"), 0o644)
	r := &bridgeRunner{}
	n := Narrator{Tools: Tools{TTS: "tts.exe", Node: "node", TTSBridge: "tts.mjs", ModelDir: model}, Runner: r,
		CacheDir: filepath.Join(dir, "cache"), Lang: "pt-br", Pace: 1.1}
	wav, err := n.Synthesize(context.Background(), 42, "Você não sabe")
	if err != nil {
		t.Fatal(err)
	}
	if r.name != "node" || r.args[0] != "tts.mjs" {
		t.Fatalf("ran %s %v, want the Node bridge", r.name, r.args)
	}
	// The text travels in a UTF-8 file, never on the command line.
	if r.req["text"] != "Você não sabe" || r.req["lang"] != "pt-br" || r.req["sid"].(float64) != 42 {
		t.Errorf("request = %v", r.req)
	}
	for _, a := range r.args {
		if strings.Contains(a, "Você") {
			t.Error("text must not be passed as an argument")
		}
	}
	if _, err := os.Stat(wav); err != nil {
		t.Errorf("narration not stored: %v", err)
	}
	if _, err := os.Stat(wav + ".tmp.wav.json"); err == nil {
		t.Error("request file must be removed")
	}
	// Without the bridge installed, Portuguese narration reports missing tools.
	n.Tools.TTSBridge = ""
	if _, err := n.Synthesize(context.Background(), 42, "Outra frase"); !errors.Is(err, ErrToolsMissing) {
		t.Errorf("err = %v", err)
	}
}
