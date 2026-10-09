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
	if got := c.SayReplyChancePct(); got != 50 {
		t.Fatalf("default say reply chance = %d, want 50", got)
	}
	if got := c.SayCollectSeconds(); got != 10 {
		t.Fatalf("default say collect seconds = %d, want 10", got)
	}
	lo, hi := c.SayWordRange()
	if lo != 4 || hi != 13 {
		t.Fatalf("default say word range = %d..%d, want 4..13", lo, hi)
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
	lo, hi := c.SayWordRange()
	if lo != 5 || hi != 20 {
		t.Fatalf("an inverted word range should be swapped, got %d..%d", lo, hi)
	}

	c.Modes.Whisper.Enabled = Bool(false)
	if c.ModeEnabled("whisper") {
		t.Fatalf("an explicit false must turn the mode off")
	}
}
