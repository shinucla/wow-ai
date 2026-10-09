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

func TestWhisperHelpFlagAndPrompt(t *testing.T) {
	payload := strings.Join([]string{"s", "c", "4", "", "w", "W: Bob", "help me reply"}, "\x1f")
	jobs := protocol.JobsFromStrip(0, payload)
	if len(jobs) != 1 || !jobs[0].WhisperHelp {
		t.Fatalf("%+v", jobs)
	}
	s := protocol.SystemPrompt("", "", protocol.CoachWhisper, protocol.Style{})
	if !strings.Contains(s, `"action":"reply"`) || !strings.Contains(s, "NEVER skip a question") || !strings.Contains(s, "Just checking in!") {
		t.Fatal(s)
	}
	if !strings.Contains(s, "are you a bot") || !strings.Contains(s, "Be creative every time") {
		t.Fatal(s)
	}
	if strings.Contains(protocol.SystemPrompt("", "", protocol.CoachNone, protocol.Style{}), `"action":"reply"`) {
		t.Fatal("whisper coach should be opt-in")
	}
	chat := protocol.SystemPrompt("Game: WoW\nCharacter: Test, level 80 Human Death Knight\nLocation: Dalaran\nTalents: Blood 51 / Frost 11 / Unholy 0", "primer text", protocol.CoachNone, protocol.Style{})
	if !strings.Contains(chat, "wow-ai addon") || !strings.Contains(chat, `"TL;DR:"`) {
		t.Fatal("assist prompt missing reply format")
	}
	if !strings.Contains(chat, "AI assistant") {
		t.Fatal("assist prompt must identify as AI assistant, not a player persona")
	}
	if !strings.Contains(chat, "Location: Dalaran") || !strings.Contains(chat, "Talents: Blood") {
		t.Fatal("assist prompt must include full game context:\n", chat)
	}
	if !strings.Contains(chat, "wowmacro") || !strings.Contains(chat, "primer text") {
		t.Fatal("assist prompt missing macro/primer with context")
	}
	if strings.Contains(chat, "cocky alpha-male") || strings.Contains(chat, `"action":"skip"`) || strings.Contains(chat, "composed and steady") {
		t.Fatal("assist prompt must not include whisper/party persona")
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
	if !strings.Contains(s, "more engaging") || !strings.Contains(s, "Gibberish") {
		t.Fatal("party should lean more engaging than whisper, and skip gibberish:\n", s)
	}
	if !strings.Contains(s, "Keep the thread going") || !strings.Contains(s, "Never skip an answer") {
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
	// The bridge picks a per-turn word budget through SaySystemPrompt.
	wp := protocol.SaySystemPrompt("", 7, protocol.Style{})
	if !strings.Contains(wp, "at most 7 words") {
		t.Fatal("say prompt must carry the word budget:\n", wp)
	}
	if got := protocol.ClipWords("one two three four five six", 3); got != "one two three" {
		t.Fatalf("ClipWords = %q", got)
	}
	if got := protocol.ClipWords("short line", 9); got != "short line" {
		t.Fatalf("ClipWords under budget changed text: %q", got)
	}
	// The scholar must almost never skip, and must aim to make the player look smart.
	if !strings.Contains(s, "lean hard toward reply") || !strings.Contains(s, "When in doubt, reply") {
		t.Fatal("say coach must rarely skip:\n", s)
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

func TestParseSayDecisionLeansTowardReply(t *testing.T) {
	cases := []struct {
		in, action, text string
	}{
		// JSON reply works as usual.
		{`{"action":"reply","text":"Wish I had 600g to spare myself."}`, "reply", "Wish I had 600g to spare myself."},
		// A plain spoken line with no JSON must still count as an answer.
		{`I wish someone would hand me 600g too.`, "reply", "I wish someone would hand me 600g too."},
		{`  600g? In this economy?  `, "reply", "600g? In this economy?"},
		// Explicit skip is honored.
		{`{"action":"skip"}`, "skip", ""},
		{`skip`, "skip", ""},
		// Pure noise is still skipped.
		{`asdlkjhasd`, "reply", "asdlkjhasd"}, // meaning-bearing-ish: lenient parser keeps it
		// Bot-sounding text is still refused.
		{`As an AI I cannot help with that.`, "skip", ""},
	}
	for _, c := range cases {
		d := protocol.ParseSayDecision(c.in)
		if d.Action != c.action || d.Text != c.text {
			t.Fatalf("%q → %+v want action=%q text=%q", c.in, d, c.action, c.text)
		}
	}
}

func TestParseWhisperDecision(t *testing.T) {
	cases := []struct {
		in, action, text string
	}{
		{`{"action":"reply","text":"yo"}`, "reply", "yo"},
		{`Sure.\n{"action":"skip"}`, "skip", ""},
		{`{"action":"reply","text":"Just checking in!"}`, "skip", ""},
		{`{"action":"reply","text":"As an AI I can help"}`, "skip", ""},
		{`{"action":"reply","text":""}`, "skip", ""},
		{`not json at all`, "skip", ""},
		{"```json\n{\"action\":\"reply\",\"text\":\"sup\"}\n```", "reply", "sup"},
	}
	for _, c := range cases {
		d := protocol.ParseWhisperDecision(c.in)
		if d.Action != c.action || d.Text != c.text {
			t.Fatalf("%q → %+v want action=%q text=%q", c.in, d, c.action, c.text)
		}
	}
}
