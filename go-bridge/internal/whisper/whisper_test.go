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
	s := &stubChat{raw: `{"text":"do your research"}`}
	r, err := whisper.Decide(context.Background(), s, "best unholy build?", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if r.Text != "do your research" {
		t.Fatalf("%+v", r)
	}
	if s.user != "best unholy build?" || !strings.Contains(s.sys, "always reply") {
		t.Fatalf("sys=%q user=%q", s.sys, s.user)
	}
	// The model is no longer offered a reply/skip decision.
	if strings.Contains(s.sys, `"action"`) {
		t.Fatalf("whisper prompt must not ask the model for an action:\n%s", s.sys)
	}
}

func TestDecideWithNoUsableAnswerStaysQuiet(t *testing.T) {
	for _, raw := range []string{`{"text":""}`, `{"action":"skip"}`, `As an AI I cannot.`} {
		s := &stubChat{raw: raw}
		r, err := whisper.Decide(context.Background(), s, "WTS gold cheap http://x", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if r.Text != "" {
			t.Fatalf("%q should yield no line, got %+v", raw, r)
		}
	}
}
