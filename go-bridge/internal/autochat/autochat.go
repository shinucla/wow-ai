// Package autochat runs persona-prompt → LLM → reply text for the auto-reply modes.
// Used for whisper, party and say (and their short-circuit CLI clients). The models
// no longer decide reply vs skip: the bridge's reply chance decides whether a line
// is sent to the model at all, and when it is, the model always produces a line.
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

// Result is one persona reply after the model responds.
type Result struct {
	Text  string // the line to speak; "" means the model produced nothing usable
	Raw   string
	Coach protocol.Coach
}

// Decide asks the LLM with the given persona coach and returns the line to speak.
// coach must be one of CoachWhisper, CoachParty or CoachSay — normal addon chat must
// not use this path. maxWords > 0 adds a per-turn word budget (the bridge picks it)
// and hard-clips the reply to it; profile is the player's "Myself" reference material.
// An empty Text means there was nothing usable to say, and the caller stays quiet.
func Decide(ctx context.Context, client Chatter, coach protocol.Coach, incoming, gameCtx string, history []llm.Message, maxWords int, style protocol.Style, profile protocol.Profile) (Result, error) {
	switch coach {
	case protocol.CoachWhisper, protocol.CoachParty, protocol.CoachSay:
	default:
		return Result{}, fmt.Errorf("autochat.Decide: coach %q not allowed (whisper, party or say only)", coach)
	}
	raw, err := client.Chat(ctx, protocol.BudgetedSystemPrompt(gameCtx, coach, style, profile, maxWords), incoming, history)
	if err != nil {
		return Result{}, err
	}
	text := protocol.ClipWords(protocol.ReplyText(raw, 200), maxWords)
	return Result{Text: text, Raw: raw, Coach: coach}, nil
}
