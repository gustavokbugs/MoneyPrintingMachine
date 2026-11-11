package production

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidVoiceUsesOfferedSpeakers(t *testing.T) {
	if !ValidVoice(DefaultVoice) {
		t.Fatal("default voice must be offered")
	}
	for _, id := range []int{-1, 0, 30, 53, 99} {
		if ValidVoice(id) {
			t.Errorf("voice %d should not be offered", id)
		}
	}
}

func TestLexiconPathPrefersV1Name(t *testing.T) {
	dir := t.TempDir()
	tools := Tools{ModelDir: dir}
	if got := tools.LexiconPath(); filepath.Base(got) != "lexicon-en-us.txt" {
		t.Fatalf("fallback = %s", got)
	}
	_ = os.WriteFile(filepath.Join(dir, "lexicon-us-en.txt"), []byte("a || x\n"), 0o644)
	if got := tools.LexiconPath(); filepath.Base(got) != "lexicon-us-en.txt" {
		t.Fatalf("v1 lexicon not preferred: %s", got)
	}
	args := strings.Join(TTSArgs(tools, "model.onnx", 3, "hi", "out.wav"), " ")
	if !strings.Contains(args, "lexicon-us-en.txt") || !strings.Contains(args, "--sid=3") {
		t.Fatalf("args = %s", args)
	}
}

func TestModelFilePrefersFullPrecision(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"model.int8.onnx", "model.onnx"} {
		_ = os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	if got, err := modelFile(dir); err != nil || got != "model.onnx" {
		t.Fatalf("modelFile = %q, %v", got, err)
	}
}
