package protocol

import (
	"strings"
	"testing"
)

func TestProfileBlockReachesPersonaPrompt(t *testing.T) {
	if !(Profile{}).IsZero() {
		t.Fatal("an empty profile should report IsZero")
	}
	p := Profile{
		About:     "31, backend dev in the uk, played since wrath, main is a resto druid",
		Interests: "fly fishing, hiking, sci-fi films",
		Opinions:  "thinks server-first guilds are overrated",
	}
	if p.IsZero() {
		t.Fatal("a set profile must not be IsZero")
	}
	got := BudgetedSystemPrompt("", CoachParty, Style{}, p, 8)
	for _, want := range []string{
		"About the player you are speaking as",
		"Who I am: 31, backend dev",
		"Interests and hobbies: fly fishing",
		"Opinions and hot takes: thinks server-first guilds are overrated",
		"rules above always win",
		"at most 8 words",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("profile prompt missing %q:\n%s", want, got)
		}
	}
	// The profile sits alongside the persona, it does not replace it, and it never
	// carries tone/slang — those live in the per-mode Style.
	if !strings.Contains(got, "composed and steady") {
		t.Fatal("profile must not displace the persona")
	}
	if strings.Contains(got, "How I talk") || strings.Contains(got, "Personality:") {
		t.Fatal("profile must not carry tone/slang fields")
	}
}

func TestProfileIsReferenceOnlyForPersonaModes(t *testing.T) {
	// A zero profile adds nothing: the prompt is byte-identical to the plain one.
	plain := SystemPrompt("", "", CoachWhisper, Style{})
	with := BudgetedSystemPrompt("", CoachWhisper, Style{}, Profile{}, 5)
	if want := plain + "\n\nThis turn: reply in at most 5 words. Stay under 5; shorter is better."; with != want {
		t.Fatalf("zero profile changed the prompt:\n%s", with)
	}
	// Assist mode never carries the player profile.
	p := Profile{Opinions: "hates gold sellers"}
	assist := BudgetedSystemPrompt("", CoachNone, Style{}, p, 0)
	if strings.Contains(assist, "hates gold sellers") {
		t.Fatal("assist mode must not include the persona profile")
	}
}
