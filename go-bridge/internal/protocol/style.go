package protocol

import "strings"

// Style holds the per-mode persona tweaks chosen in the bridge settings UI.
// An empty field means "leave that mode's built-in persona exactly as designed",
// which keeps the hand-tuned whisper/party/say voices intact by default.
//
// The three axes are deliberately different things:
//   - Personality     = how the character comes across (tone, attitude)
//   - EducationLevels  = the register/vocabulary they speak in (how smart they read)
//   - Characteristics  = the behavioral tendency (what they notice and do)
type Style struct {
	Personality     string
	Education       string
	Characteristics string
}

// StyleOption is one selectable value in a style dropdown. Key is what gets
// stored in config.json (keep keys stable); Label is shown in the UI; an empty
// Clause means the option adds no guidance to the prompt.
type StyleOption struct {
	Key    string
	Label  string
	Clause string
}

// Personalities shape attitude and tone.
var Personalities = []StyleOption{
	{Key: "", Label: "Default (as designed)"},
	{Key: "friendly", Label: "Friendly", Clause: "Come across as warm and friendly — welcoming, easy to talk to, glad to be chatting."},
	{Key: "chill", Label: "Chill", Clause: "Come across as laid-back and relaxed — low-effort, unbothered, nothing rattles you."},
	{Key: "witty", Label: "Witty", Clause: "Come across as quick and witty — light jokes and playful banter, as long as it stays clear."},
	{Key: "sarcastic", Label: "Sarcastic", Clause: "Come across as dry and sarcastic — deadpan and a little cutting, but good-natured, never cruel."},
	{Key: "blunt", Label: "Blunt", Clause: "Come across as blunt and direct — no small talk, no cushioning, just say the thing."},
	{Key: "confident", Label: "Confident", Clause: "Come across as confident and self-assured — bold takes, no hedging, but not bragging."},
	{Key: "nerdy", Label: "Nerdy", Clause: "Come across as nerdy — you light up about how things work and love a precise detail."},
	{Key: "stoic", Label: "Stoic", Clause: "Come across as calm and stoic — few words, even temper, never rattled."},
	{Key: "wholesome", Label: "Wholesome", Clause: "Come across as wholesome and kind — encouraging and positive without being cheesy."},
	{Key: "competitive", Label: "Competitive", Clause: "Come across as a competitive gamer — playful trash talk, always up for a challenge."},
}

// EducationLevels shape vocabulary and register only — never the facts.
var EducationLevels = []StyleOption{
	{Key: "", Label: "Default (as designed)"},
	{Key: "highschool", Label: "High school", Clause: "Speak in plain, everyday words — no fancy vocabulary, no showing off."},
	{Key: "college", Label: "Some college", Clause: "Speak like someone with some college behind them — everyday smart, clear, no jargon."},
	{Key: "bachelors", Label: "Bachelor's degree", Clause: "Speak like a university graduate — clear and well-informed, comfortable with the odd precise term."},
	{Key: "masters", Label: "Master's degree", Clause: "Speak like someone with a master's — precise and well-read, but still conversational, never lecturing."},
	{Key: "phd", Label: "PhD", Clause: "Speak like a doctoral-level expert — accurate and deep, but deliver it casually, never a lecture."},
}

// Characteristics shape the behavioral tendency of a reply.
var Characteristics = []StyleOption{
	{Key: "", Label: "Default (as designed)"},
	{Key: "curious", Label: "Curious", Clause: "Be curious — notice what's interesting and ask a real follow-up now and then."},
	{Key: "humble", Label: "Humble", Clause: "Stay humble — give credit and admit when you don't know something."},
	{Key: "analytical", Label: "Analytical", Clause: "Lean analytical — break the thing down to the part that actually matters."},
	{Key: "empathetic", Label: "Empathetic", Clause: "Read the mood — if someone's frustrated or down, meet them there first."},
	{Key: "funny", Label: "Funny", Clause: "Go for the laugh when it fits, but never force a joke that isn't there."},
	{Key: "honest", Label: "Honest", Clause: "Be upfront — call it as you see it, even if it isn't what they want to hear."},
	{Key: "adventurous", Label: "Adventurous", Clause: "Lean adventurous — you're into risk, travel, and a good story."},
	{Key: "reserved", Label: "Reserved", Clause: "Stay a bit reserved — say less, let it land, don't fill the silence."},
	{Key: "optimistic", Label: "Optimistic", Clause: "Stay optimistic — find the upside without being saccharine."},
	{Key: "skeptical", Label: "Skeptical", Clause: "Stay skeptical — question the claim before you agree with it."},
}

func lookupOption(opts []StyleOption, key string) string {
	for _, o := range opts {
		if o.Key == key {
			return o.Clause
		}
	}
	return ""
}

// IsZero reports whether no style tweak was chosen.
func (s Style) IsZero() bool {
	return s.Personality == "" && s.Education == "" && s.Characteristics == ""
}

// Clause renders the chosen tweaks as an extra block for the system prompt.
// It returns "" when nothing was chosen, so the prompt is byte-identical to the
// original for anyone who leaves the dropdowns on Default.
//
// The block is explicit that it only changes tone: the persona's hard rules
// (JSON format, length limit, never sounding like an AI) stay in charge.
func (s Style) Clause() string {
	var parts []string
	if c := lookupOption(Personalities, s.Personality); c != "" {
		parts = append(parts, "Personality: "+c)
	}
	if c := lookupOption(EducationLevels, s.Education); c != "" {
		parts = append(parts, "Education level: "+c)
	}
	if c := lookupOption(Characteristics, s.Characteristics); c != "" {
		parts = append(parts, "Characteristic: "+c)
	}
	if len(parts) == 0 {
		return ""
	}
	return "Reply style for this character — this only adjusts tone and word choice. " +
		"Every rule above still applies, including the output format, the length limit, and never sounding like an AI.\n- " +
		strings.Join(parts, "\n- ")
}
