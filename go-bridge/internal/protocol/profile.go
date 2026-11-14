package protocol

import "strings"

// Profile is the player's own reference material for the persona modes — the
// "Myself" tab. Each field is free text; empty fields are omitted. A persona's own
// voice and rules (and the per-mode Style) stay dominant: this is background so the
// whisper/party/say personas sound like the same real person and can answer things
// like "what do you think of that guild?" from the player's actual views, instead of
// inventing a new personality every conversation. Tone and slang are deliberately
// not here — those belong to each mode's Style.
type Profile struct {
	About     string // who I am: age, work/study, region, how long I've played, main
	Interests string // interests & hobbies, in game and out
	Opinions  string // opinions & hot takes, in game and general
}

// IsZero reports whether no profile field is set.
func (p Profile) IsZero() bool {
	return p.clause() == ""
}

// clause renders the profile block appended to a persona prompt, or "" when unset.
// Profile fields may be free text or a short list; a line break stays a line break
// so "interests: fishing, hiking" style answers read naturally.
func (p Profile) clause() string {
	var parts []string
	add := func(label, v string) {
		if t := strings.TrimSpace(v); t != "" {
			parts = append(parts, label+": "+t)
		}
	}
	add("Who I am", p.About)
	add("Interests and hobbies", p.Interests)
	add("Opinions and hot takes", p.Opinions)
	if len(parts) == 0 {
		return ""
	}
	return "About the player you are speaking as — true facts, from the player:\n- " +
		strings.Join(parts, "\n- ") + "\n" +
		"Use this as background so you sound like the same real person, and draw on it when someone asks what you think. " +
		"Your voice and the rules above always win over it, so never contradict them to fit it in. " +
		"Bring up a detail only when it genuinely fits, and never list or recite these."
}
