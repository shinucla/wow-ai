package protocol

import (
	"strings"
	"testing"
)

// A zero Style must leave every persona prompt byte-identical to the original.
func TestZeroStyleAddsNothing(t *testing.T) {
	for _, coach := range []Coach{CoachWhisper, CoachParty, CoachSay} {
		plain := SystemPrompt("", "", coach, Style{})
		if strings.Contains(plain, "Reply style for this character") {
			t.Fatalf("%s: zero style must not add a style block", coach)
		}
	}
	if got := SaySystemPrompt("", 7, Style{}); strings.Contains(got, "Reply style") {
		t.Fatal("say: zero style must not add a style block")
	}
	if !(Style{}).IsZero() {
		t.Fatal("empty style should report IsZero")
	}
	if (Style{Personality: "witty"}).IsZero() {
		t.Fatal("a set field must make IsZero false")
	}
}

// Chosen options must reach the prompt, without displacing the persona's rules.
func TestStyleClauseReachesPrompt(t *testing.T) {
	style := Style{Personality: "sarcastic", Education: "phd", Characteristics: "curious"}
	got := SystemPrompt("", "", CoachSay, style)

	if !strings.Contains(got, "Reply style for this character") {
		t.Fatal("style block missing from the prompt")
	}
	for _, want := range []string{"dry and sarcastic", "doctoral-level expert", "Be curious"} {
		if !strings.Contains(got, want) {
			t.Fatalf("style clause missing %q:\n%s", want, got)
		}
	}
	// The persona's own hard rules must survive a style tweak.
	if !strings.Contains(got, "PhD student") || !strings.Contains(got, "Never sound like an AI") {
		t.Fatal("style must not replace the persona rules")
	}
	if !strings.Contains(got, "only adjusts tone") {
		t.Fatal("style block must state it only changes tone")
	}
}

func TestStyleClauseIgnoresUnknownKeys(t *testing.T) {
	got := SystemPrompt("", "", CoachWhisper, Style{Personality: "definitely-not-real"})
	if strings.Contains(got, "Reply style for this character") {
		t.Fatalf("an unknown key must be ignored, got:\n%s", got)
	}
}

func TestSaySystemPromptKeepsBudgetWithStyle(t *testing.T) {
	got := SaySystemPrompt("", 6, Style{Personality: "friendly"})
	if !strings.Contains(got, "at most 6 words") {
		t.Fatal("word budget must survive a style tweak")
	}
	if !strings.Contains(got, "warm and friendly") {
		t.Fatal("style clause missing from the say prompt")
	}
}

// The dropdown catalogs must stay in sync with the clauses they advertise.
func TestStyleCatalogsAreWellFormed(t *testing.T) {
	for name, opts := range map[string][]StyleOption{
		"personalities":   Personalities,
		"education":       EducationLevels,
		"characteristics": Characteristics,
	} {
		if len(opts) == 0 || opts[0].Key != "" {
			t.Fatalf("%s must offer a default option first", name)
		}
		seen := map[string]bool{}
		for _, o := range opts {
			if seen[o.Key] {
				t.Fatalf("%s has a duplicate key %q", name, o.Key)
			}
			seen[o.Key] = true
			if o.Key != "" && (o.Label == "" || o.Clause == "") {
				t.Fatalf("%s: option %q needs both a label and a clause", name, o.Key)
			}
		}
	}
}
