// Package autochat runs coach-prompt → LLM → JSON reply|skip decisions.
// Used for whisper and party auto-reply (and their short-circuit CLI clients).
package autochat

import (
	"context"
	"fmt"

	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

// Chatter is satisfied by *llm.Client.
type Chatter interface {
	Chat(ctx context.Context, system, user string, history []llm.Message) (string, error)
}

// Result is one auto-reply decision after the model responds.
type Result struct {
	Decision protocol.WhisperDecision
	Raw      string
	Text     string // "skip" or the outbound chat body
	Coach    protocol.Coach
}

// Decide asks the LLM with the given coach rules and returns a structured decision.
// coach must be CoachWhisper or CoachParty — normal addon chat must not use this path.
func Decide(ctx context.Context, client Chatter, coach protocol.Coach, incoming, gameCtx string, history []llm.Message) (Result, error) {
	if coach != protocol.CoachWhisper && coach != protocol.CoachParty {
		return Result{}, fmt.Errorf("autochat.Decide: coach %q not allowed (use whisper or party only)", coach)
	}
	raw, err := client.Chat(ctx, protocol.SystemPrompt(gameCtx, "", coach), incoming, history)
	if err != nil {
		return Result{}, err
	}
	dec := protocol.ParseWhisperDecision(raw)
	text := "skip"
	if dec.Action == "reply" {
		text = dec.Text
	}
	return Result{Decision: dec, Raw: raw, Text: text, Coach: coach}, nil
}
