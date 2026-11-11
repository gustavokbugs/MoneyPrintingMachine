package production

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"contentfactory/internal/channel"
)

func TestVoiceForSpeaker(t *testing.T) {
	v := &Video{VoiceID: 3, VoiceB: 21, DialogueMode: DialogueNarration}
	if v.voiceFor("B") != 3 {
		t.Error("single-narrator videos must use one voice")
	}
	v.DialogueMode = DialogueQA
	if v.voiceFor("B") != 21 || v.voiceFor("A") != 3 || v.voiceFor("") != 3 {
		t.Error("dialogue voices wrong")
	}
	v.VoiceB = 0
	if v.voiceFor("B") != DefaultVoiceB {
		t.Error("missing second voice should use the default")
	}
}

func TestSetDialogueValidates(t *testing.T) {
	svc, store := newTestService(t, &scriptedGen{}, nil, Tools{})
	v, err := svc.CreateFromIdea(context.Background(), "alice", "ch1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if v.DialogueMode != DialogueNarration || v.VoiceB != DefaultVoiceB {
		t.Errorf("defaults = %q %d", v.DialogueMode, v.VoiceB)
	}
	var ve *channel.ValidationError
	if _, err := svc.SetDialogue(context.Background(), "alice", v.ID, "chat", 21); !errors.As(err, &ve) {
		t.Errorf("unknown mode: %v", err)
	}
	if _, err := svc.SetDialogue(context.Background(), "alice", v.ID, DialogueQA, 999); !errors.As(err, &ve) {
		t.Errorf("unknown voice: %v", err)
	}
	got, err := svc.SetDialogue(context.Background(), "alice", v.ID, DialogueDebate, 21)
	if err != nil || got.DialogueMode != DialogueDebate || got.VoiceB != 21 {
		t.Fatalf("set: %+v %v", got, err)
	}
	stored := store.items[v.ID]
	stored.Status = StatusProducing
	store.items[v.ID] = stored
	var se *StateError
	if _, err := svc.SetDialogue(context.Background(), "alice", v.ID, DialogueQA, 21); !errors.As(err, &se) {
		t.Errorf("while producing: %v", err)
	}
	if _, err := svc.SetDialogue(context.Background(), "mallory", v.ID, DialogueQA, 21); !errors.Is(err, ErrNotFound) {
		t.Errorf("other owner: %v", err)
	}
}

func TestDialogueScriptKeepsSpeakers(t *testing.T) {
	var out scriptOutput
	raw := `{"hook":"h","sections":[{"heading":"a","narration":"Why?","onScreenText":"","speaker":"A"},{"heading":"b","narration":"Because.","onScreenText":"","speaker":"B"},{"heading":"c","narration":"Hm.","onScreenText":"","speaker":"Z"}],"outro":"o","claims":[]}`
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatal(err)
	}
	sc := scriptFromOutput(out, "m", time.Now())
	if sc.Sections[0].Speaker != "A" || sc.Sections[1].Speaker != "B" || sc.Sections[2].Speaker != "" {
		t.Errorf("speakers = %+v", sc.Sections)
	}
	bad := *sc
	bad.Sections = append([]Section(nil), sc.Sections...)
	bad.Sections[0].Speaker = "C"
	var ve *channel.ValidationError
	if err := bad.Validate(); !errors.As(err, &ve) {
		t.Errorf("invalid speaker accepted: %v", err)
	}
	if !strings.Contains(string(scriptSchema(4, 6, true)), `"enum":["A","B"]`) || strings.Contains(string(scriptSchema(2, 4, false)), "speaker") {
		t.Error("schema speaker field wrong")
	}
}

func TestWordCaptionsKeepSpeakersApart(t *testing.T) {
	segs := []Timed{{Duration: 1, Text: "So"}, {Duration: 2, Text: "it works", Speaker: "B"}}
	got := WordCaptions(segs)
	if len(got) != 2 || len(got[0].Words) != 1 || got[0].Speaker != "" || got[1].Speaker != "B" {
		t.Fatalf("captions = %+v (a short word must not pair across speakers)", got)
	}
	ass := BuildWordASS(got, 1080, 1920)
	if !strings.Contains(ass, `\c&H00FFDB7F&`) {
		t.Error("second speaker not colored")
	}
}
func TestStripSpeakerLabel(t *testing.T) {
	cases := []struct{ in, text, speaker string }{
		{"Speaker A: Fleming noticed mold.", "Fleming noticed mold.", "A"},
		{"speaker b - But he published in 1929.", "But he published in 1929.", "B"},
		{"**Speaker B:** Not quite.", "Not quite.", "B"},
		{"Narrator: It was 1928.", "It was 1928.", ""},
		{"Hostile bacteria died.", "Hostile bacteria died.", ""},
		{"Speakers at the time: none.", "Speakers at the time: none.", ""},
	}
	for _, c := range cases {
		text, speaker := stripSpeakerLabel(c.in)
		if text != c.text || speaker != c.speaker {
			t.Errorf("stripSpeakerLabel(%q) = %q, %q; want %q, %q", c.in, text, speaker, c.text, c.speaker)
		}
	}
}
