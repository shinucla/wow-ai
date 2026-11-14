package protocol_test

import (
	"strings"
	"testing"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

func TestJobsFromStrip(t *testing.T) {
	payload := strings.Join([]string{"sess", "c1", "12", "", "h", "Chat", "hello"}, "\x1f")
	jobs := protocol.JobsFromStrip(0, payload)
	if len(jobs) != 1 {
		t.Fatalf("jobs %d", len(jobs))
	}
	j := jobs[0]
	if j.ID != 12 || j.Chat != "c1" || j.Text != "hello" || !j.Hello {
		t.Fatalf("%+v", j)
	}
}

func TestJobsWithContext(t *testing.T) {
	payload := strings.Join([]string{"s", "c", "3", "", "c", "N", "Zone: Elwynn", "hi"}, "\x1f")
	jobs := protocol.JobsFromStrip(0, payload)
	if len(jobs) != 1 || jobs[0].Ctx != "Zone: Elwynn" || jobs[0].Text != "hi" {
		t.Fatalf("%+v", jobs)
	}
}

func TestLuaTableAndSummary(t *testing.T) {
	body := protocol.LuaTable("WoWAI_SlotData", []protocol.Reply{{
		Chat: "c", ID: 1, Status: "done", Text: "answer\n\nTL;DR:\nfixed it", Agent: "api",
	}}, protocol.LuaOpts{Now: time.Unix(1700000000, 0).UTC(), Cwd: `C:\proj`, Agent: "api", Agents: []string{"api"}})
	if !strings.Contains(body, `WoWAI_SlotData = {`) {
		t.Fatal(body)
	}
	if !strings.Contains(body, `status = "done"`) {
		t.Fatal(body)
	}
	full, sum := protocol.SplitSummary("answer\n\nTL;DR:\nfixed it")
	if full == "" || sum != "fixed it" {
		t.Fatalf("%q %q", full, sum)
	}
}

func TestSilentWAV(t *testing.T) {
	if len(protocol.SilentWAV) < 44 {
		t.Fatal(len(protocol.SilentWAV))
	}
	if string(protocol.SilentWAV[0:4]) != "RIFF" {
		t.Fatal("not riff")
	}
}

func TestJobsInstanceFlag(t *testing.T) {
	payload := strings.Join([]string{"s", "c", "9", "", "i=2", "N", "hi"}, "\x1f")
	jobs := protocol.JobsFromStrip(0, payload)
	if len(jobs) != 1 || jobs[0].Instance != 2 {
		t.Fatalf("%+v", jobs)
	}
}

func TestPartyHelpFlagAndPrompt(t *testing.T) {
	payload := strings.Join([]string{"s", "c", "5", "", "p", "Party", "Bob: ready?"}, "\x1f")
	jobs := protocol.JobsFromStrip(0, payload)
	if len(jobs) != 1 || !jobs[0].PartyHelp || jobs[0].CoachFor() != protocol.CoachParty {
		t.Fatalf("%+v", jobs)
	}
	s := protocol.SystemPrompt("", "", protocol.CoachParty, protocol.Style{})
	if !strings.Contains(s, "composed and steady") || !strings.Contains(s, "party chat") {
		t.Fatal(s)
	}
	// Party always speaks: no reply/skip decision is handed to the model, and the
	// bridge's reply chance decides whether the model is asked at all.
	if !strings.Contains(s, `{"text":"short reply here"}`) || strings.Contains(s, `"action"`) {
		t.Fatalf("party must request only text:\n%s", s)
	}
	if !strings.Contains(s, "always say something back") {
		t.Fatalf("party must always speak:\n%s", s)
	}
	// Casual chat casing, like a real player typing fast.
	if !strings.Contains(s, "ALL LOWERCASE") {
		t.Fatal("party must ask for all-lowercase lines:\n", s)
	}
	if !strings.Contains(s, "more engaging") || !strings.Contains(s, "gibberish") {
		t.Fatal("party should lean more engaging than whisper, and say something even for gibberish:\n", s)
	}
	if !strings.Contains(s, "Keep the thread going") {
		t.Fatal("party must continue conversations after its own questions:\n", s)
	}
	if strings.Contains(s, "cocky alpha-male") {
		t.Fatal("party must not use whisper voice")
	}
}

func TestSayHelpFlagAndPrompt(t *testing.T) {
	payload := strings.Join([]string{"s", "c6", "7", "", "s", "Say", "what is gravity?"}, "\x1f")
	jobs := protocol.JobsFromStrip(0, payload)
	if len(jobs) != 1 || !jobs[0].SayHelp || jobs[0].CoachFor() != protocol.CoachSay {
		t.Fatalf("%+v", jobs)
	}
	s := protocol.SystemPrompt("", "", protocol.CoachSay, protocol.Style{})
	if !strings.Contains(s, "/say chat") || !strings.Contains(s, "PhD student") {
		t.Fatal("say coach must be the PhD-student persona:\n", s)
	}
	// Persona flavor: outdoors + movies, and no patience for beggars.
	if !strings.Contains(s, "fishing") || !strings.Contains(s, "hiking") || !strings.Contains(s, "movies") {
		t.Fatal("say persona should be into fishing, hiking and movies:\n", s)
	}
	if !strings.Contains(s, "beggar") || !strings.Contains(s, "earn it") {
		t.Fatal("say persona should push back on beggars:\n", s)
	}
	// Say lines arrive labelled with their speaker; the coach must know that and
	// must be told not to over-use the names.
	if !strings.Contains(s, "who said it") || !strings.Contains(s, `1: Bob:`) || !strings.Contains(s, "2: Jane:") {
		t.Fatal("say coach must be told each line carries its speaker:\n", s)
	}
	if !strings.Contains(s, "Use a name only when it earns its place") || !strings.Contains(s, "Never invent") {
		t.Fatal("say coach must be told to use names sparingly and never invent one:\n", s)
	}
	// The bridge picks a per-turn word budget through BudgetedSystemPrompt.
	wp := protocol.BudgetedSystemPrompt("", protocol.CoachSay, protocol.Style{}, protocol.Profile{}, 7)
	if !strings.Contains(wp, "at most 7 words") {
		t.Fatal("say prompt must carry the word budget:\n", wp)
	}
	if got := protocol.ClipWords("one two three four five six", 3); got != "one two three" {
		t.Fatalf("ClipWords = %q", got)
	}
	if got := protocol.ClipWords("short line", 9); got != "short line" {
		t.Fatalf("ClipWords under budget changed text: %q", got)
	}
	// The scholar always says something; nothing in the prompt offers a skip.
	if strings.Contains(s, `"action"`) || !strings.Contains(s, "you always say something") {
		t.Fatal("say coach must always speak and never offer an action:\n", s)
	}
	if !strings.Contains(s, "When in doubt, say something") {
		t.Fatal("say coach must lean toward speaking:\n", s)
	}
	if !strings.Contains(s, "make the player look smart") || !strings.Contains(s, "knowledgeable") {
		t.Fatal("say coach must aim to make the player look smart and knowledgeable:\n", s)
	}
	// Must sound human, with only occasional Gen-Z slang (~5%), never AI.
	if !strings.Contains(s, "Gen-Z") || !strings.Contains(s, "1 line in 20") {
		t.Fatal("say coach must keep Gen-Z slang rare:\n", s)
	}
	if !strings.Contains(s, "Never sound like an AI") || !strings.Contains(s, "let me know if") {
		t.Fatal("say coach must forbid AI-sounding phrasings:\n", s)
	}
	if strings.Contains(s, "cocky alpha-male") || strings.Contains(s, "composed and steady") {
		t.Fatal("say must not reuse whisper/party voice")
	}
	// The scholar persona must not leak into the assist prompt.
	if strings.Contains(protocol.SystemPrompt("", "", protocol.CoachNone, protocol.Style{}), "scholarly professor") {
		t.Fatal("say coach should be opt-in")
	}
}

func TestSayTextSerialized(t *testing.T) {
	body := protocol.LuaTable("WoWAI_SlotData", []protocol.Reply{{
		Chat: "c6", ID: 7, Status: "done", Text: "Paris.", SayText: "Paris.", Agent: "api",
	}}, protocol.LuaOpts{Now: time.Unix(1700000000, 0).UTC(), Agent: "api", Agents: []string{"api"}})
	if !strings.Contains(body, `sayText = "Paris."`) {
		t.Fatal("sayText should reach the addon:\n", body)
	}
}

// ReplyText is the single parser for all three persona modes: the models no longer
// choose reply vs skip, so it only ever yields the line (or "" for nothing usable).
func TestReplyText(t *testing.T) {
	cases := []struct{ in, want string }{
		// The requested {"text":...} shape.
		{`{"text":"ready"}`, "ready"},
		// Persona chat is forced to all lowercase.
		{`{"text":"Sure, Paris."}`, "sure, paris."},
		{`Yeah I'm In`, "yeah i'm in"},
		// A bare spoken line with no JSON is still the answer.
		{`I wish someone would hand me 600g too.`, "i wish someone would hand me 600g too."},
		{`  600g? In this economy?  `, "600g? in this economy?"},
		// Fenced JSON is unwrapped.
		{"```json\n{\"text\":\"sup\"}\n```", "sup"},
		// No usable answer: empty text, a stray skip, or anything bot-sounding.
		{`{"text":""}`, ""},
		{`{"action":"skip"}`, ""},
		{`skip`, ""},
		{`As an AI I cannot help with that.`, ""},
	}
	for _, c := range cases {
		if got := protocol.ReplyText(c.in, 200); got != c.want {
			t.Fatalf("%q → %q want %q", c.in, got, c.want)
		}
	}
	// A secondary rune cap guards against a runaway line even without a word budget.
	if got := protocol.ReplyText(strings.Repeat("a", 300), 200); len([]rune(got)) != 200 {
		t.Fatalf("ReplyText should clip to the rune cap, got %d", len([]rune(got)))
	}
}
