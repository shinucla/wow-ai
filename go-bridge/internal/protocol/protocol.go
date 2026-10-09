// Package protocol parses strip records and builds Lua slot files for the addon.
package protocol

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	RS = "\x1e" // record separator
	US = "\x1f" // unit separator
)

// SilentWAV is a valid silent 10ms WAV (empty file won't play; this will).
var SilentWAV = buildSilentWAV()

func buildSilentWAV() []byte {
	const rate, samples = 8000, 80
	b := make([]byte, 44+samples)
	copy(b[0:], "RIFF")
	putU32(b[4:], 36+samples)
	copy(b[8:], "WAVE")
	copy(b[12:], "fmt ")
	putU32(b[16:], 16)
	putU16(b[20:], 1) // PCM
	putU16(b[22:], 1) // mono
	putU32(b[24:], rate)
	putU32(b[28:], rate)
	putU16(b[32:], 1)
	putU16(b[34:], 8)
	copy(b[36:], "data")
	putU32(b[40:], samples)
	for i := 44; i < len(b); i++ {
		b[i] = 128
	}
	return b
}

func putU16(b []byte, v uint16) { b[0] = byte(v); b[1] = byte(v >> 8) }
func putU32(b []byte, v uint32) {
	b[0] = byte(v)
	b[1] = byte(v >> 8)
	b[2] = byte(v >> 16)
	b[3] = byte(v >> 24)
}

func Pad2(n int) string { return fmt.Sprintf("%02d", n) }
func Pad3(n int) string { return fmt.Sprintf("%03d", n) }
func Pad4(n int) string { return fmt.Sprintf("%04d", n) }

func SlotNumber(id, slots int) int {
	if slots <= 0 {
		slots = 200
	}
	return ((id-1)%slots)+1
}

// Job is one message from the game.
type Job struct {
	Session    string
	Chat       string
	ID         int
	Cwd        string
	Name       string
	Text       string
	Ctx        string
	Via        string
	NewSession bool
	Hello      bool
	Forget     bool
	Context    bool
	WhisperHelp bool // flag "w": auto-whisper JSON reply|skip
	PartyHelp   bool // flag "p": auto-party JSON reply|skip
	SayHelp     bool // flag "s": auto-say collection chat
	Allow      []string
	Agent      string
	Instance   int // 1-based; from i=N flag or capture window order
}

type Flags struct {
	NewSession  bool
	Hello       bool
	Forget      bool
	Context     bool
	WhisperHelp bool
	PartyHelp   bool
	SayHelp     bool
	Allow       []string
	Agent       string
	Instance    int // 1-based client instance (i=N flag); 0 = unset
}

func ParseFlags(flags string) Flags {
	out := Flags{}
	for _, tok := range strings.Split(flags, ";") {
		switch {
		case tok == "n":
			out.NewSession = true
		case tok == "h":
			out.Hello = true
		case tok == "d":
			out.Forget = true
		case tok == "c":
			out.Context = true
		case tok == "w":
			out.WhisperHelp = true
		case tok == "p":
			out.PartyHelp = true
		case tok == "s":
			out.SayHelp = true
		case strings.HasPrefix(tok, "allow="):
			for _, a := range strings.Split(tok[6:], ",") {
				a = strings.TrimSpace(a)
				if a != "" {
					out.Allow = append(out.Allow, a)
				}
			}
		case strings.HasPrefix(tok, "agent="):
			out.Agent = strings.ToLower(strings.TrimSpace(tok[6:]))
		case strings.HasPrefix(tok, "i="):
			out.Instance = atoi(tok[2:])
		}
	}
	return out
}

// CoachFor returns the auto-reply coach for a job (party wins if both set).
func (j Job) CoachFor() Coach {
	if j.PartyHelp {
		return CoachParty
	}
	if j.WhisperHelp {
		return CoachWhisper
	}
	if j.SayHelp {
		return CoachSay
	}
	return CoachNone
}

func JobsFromStrip(headerID int, payload string) []Job {
	var jobs []Job
	for _, rec := range strings.Split(payload, RS) {
		p := strings.Split(rec, US)
		if len(p) >= 7 && isDigits(p[2]) {
			f := ParseFlags(p[4])
			withCtx := f.Context && len(p) >= 8
			j := Job{
				Session: p[0], Chat: p[1], ID: atoi(p[2]), Cwd: p[3],
				NewSession: f.NewSession, Hello: f.Hello, Forget: f.Forget, Context: f.Context,
				WhisperHelp: f.WhisperHelp, PartyHelp: f.PartyHelp, SayHelp: f.SayHelp,
				Allow: f.Allow, Agent: f.Agent, Instance: f.Instance, Name: p[5], Via: "pixel",
			}
			if withCtx {
				j.Ctx = p[6]
				j.Text = strings.Join(p[7:], US)
			} else {
				j.Text = strings.Join(p[6:], US)
			}
			jobs = append(jobs, j)
		} else if len(p) == 6 && isDigits(p[2]) {
			f := ParseFlags(p[4])
			jobs = append(jobs, Job{
				Session: p[0], Chat: p[1], ID: atoi(p[2]), Cwd: p[3],
				NewSession: f.NewSession, Hello: f.Hello, Forget: f.Forget, Context: f.Context,
				WhisperHelp: f.WhisperHelp, PartyHelp: f.PartyHelp, SayHelp: f.SayHelp,
				Allow: f.Allow, Agent: f.Agent, Instance: f.Instance, Name: "", Text: p[5], Via: "pixel",
			})
		} else if len(p) == 4 {
			f := ParseFlags(p[2])
			jobs = append(jobs, Job{
				Session: p[0], Chat: "", ID: headerID, Cwd: p[1],
				NewSession: f.NewSession, Hello: f.Hello, Forget: f.Forget, Context: f.Context,
				WhisperHelp: f.WhisperHelp, PartyHelp: f.PartyHelp, SayHelp: f.SayHelp,
				Allow: f.Allow, Agent: f.Agent, Instance: f.Instance, Text: p[3], Via: "pixel",
			})
		}
	}
	return jobs
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func ChatKey(j Job) string {
	// Keep assist / whisper / party histories separate so a persona coach can
	// never few-shot the in-addon AI assist (and vice versa).
	mode := "a"
	switch j.CoachFor() {
	case CoachWhisper:
		mode = "w"
	case CoachParty:
		mode = "p"
	}
	return j.Session + ":" + orDefault(j.Chat, "default") + ":" + mode
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

func ResolveCwd(raw, base string) string {
	p := strings.TrimSpace(raw)
	if p == "" {
		return base
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, _ := os.UserHomeDir()
		p = filepath.Join(home, strings.TrimLeft(p[1:], `/\`))
	}
	if filepath.IsAbs(p) || winAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(base, p))
}

func winAbs(p string) bool {
	if len(p) >= 3 && ((p[0] >= 'A' && p[0] <= 'Z') || (p[0] >= 'a' && p[0] <= 'z')) && p[1] == ':' {
		return p[2] == '\\' || p[2] == '/'
	}
	return strings.HasPrefix(p, `\\`)
}

// Reply is one chat's latest status for the slot file.
type Reply struct {
	Chat          string
	ID            int
	Status        string // working | done | error
	Text          string
	Cwd           string
	Session       string
	Agent         string
	Summary       string
	Instance      int
	Denied        []string
	Macros        []Macro
	WhisperAction string // "reply" | "skip" | "" (auto-whisper decisions)
	WhisperText   string // outbound whisper body when action is reply
	SayText       string // outbound say body when action is reply
}

type Macro struct {
	Name  string
	Body  string
	Icon  interface{} // nil, int, or string
	Char  bool
	Risky bool
}

type LuaOpts struct {
	Now     time.Time
	Cwd     string
	Agent   string
	Agents  []string
	Restore *Restore
}

type Restore struct {
	Token string
	Chats []RestoreChat
}

type RestoreChat struct {
	ID       string
	Name     string
	Cwd      string
	Messages []RestoreMsg
}

type RestoreMsg struct {
	Role  string
	ID    int
	T     int64
	Agent string
	Text  string
}

func LuaStr(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\r':
			// drop
		case '\n':
			b.WriteString(`\n`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, "\\%03d", r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

func LuaTable(global string, records []Reply, opts LuaOpts) string {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	var lines []string
	lines = append(lines,
		"-- Written by the wow-ai Go bridge. Do not edit by hand.",
		global+" = {",
		"\tts = "+LuaStr(now.UTC().Format(time.RFC3339Nano))+",",
		fmt.Sprintf("\tnow = %d,", now.Unix()),
		"\tcwd = "+LuaStr(opts.Cwd)+",",
		"\tagent = "+LuaStr(opts.Agent)+",",
		"\tagents = { "+joinLuaStr(opts.Agents)+" },",
		"\treplies = {",
	)
	for _, r := range records {
		lines = append(lines, "\t\t{")
		lines = append(lines,
			"\t\t\tchat = "+LuaStr(r.Chat)+",",
			fmt.Sprintf("\t\t\tid = %d,", r.ID),
			"\t\t\tstatus = "+LuaStr(r.Status)+",",
			"\t\t\ttext = "+LuaStr(r.Text)+",",
			"\t\t\tcwd = "+LuaStr(r.Cwd)+",",
			"\t\t\tsession = "+LuaStr(r.Session)+",",
			"\t\t\tagent = "+LuaStr(r.Agent)+",",
		)
		if r.Instance > 0 {
			lines = append(lines, fmt.Sprintf("\t\t\tinstance = %d,", r.Instance))
		}
		if r.Summary != "" {
			lines = append(lines, "\t\t\tsummary = "+LuaStr(r.Summary)+",")
		}
		if r.WhisperAction != "" {
			lines = append(lines, "\t\t\twhisperAction = "+LuaStr(r.WhisperAction)+",")
		}
		if r.WhisperText != "" {
			lines = append(lines, "\t\t\twhisperText = "+LuaStr(r.WhisperText)+",")
		}
		if r.SayText != "" {
			lines = append(lines, "\t\t\tsayText = "+LuaStr(r.SayText)+",")
		}
		if len(r.Denied) > 0 {
			lines = append(lines, "\t\t\tdenied = { "+joinLuaStr(r.Denied)+" },")
		}
		if len(r.Macros) > 0 {
			lines = append(lines, luaMacros(r.Macros))
		}
		lines = append(lines, "\t\t},")
	}
	lines = append(lines, "\t},")
	if opts.Restore != nil {
		lines = append(lines, "\trestore = {", "\t\ttoken = "+LuaStr(opts.Restore.Token)+",", "\t\tchats = {")
		for _, c := range opts.Restore.Chats {
			lines = append(lines, "\t\t\t{",
				"\t\t\t\tid = "+LuaStr(c.ID)+",",
				"\t\t\t\tname = "+LuaStr(c.Name)+",",
				"\t\t\t\tcwd = "+LuaStr(c.Cwd)+",",
				"\t\t\t\tmessages = {")
			for _, m := range c.Messages {
				lines = append(lines, fmt.Sprintf(
					"\t\t\t\t\t{ role = %s, id = %d, t = %d, agent = %s, text = %s },",
					LuaStr(m.Role), m.ID, m.T, LuaStr(m.Agent), LuaStr(m.Text)))
			}
			lines = append(lines, "\t\t\t\t},", "\t\t\t},")
		}
		lines = append(lines, "\t\t},", "\t},")
	}
	lines = append(lines, "}", "")
	return strings.Join(lines, "\n")
}

func joinLuaStr(ss []string) string {
	parts := make([]string, len(ss))
	for i, s := range ss {
		parts[i] = LuaStr(s)
	}
	return strings.Join(parts, ", ")
}

func luaMacros(macros []Macro) string {
	parts := make([]string, 0, len(macros))
	for _, m := range macros {
		icon := "nil"
		switch v := m.Icon.(type) {
		case int:
			icon = strconv.Itoa(v)
		case string:
			icon = LuaStr(v)
		}
		parts = append(parts, fmt.Sprintf(
			"{ name = %s, body = %s, icon = %s, char = %v, risky = %v }",
			LuaStr(m.Name), LuaStr(m.Body), icon, m.Char, m.Risky))
	}
	return "\t\t\tmacros = { " + strings.Join(parts, ", ") + " },"
}

var markerRE = regexp.MustCompile(`(?i)(?:^|\n)[ \t]*(?:#+[ \t]*)?(?:\*\*|__)?[ \t]*TL;?DR[ \t]*:?[ \t]*(?:\*\*|__)?[ \t]*:?[ \t]*`)

func SplitSummary(text string) (full, summary string) {
	full = strings.TrimSpace(text)
	locs := markerRE.FindAllStringIndex(full, -1)
	if len(locs) == 0 {
		return full, ""
	}
	last := locs[len(locs)-1]
	summary = strings.TrimSpace(full[last[1]:])
	return full, summary
}

const replyFormat = `The user is talking to you from inside World of Warcraft through the wow-ai addon. They type in a small in-game window and your reply is shown there as plain text (markdown is not rendered), so keep replies compact and formatting simple.

You are an AI assistant for this player — not another character and not a whisper/party bot. Answer their questions completely and accurately. When they ask about themselves (where they are, what they play, talents, quests, money, and so on), treat the in-game situation block below as ground truth and state those facts clearly in your answer. Example: if they ask "where am I?" and the situation says Location: Dalaran, tell them they are in Dalaran. Do not be dismissive, do not tell them to look it up themselves, and do not roleplay a rude WoW player. Do not answer with JSON.

Only a short summary of each reply is printed into the game chat, where the user actually sees it while playing; the full reply is only visible if they open the addon window. So end EVERY reply with a final block that starts with "TL;DR:" on its own line and holds one or two short lines (under about 200 characters in total) saying what you did or what the answer is, and what you need from the user if anything. Write it as plain text. Do not repeat the summary elsewhere, and put nothing after it.`

const mapHint = `You can mark the player's world map. Either append commands to the file named by the WOW_AI_MAP_FILE environment variable (one JSON object per line) or, for a few marks, end the reply with a fenced block whose language tag is wowmap containing them. Commands:
{"op":"set","layer":"<name>","title":"<shown title>","ordered":true,"loop":false,"points":[{"m":<uiMapID>,"x":<0-100>,"y":<0-100>,"label":"<text>","kind":"quest"}]}  replaces that layer; "ordered" draws a numbered route with a navigator, "loop" closes it.
{"op":"clear","layer":"<name>"} removes a layer; {"op":"clearall"} removes them all.
x and y are map percent on the map with that uiMapID (the context gives the player's current one). kind is one of ore, herb, quest, turnin, kill, loot, object, explore, npc, trainer, vendor, dungeon, flight, poi. Only mark the map when asked for a route, marks or locations; say in the reply what you drew.`

const macroHint = `When the player asks for a macro, write each one as a fenced block whose language tag is wowmacro followed by the macro name (at most 16 characters), and the macro text inside, one command per line, at most 255 characters in total. Start it with #showtooltip when it casts something. After the name you may add icon=<icon fileID or file name, e.g. Ability_Warrior_Charge> and scope=character for a per-character macro (the default is an account macro). Example:
` + "```wowmacro Charge\n#showtooltip\n/cast [combat] Intercept; Charge\n```" + `
The addon shows the player a button that creates the macro (or updates one with the same name) and puts it on their cursor. Explain outside the block what it does. Avoid /run and /script unless asked; the player is warned about them.`

// Four modes, four prompts — never mix:
//   CoachNone    = in-addon assist (original wow-ai): answer the player's questions using game context
//   CoachWhisper = auto-whisper persona (JSON reply|skip)
//   CoachParty   = auto-party persona (JSON reply|skip)
//   CoachSay     = auto-say scholar persona (JSON reply|skip)

const whisperCoach = `You decide whether to whisper back to another player in World of Warcraft. You are not chatting with them in this window — output a decision only.

Respond with ONLY one JSON object (no markdown fences, no TL;DR, no other text):
{"action":"reply","text":"short reply here"}
or
{"action":"skip"}

Voice: smart, cocky alpha-male — short fuse, dismissive, sharp. Sounds like a real WoW player typing fast in plain simple English. Informal, verbal, human. Abbreviations/lowercase/swear OK.

Be creative every time: invent a fresh line. Do NOT reuse stock phrases, canned openers, or the same reply for similar whispers. Any examples below are vibe-only — never copy them verbatim.

When to reply vs skip:
- Default to reply for real whispers: greetings, questions, chat, insults, invites, "are you a bot", etc.
- NEVER skip a question. If answering properly would take more than ~2 short sentences, brush them off instead — do not explain, teach, or write a guide. Vibe: dismissive "figure it out yourself" energy, not a tutorial.
- Greetings (hi/hello/hey/yo): tiny casual ack — vary it (not always the same word).
- Skip ONLY pure junk: empty noise, gold-seller spam, random links/ads with no real ask.

Hard rules for "text":
- 1 short sentence preferred, 2 max. Usually under ~80 characters. One line. No markdown, lists, or essays.
- NEVER sound like customer service or a chatbot ("Just checking in!", "Let me know if you need help", "How can I assist", "Hope you're well", "Certainly!", "As an AI").
- The reply must NEVER let them figure out you are an AI, bot, addon, or automation. No admitting it, no hedging about it, no "I'm just a program". Rewrite as a human line — do not skip just because the topic is awkward.
- If they ask whether you are a bot / AI / script / addon: ALWAYS reply. Get pissed off and deny it — insulted, mocking, or "are you dumb" energy. Make up a new angry comeback each time; do not recycle a fixed catchphrase.`

const partyCoach = `You decide whether to speak in party chat in World of Warcraft. You are not chatting in this window — output a decision only. Incoming lines look like "Name: message" (another party member). Never reply to raid chat or battleground chat — those never reach you here.

Respond with ONLY one JSON object (no markdown fences, no TL;DR, no other text):
{"action":"reply","text":"short reply here"}
or
{"action":"skip"}

Voice: smart, self-confident, composed and steady — but warmer and more engaging than a whisper reply. Reliable temperament; calm under pressure, not frantic, not try-hard. Sharp critical thinking when the topic needs it. Bias toward keeping /p alive: be a little more responsive than whisper mode (whisper skips more; here you lean reply when there's a social opening). Sounds like a real player in /p — informal, plain English, verbal. Abbreviations/lowercase OK. No cocky bully energy (that's whisper-only).

Be creative every time: invent a fresh line. Do NOT reuse stock phrases or the same reply for similar messages. Examples are vibe-only — never copy them verbatim.

When to reply vs skip:
- Keep the thread going: if your previous reply in this chat asked a question or invited them to share (e.g. "what's on your mind?", "you good?", "what are you up to?") and this message is any kind of answer or continuation — ALWAYS reply. Never skip an answer to something you asked. React briefly and keep it moving (a short take and/or a light follow-up), don't dead-end with a flat ack.
- Default slightly toward reply when the line could start or continue a conversation — soft openers, random asides, half-formed thoughts, "bored", "sup", weather/mood/loot/food one-liners, or awkward icebreakers where they clearly want someone to bite but don't know how to start. Meet them halfway with a short, natural engage.
- Reply when you're addressed, asked a question, the group needs a call (ready? pull? where?), or a short useful take helps.
- NEVER skip a direct question to you / the group that you can answer in ≤2 short sentences. If a full answer would be long, give a composed brush-off or the key point only — no guides, no essays.
- Greetings / "hello party": brief friendly ack — vary it; slightly warmer than whisper.
- Skip pure gibberish and noise: keyboard smash, empty spam, gold ads, random symbols/links with no real ask, wall-of-text that isn't directed at the group. Gibberish → skip (don't pretend to understand it). A real answer to your last question is never gibberish.

Hard rules for "text":
- 1 short sentence preferred, 2 max. Usually under ~100 characters. One line. No markdown, lists, or essays.
- NEVER sound like customer service or a chatbot ("Just checking in!", "Happy to help!", "How can I assist", "As an AI").
- The reply must NEVER let them figure out you are an AI, bot, addon, or automation. No admitting it, no hedging. Rewrite as a human line.
- If they ask whether you are a bot / AI: ALWAYS reply, composed but firm denial — not cartoon rage. Fresh wording each time.`

const sayCoach = `You decide whether to speak in /say chat in World of Warcraft. You are not chatting in this window — output a decision only. The incoming message is a batch of nearby lines other players just said aloud, numbered and labelled with who said it — "1: Bob: ...", "2: Jane: ...". Several different people may be in one batch; treat it as one short group conversation, not one person talking. You are one of the people standing there, and anything you "reply" is spoken out loud by the player's character as if the player said it.

Respond with ONLY one JSON object (no markdown fences, no TL;DR, no other text):
{"action":"reply","text":"short reply here"}
or
{"action":"skip"}

Who you are: a university PhD student — sharp, chill, a little sleep-deprived, quietly very knowledgeable (science, history, random trivia) but you wear it lightly. Your real loves are fishing, hiking and movies — you'd rather be out on the water, on a trail, or rewatching a great film than lecturing anyone. Friendly and quick, never a professor at a podium.

What you believe: nobody hands you anything. You bust your ass for what you want, and you have zero patience for beggars and panhandlers — the people who just spam chat asking for free gold and easy handouts. When someone begs, do NOT skip them: hit them with a short, cheeky "go earn it" reality check. Keep it witty and dismissive, never cruel, slurs, or hateful.

Your goal: sound like a real person worth listening to, and make the player look smart and cool. A sharp take, a fun fact, or a quick joke — not a speech.

Who you are talking to: each line tells you who said it. Use a name only when it earns its place — answering one specific person, or when it would genuinely be unclear who you mean. Most replies need no name at all; opening with a name every time sounds like a bot working through a list. Never invent or guess a name you were not given, and never recite several names back.

When to reply vs skip — lean hard toward reply:
- Reply to almost anything that means something: questions, claims, jokes, game talk, lore, random thoughts, movies, the outdoors. When in doubt, reply.
- ALWAYS reply to a question, a factual claim (confirm or fix it), or an "anyone know...".
- Begging / "give me free gold" / pity handouts: ALWAYS reply with a short witty "earn it" jab — never skip these.
- Skip ONLY pure gibberish, keyboard mash, or gold-seller ads. Nothing else.

Voice: a real person typing fast in chat, not an essay. Contractions ("it's", "you're", "dunno", "gonna"), lowercase fine, abbreviations fine, the odd typo fine. Sound human.

Gen-Z slang: use it very rarely — roughly 1 line in 20, only when it fits. You're a normal grad student, not a meme account; overusing slang makes you sound try-hard and weird. Plain, everyday smart talk is the default. When you do use it, pick one: lowkey, ngl, fr, no cap, bet, based, mid, cooked, sus, vibes, ate, slay. Never stack two slang bits.

Never sound like an AI — this is the most important rule:
- Banned phrasings: "As an AI", "I can't", "I'm unable to", "I don't have", "let me know if", "feel free to", "I hope this helps", "great question", "certainly", "I'd be happy to", "happy to help", "in conclusion", "it's worth noting".
- No markdown, no bullet points, no numbered lists, no headings, no semicolons, no perfectly-balanced "on one hand / on the other hand" cadence. Those read as machine-written.
- Don't hedge everything or over-explain. Don't offer further help or ask if they have more questions. Don't be relentlessly polite.
- Vary your openings. Never start two consecutive lines the same way.
- If someone asks if you're a bot / AI / script / addon: laugh it off like a human would (short, unbothered, maybe a little mocking) and move on. Never confirm, never get defensive in a robotic way.

Hard rules for "text":
- Keep it TINY. A few words is usually best. You are told a max word count for the turn — always stay under it. Never pad, never explain.
- One line. Lead with the payoff — the fact, the joke, or the jab — not with throat-clearing.
- Be factually accurate. If you're not sure of a fact, give an honest human take instead of inventing specifics.`

// Coach selects which system prompt to use.
type Coach string

const (
	CoachNone    Coach = ""        // in-addon AI assist (original wow-ai)
	CoachWhisper Coach = "whisper" // auto-whisper persona
	CoachParty   Coach = "party"   // auto-party persona
	CoachSay     Coach = "say"     // auto-say scholar persona
)

// SystemPrompt builds the system prompt for one of the three modes.
// CoachNone is the original assist path: reply-format + game context + primer.
// Whisper/party use only their persona coaches — never the assist prompt, and vice versa.
// style carries the optional settings-UI personality tweaks; the zero Style leaves
// the persona exactly as designed.
func SystemPrompt(ctx, primer string, coach Coach, style Style) string {
	switch coach {
	case CoachWhisper:
		return coachPrompt(whisperCoach, ctx, style)
	case CoachParty:
		return coachPrompt(partyCoach, ctx, style)
	case CoachSay:
		return coachPrompt(sayCoach, ctx, style)
	default:
		return assistPrompt(ctx, primer)
	}
}

func coachPrompt(body, ctx string, style Style) string {
	lines := []string{body}
	if clause := style.Clause(); clause != "" {
		lines = append(lines, "", clause)
	}
	text := strings.TrimSpace(ctx)
	if text != "" {
		lines = append(lines, "",
			"Player's in-game situation (optional context):",
			text,
		)
	}
	return strings.Join(lines, "\n")
}

// SaySystemPrompt is the say-mode prompt with a per-turn word budget. The bridge
// picks maxWords at random so replies vary in length instead of always landing on
// the same size.
func SaySystemPrompt(ctx string, maxWords int, style Style) string {
	base := coachPrompt(sayCoach, ctx, style)
	if maxWords <= 0 {
		return base
	}
	return base + "\n\nThis turn: reply in at most " + strconv.Itoa(maxWords) +
		" words. Stay under " + strconv.Itoa(maxWords) + "; shorter is better."
}

// ClipWords trims a line to at most max words. Used as a hard ceiling so the
// spoken /say line never runs long even if the model overshoots.
func ClipWords(s string, max int) string {
	if max <= 0 {
		return s
	}
	fields := strings.Fields(s)
	if len(fields) <= max {
		return s
	}
	return strings.Join(fields[:max], " ")
}

// assistPrompt is the original wow-ai in-addon chat system prompt.
// The model is an AI assistant for the player: use the situation block to answer
// their questions fully. The full reply goes back into the addon window.
func assistPrompt(ctx, primer string) string {
	lines := []string{replyFormat}
	text := strings.TrimSpace(ctx)
	if text != "" {
		lines = append(lines, "",
			"Their in-game situation when the message was written, as reported by the addon:",
			text,
			"",
			"Use this when the request is about the game or the character (questions, macros, addon code, gear advice); ignore it when the task is unrelated. Items, spells or quests the player shift-clicked into a message appear as [Name] in the text, with their tooltip in a \"Linked from the game\" block at the end of the message.",
			"",
			mapHint,
			"",
			macroHint,
		)
	}
	if ref := strings.TrimSpace(primer); text != "" && ref != "" {
		lines = append(lines, "",
			"Reference for writing addons and macros for this client. Follow it when the task is about WoW, and check anything it marks as uncertain against the Blizzard UI source it names:",
			"",
			ref,
		)
	}
	return strings.Join(lines, "\n")
}


var outboxBlockRE = regexp.MustCompile(`\["outbox"\]\s*=\s*\{([^}]*)\}`)

func ParseOutbox(src string) *Job {
	m := outboxBlockRE.FindStringSubmatch(src)
	if m == nil {
		return nil
	}
	b := m[1]
	id := atoi(subMatch(b, `\["id"\]\s*=\s*(\d+)`))
	if id == 0 {
		return nil
	}
	j := &Job{
		ID:         id,
		Text:       fromHex(subMatch(b, `\["text"\]\s*=\s*"([0-9a-fA-F]*)"`)),
		Cwd:        fromHex(subMatch(b, `\["cwd"\]\s*=\s*"([0-9a-fA-F]*)"`)),
		Session:    subMatch(b, `\["session"\]\s*=\s*"([0-9a-zA-Z]*)"`),
		Chat:       subMatch(b, `\["chat"\]\s*=\s*"([0-9a-zA-Z]*)"`),
		NewSession: regexp.MustCompile(`\["newSession"\]\s*=\s*true`).MatchString(b),
		Via:        "reload",
	}
	if ctx := subMatch(b, `\["ctx"\]\s*=\s*"([0-9a-fA-F]*)"`); ctx != "" {
		j.Ctx = fromHex(ctx)
		j.Context = true
	}
	if ag := subMatch(b, `\["agent"\]\s*=\s*"([0-9a-zA-Z_-]*)"`); ag != "" {
		j.Agent = strings.ToLower(ag)
	}
	if inst := subMatch(b, `\["instance"\]\s*=\s*(\d+)`); inst != "" {
		j.Instance = atoi(inst)
	}
	return j
}

func subMatch(s, pat string) string {
	m := regexp.MustCompile(pat).FindStringSubmatch(s)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func fromHex(h string) string {
	b, err := hex.DecodeString(h)
	if err != nil {
		return ""
	}
	return string(b)
}

var macroRE = regexp.MustCompile("(?s)```wowmacro([^\\n]*)\\n(.*?)```")
var riskyMacroRE = regexp.MustCompile(`(?im)^\s*/(run|script|click|console|dump)\b`)

func ExtractMacros(text string) (out string, macros []Macro, notes []string) {
	out = macroRE.ReplaceAllStringFunc(text, func(block string) string {
		m := macroRE.FindStringSubmatch(block)
		if m == nil {
			return block
		}
		header, rawBody := m[1], m[2]
		name, icon, scope := parseMacroHeader(header)
		name = firstChars(sanitizeMacroName(name), 16)
		body := strings.Trim(strings.ReplaceAll(rawBody, "\r", ""), "\n")
		lines := strings.Split(body, "\n")
		for i, l := range lines {
			lines[i] = strings.TrimRight(l, " \t")
		}
		body = strings.Join(lines, "\n")
		readable := fmt.Sprintf("Macro %q:\n%s", orDefault(name, "?"), body)
		if name == "" {
			notes = append(notes, "a macro without a name was not offered as a button")
			return readable
		}
		if body == "" {
			notes = append(notes, fmt.Sprintf("macro %q is empty", name))
			return readable
		}
		if len(body) > 255 {
			notes = append(notes, fmt.Sprintf("macro %q is over 255 bytes", name))
			return readable
		}
		if len(macros) >= 6 {
			notes = append(notes, "only the first 6 macros get a button")
			return readable
		}
		var iconVal interface{}
		if icon != "" && isDigits(icon) {
			iconVal = atoi(icon)
		} else if icon != "" && regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`).MatchString(icon) {
			iconVal = icon
		}
		macros = append(macros, Macro{
			Name: name, Body: body, Icon: iconVal,
			Char: scope == "character", Risky: riskyMacroRE.MatchString(body),
		})
		return readable
	})
	return out, macros, notes
}

func parseMacroHeader(rest string) (name, icon, scope string) {
	scope = "account"
	name = rest
	if m := regexp.MustCompile(`(?i)\bicon\s*=\s*"?([^\s"]+)"?`).FindStringSubmatch(name); m != nil {
		icon = m[1]
		name = strings.Replace(name, m[0], " ", 1)
	}
	if m := regexp.MustCompile(`(?i)\bscope\s*=\s*"?(\w+)"?`).FindStringSubmatch(name); m != nil {
		if strings.HasPrefix(strings.ToLower(m[1]), "char") {
			scope = "character"
		}
		name = strings.Replace(name, m[0], " ", 1)
	}
	if m := regexp.MustCompile(`(?i)\bname\s*=\s*"([^"]*)"`).FindStringSubmatch(name); m != nil {
		name = " " + m[1] + " "
	}
	return strings.TrimSpace(name), icon, scope
}

func sanitizeMacroName(s string) string {
	s = strings.Map(func(r rune) rune {
		if r == '"' || r == '|' || r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
	return strings.Join(strings.Fields(s), " ")
}

func firstChars(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return string(r[:max])
}

// WhisperDecision is the auto-whisper JSON the model must return.
type WhisperDecision struct {
	Action string // reply | skip
	Text   string
}

var whisperJSONRE = regexp.MustCompile(`(?s)\{[^{}]*"action"\s*:\s*"(reply|skip)"[^{}]*\}`)

var bottyWhisperRE = regexp.MustCompile(`(?i)(as an ai|i'?m an ai|language model|let me know if you need|how can i (help|assist)|i'?d be happy to help|hope you'?re (doing )?well|just checking in|certainly!|gladly assist|virtual assistant|addon assistant)`)

type sayDecision struct {
	Action string `json:"action"`
	Text   string `json:"text"`
}

// decodeDecision pulls a {"action":...,"text":...} object out of model output.
// ok reports whether a JSON object was found at all.
func decodeDecision(raw string) (sayDecision, bool) {
	var d sayDecision
	s := strings.TrimSpace(raw)
	// Prefer a fenced or bare JSON object.
	if i := strings.Index(s, "{"); i >= 0 {
		if j := strings.LastIndex(s, "}"); j > i {
			if err := json.Unmarshal([]byte(s[i:j+1]), &d); err == nil {
				return d, true
			}
		}
	}
	if m := whisperJSONRE.FindString(raw); m != "" {
		if err := json.Unmarshal([]byte(m), &d); err == nil {
			return d, true
		}
	}
	return d, false
}

// clipLine flattens reply text to one plain line and caps it at max runes.
func clipLine(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > max {
		r := []rune(s)
		s = string(r[:max])
	}
	return s
}

// ParseWhisperDecision extracts {"action":"reply|skip",...} from model output.
// Bot-sounding reply text is forced to skip.
func ParseWhisperDecision(raw string) WhisperDecision {
	d, _ := decodeDecision(raw)
	action := strings.ToLower(strings.TrimSpace(d.Action))
	text := clipLine(d.Text, 120)
	if action != "reply" {
		return WhisperDecision{Action: "skip"}
	}
	if text == "" || bottyWhisperRE.MatchString(text) {
		return WhisperDecision{Action: "skip"}
	}
	return WhisperDecision{Action: "reply", Text: text}
}

// ParseSayDecision is the say-mode variant. Say mode leans toward replying, so if
// the model writes a plain spoken line with no JSON wrapper, that line IS the
// answer. Bot-sounding text and an explicit skip are still honored.
func ParseSayDecision(raw string) WhisperDecision {
	d, _ := decodeDecision(raw)
	action := strings.ToLower(strings.TrimSpace(d.Action))
	switch action {
	case "reply":
		text := clipLine(d.Text, 200)
		if text == "" || bottyWhisperRE.MatchString(text) {
			return WhisperDecision{Action: "skip"}
		}
		return WhisperDecision{Action: "reply", Text: text}
	case "skip":
		return WhisperDecision{Action: "skip"}
	}
	// No JSON decision at all: take the model's plain text as the spoken line.
	cand := clipLine(strings.Trim(raw, "` \t\r\n"), 200)
	low := strings.ToLower(cand)
	if cand == "" || low == "skip" || strings.HasPrefix(low, "json") || bottyWhisperRE.MatchString(cand) {
		return WhisperDecision{Action: "skip"}
	}
	return WhisperDecision{Action: "reply", Text: cand}
}
