package config

import "testing"

// A config file written before the modes section existed must still enable every
// mode: zero-value bools must not silently turn auto-reply off.
func TestModeDefaultsWhenModesAbsent(t *testing.T) {
	var c Config
	c.ApplyDefaults()

	if !c.ModeEnabled("whisper") || !c.ModeEnabled("party") || !c.ModeEnabled("say") {
		t.Fatalf("modes must default to enabled, got whisper=%v party=%v say=%v",
			c.ModeEnabled("whisper"), c.ModeEnabled("party"), c.ModeEnabled("say"))
	}
	// Every persona mode defaults to 100 (always ask the model), so a legacy config
	// never starts skipping lines it used to answer.
	for _, mode := range []string{"whisper", "party", "say"} {
		if got := c.ModeReplyChancePct(mode); got != 100 {
			t.Fatalf("default %s reply chance = %d, want 100", mode, got)
		}
		lo, hi := c.ModeWordRange(mode)
		if lo != 4 || hi != 13 {
			t.Fatalf("default %s word range = %d..%d, want 4..13", mode, lo, hi)
		}
	}
	if got := c.ModeReplyChancePct("assist"); got != 100 {
		t.Fatalf("a mode without the setting must be 100, got %d", got)
	}
	if got := c.SayCollectSeconds(); got != 10 {
		t.Fatalf("default say collect seconds = %d, want 10", got)
	}
	if c.ModeEnabled("nope") != true {
		t.Fatalf("an unknown mode name should default to enabled")
	}
}

func TestModeExplicitValuesAndClamping(t *testing.T) {
	c := Default()
	c.Modes.Say.ReplyChance = Int(999)
	c.Modes.Say.CollectSeconds = Int(0)
	c.Modes.Say.MinWords = Int(20)
	c.Modes.Say.MaxWords = Int(5)
	c.ApplyDefaults()

	if got := c.SayReplyChancePct(); got != 100 {
		t.Fatalf("reply chance should clamp to 100, got %d", got)
	}
	if got := c.SayCollectSeconds(); got != 1 {
		t.Fatalf("collect seconds should clamp up to 1, got %d", got)
	}
	lo, hi := c.ModeWordRange("say")
	if lo != 5 || hi != 20 {
		t.Fatalf("an inverted word range should be swapped, got %d..%d", lo, hi)
	}

	c.Modes.Whisper.Enabled = Bool(false)
	if c.ModeEnabled("whisper") {
		t.Fatalf("an explicit false must turn the mode off")
	}

	c.Modes.Party.ReplyChance = Int(999)
	c.Modes.Whisper.ReplyChance = Int(-5)
	if got := c.ModeReplyChancePct("party"); got != 100 {
		t.Fatalf("party reply chance should clamp to 100, got %d", got)
	}
	if got := c.ModeReplyChancePct("whisper"); got != 0 {
		t.Fatalf("whisper reply chance should clamp up to 0, got %d", got)
	}
	if got := c.ModeReplyChancePct("nope"); got != 100 {
		t.Fatalf("an unknown mode should default to 100, got %d", got)
	}
}
