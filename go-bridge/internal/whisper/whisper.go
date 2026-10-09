// Package whisper is a thin wrapper around autochat for whisper-mode decisions.
package whisper

import (
	"context"

	"github.com/chelinho139/wow-ai/go-bridge/internal/autochat"
	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

type Chatter = autochat.Chatter
type Result = autochat.Result

func SystemPrompt(gameCtx string) string {
	return protocol.SystemPrompt(gameCtx, "", protocol.CoachWhisper, protocol.Style{})
}

func Parse(raw string) protocol.WhisperDecision {
	return protocol.ParseWhisperDecision(raw)
}

func Decide(ctx context.Context, client Chatter, incoming, gameCtx string, history []llm.Message) (Result, error) {
	return autochat.Decide(ctx, client, protocol.CoachWhisper, incoming, gameCtx, history, protocol.Style{})
}
