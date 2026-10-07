package whisper_test

import (
	"context"
	"strings"
	"testing"

	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/whisper"
)

type stubChat struct {
	raw  string
	err  error
	sys  string
	user string
}

func (s *stubChat) Chat(_ context.Context, system, user string, _ []llm.Message) (string, error) {
	s.sys, s.user = system, user
	return s.raw, s.err
}

func TestDecideReply(t *testing.T) {
	s := &stubChat{raw: `{"action":"reply","text":"do your research"}`}
	r, err := whisper.Decide(context.Background(), s, "best unholy build?", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Decision.Action != "reply" || r.Decision.Text != "do your research" || r.Text != "do your research" {
		t.Fatalf("%+v", r)
	}
	if s.user != "best unholy build?" || !strings.Contains(s.sys, "NEVER skip a question") {
		t.Fatalf("sys=%q user=%q", s.sys, s.user)
	}
}

func TestDecideSkip(t *testing.T) {
	s := &stubChat{raw: `{"action":"skip"}`}
	r, err := whisper.Decide(context.Background(), s, "WTS gold cheap http://x", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Decision.Action != "skip" || r.Text != "skip" {
		t.Fatalf("%+v", r)
	}
}
