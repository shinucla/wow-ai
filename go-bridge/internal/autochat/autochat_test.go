package autochat_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chelinho139/wow-ai/go-bridge/internal/autochat"
	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

type stubChat struct {
	raw  string
	sys  string
	user string
}

func (s *stubChat) Chat(_ context.Context, system, user string, _ []llm.Message) (string, error) {
	s.sys, s.user = system, user
	return s.raw, nil
}

func TestDecideWhisper(t *testing.T) {
	s := &stubChat{raw: `{"text":"yo"}`}
	r, err := autochat.Decide(context.Background(), s, protocol.CoachWhisper, "hi", "", nil, 0, protocol.Style{}, protocol.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "yo" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(s.sys, "cocky alpha-male") {
		t.Fatal(s.sys)
	}
}

func TestDecideParty(t *testing.T) {
	s := &stubChat{raw: `{"text":"ready"}`}
	r, err := autochat.Decide(context.Background(), s, protocol.CoachParty, "Bob: ready?", "", nil, 0, protocol.Style{}, protocol.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "ready" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(s.sys, "composed and steady") || strings.Contains(s.sys, "cocky alpha-male") {
		t.Fatal(s.sys)
	}
}

// The model always speaks, so a plain line with no JSON is the reply.
func TestDecidePartyAcceptsAPlainLine(t *testing.T) {
	s := &stubChat{raw: "yeah I'm in"}
	r, err := autochat.Decide(context.Background(), s, protocol.CoachParty, "Bob: ready?", "", nil, 0, protocol.Style{}, protocol.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "yeah i'm in" {
		t.Fatalf("a plain line must be the party reply, lowercased: %+v", r)
	}
	if strings.Contains(s.sys, `"action"`) {
		t.Fatal("the party prompt must not offer the model an action")
	}
}

func TestDecideSayWordBudget(t *testing.T) {
	// A plain line (no JSON) must still be spoken, and clipped to the word budget.
	s := &stubChat{raw: "I wish someone handed me six hundred gold too honestly no cap"}
	r, err := autochat.Decide(context.Background(), s, protocol.CoachSay, "1: give me 600g pls", "", nil, 5, protocol.Style{}, protocol.Profile{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Text == "" {
		t.Fatalf("plain line should count as a reply: %+v", r)
	}
	if got := len(strings.Fields(r.Text)); got > 5 {
		t.Fatalf("reply not clipped to the word budget (%d words): %q", got, r.Text)
	}
	if !strings.Contains(s.sys, "at most 5 words") {
		t.Fatal("say system prompt must carry the word budget:\n", s.sys)
	}
	if !strings.Contains(s.sys, "PhD student") {
		t.Fatal("say system prompt must use the PhD-student persona")
	}
}

// A coach that is not a persona mode must be rejected outright.
func TestDecideRejectsNonPersonaCoach(t *testing.T) {
	s := &stubChat{raw: "hi"}
	if _, err := autochat.Decide(context.Background(), s, protocol.CoachNone, "hi", "", nil, 0, protocol.Style{}, protocol.Profile{}); err == nil {
		t.Fatal("assist mode must not use the persona path")
	}
}

// The "Myself" profile reaches the persona system prompt as reference background.
func TestDecideIncludesProfile(t *testing.T) {
	s := &stubChat{raw: "yeah"}
	prof := protocol.Profile{Opinions: "hates overpriced auction house flippers"}
	if _, err := autochat.Decide(context.Background(), s, protocol.CoachParty, "Bob: hi", "", nil, 0, protocol.Style{}, prof); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(s.sys, "hates overpriced auction house flippers") || !strings.Contains(s.sys, "rules above always win") {
		t.Fatalf("profile must reach the persona system prompt:\n%s", s.sys)
	}
}
