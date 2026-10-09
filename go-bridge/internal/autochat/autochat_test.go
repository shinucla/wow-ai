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
	s := &stubChat{raw: `{"action":"reply","text":"yo"}`}
	r, err := autochat.Decide(context.Background(), s, protocol.CoachWhisper, "hi", "", nil, protocol.Style{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Decision.Action != "reply" || r.Text != "yo" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(s.sys, "cocky alpha-male") {
		t.Fatal(s.sys)
	}
}

func TestDecideParty(t *testing.T) {
	s := &stubChat{raw: `{"action":"reply","text":"ready"}`}
	r, err := autochat.Decide(context.Background(), s, protocol.CoachParty, "Bob: ready?", "", nil, protocol.Style{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Decision.Action != "reply" || r.Text != "ready" {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(s.sys, "composed and steady") || strings.Contains(s.sys, "cocky alpha-male") {
		t.Fatal(s.sys)
	}
}

func TestDecideSayWordBudgetAndPlainLine(t *testing.T) {
	// A plain line (no JSON) must still be spoken, and clipped to the word budget.
	s := &stubChat{raw: "I wish someone handed me six hundred gold too honestly no cap"}
	r, err := autochat.DecideSay(context.Background(), s, "1: give me 600g pls", "", nil, 5, protocol.Style{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Decision.Action != "reply" {
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
