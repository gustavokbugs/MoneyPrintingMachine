package production

import (
	"errors"
	"testing"
)

func validScript() Script {
	return Script{
		Hook:     "Did you know the Moon is drifting away?",
		Sections: []Section{{Heading: "Drift", Narration: "The Moon recedes about 3.8 cm per year.", OnScreenText: "3.8 cm per year"}},
		Outro:    "Thanks for watching.",
		Claims: []Claim{
			{Text: "The Moon recedes about 3.8 cm per year.", SourceHint: "NASA lunar laser ranging", Status: ClaimUnverified},
			{Text: "Tides cause the drift.", Status: ClaimUnverified},
		},
	}
}

func TestApprovalGate(t *testing.T) {
	v := &Video{Status: StatusPlanned}
	if err := v.Approve(); err == nil {
		t.Fatal("cannot approve without a script")
	}
	if err := v.SetScript(validScript()); err != nil {
		t.Fatal(err)
	}
	var se *StateError
	if err := v.Approve(); !errors.As(err, &se) {
		t.Fatalf("expected gate to block unresolved claims, got %v", err)
	}
	if err := v.SetClaimStatus(0, ClaimVerified, "Checked NASA LLR page"); err != nil {
		t.Fatal(err)
	}
	if err := v.SetClaimStatus(1, ClaimRemoved, ""); err != nil {
		t.Fatal(err)
	}
	if err := v.Approve(); err != nil || v.Status != StatusApproved {
		t.Fatalf("approve: %v status=%s", err, v.Status)
	}
	if err := v.CanStartProduction(); err != nil {
		t.Fatal(err)
	}
	// Re-opening a claim revokes the approval.
	if err := v.SetClaimStatus(1, ClaimUnverified, ""); err != nil || v.Status != StatusScriptReady {
		t.Fatalf("approval not revoked: %v %s", err, v.Status)
	}
	if err := v.CanStartProduction(); err == nil {
		t.Fatal("production must require approval")
	}
}

func TestEditingRequiresReapproval(t *testing.T) {
	s := validScript()
	s.Claims = nil
	v := &Video{Status: StatusPlanned}
	_ = v.SetScript(s)
	_ = v.Approve()
	v.Status = StatusProduced
	if err := v.SetScript(s); err != nil || v.Status != StatusScriptReady {
		t.Fatalf("edit after production must require approval again: %v %s", err, v.Status)
	}
	v.Status = StatusProducing
	if err := v.SetScript(s); err == nil {
		t.Fatal("cannot edit while producing")
	}
}

func TestScriptValidation(t *testing.T) {
	bad := []Script{
		{},
		{Sections: []Section{{Narration: " "}}},
		{Sections: []Section{{Narration: "ok", OnScreenText: string(make([]byte, 121))}}},
		{Sections: []Section{{Narration: "ok"}}, Claims: []Claim{{Text: "x", Status: "maybe"}}},
	}
	for i, s := range bad {
		if err := s.Validate(); err == nil {
			t.Errorf("script %d should be invalid", i)
		}
	}
	s := validScript()
	if s.WordCount() != 19 || len(s.Segments()) != 3 {
		t.Fatalf("words=%d segments=%d", s.WordCount(), len(s.Segments()))
	}
}

func TestStartRules(t *testing.T) {
	v := &Video{Status: StatusScripting}
	if err := v.CanStartScript(); err == nil {
		t.Fatal("cannot start a second script job")
	}
	v.Status = StatusFailed
	v.Script = &Script{Claims: []Claim{{Text: "x", Status: ClaimUnverified}}}
	if err := v.CanStartProduction(); err == nil {
		t.Fatal("failed video with unresolved claims cannot be produced")
	}
	v.Script.Claims[0].Status = ClaimVerified
	if err := v.CanStartProduction(); err != nil {
		t.Fatalf("failed video with resolved claims can retry: %v", err)
	}
	if w, h := (&Video{AspectRatio: "9:16"}).Dimensions(); w != 1080 || h != 1920 {
		t.Fatal("vertical dimensions")
	}
}
