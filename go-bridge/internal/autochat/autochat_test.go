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
	r, err := autochat.Decide(context.Background(), s, protocol.CoachWhisper, "hi", "", nil)
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
	r, err := autochat.Decide(context.Background(), s, protocol.CoachParty, "Bob: ready?", "", nil)
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
