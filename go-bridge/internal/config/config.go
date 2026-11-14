package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the Go bridge settings (JSON next to the binary or in AppData).
type Config struct {
	AddonDir            string        `json:"addonDir"`
	SavedVariablesFile  string        `json:"savedVariablesFile"`
	InboxFile           string        `json:"inboxFile"`
	DefaultCwd          string        `json:"defaultCwd"`
	TocInterface        string        `json:"tocInterface"`
	Slots               int           `json:"slots"`
	ActMax              int           `json:"actMax"`
	PresenceMax         int           `json:"presenceMax"`
	PresenceIntervalMs  int           `json:"presenceIntervalMs"`
	MaxParallel         int           `json:"maxParallel"`
	ProgressWriteMs    int           `json:"progressWriteMs"`
	MaxInstances       int           `json:"maxInstances"`
	PrimerFile         string        `json:"primerFile"`
	Debug              bool          `json:"debug"` // when true, Status log shows capture/startup/diagnostics
	Capture            CaptureConfig `json:"capture"`
	LLM                LLMConfig     `json:"llm"`
	Modes              ModesConfig   `json:"modes"`
	Profile            ProfileConfig `json:"profile"`
}

// ProfileConfig is the player's own reference material for the persona modes — the
// "Myself" tab. It is free text per category, injected into the whisper/party/say
// prompts so the personas sound like the same real person and can answer things like
// "what do you think of that guild?" from the player's actual views. The per-mode
// reply style stays dominant; this is background used only when it fits. Tone and
// slang are deliberately NOT here — those live in each mode's Reply style.
type ProfileConfig struct {
	About     string `json:"about"`     // who I am: age, work/study, region, how long I've played, main
	Interests string `json:"interests"` // interests & hobbies, in game and out
	Opinions  string `json:"opinions"`  // opinions & hot takes, in game and general
}

// ModesConfig holds the per-mode bridge switches and tuning for the persona
// modes (auto-whisper, auto-party, and surrounding-/say).
type ModesConfig struct {
	Whisper ModeConfig    `json:"whisper"`
	Party   ModeConfig    `json:"party"`
	Say     SayModeConfig `json:"say"`
}

// ModeConfig is a bridge-side master switch for one persona mode. The in-game
// addon checkbox still decides whether a job is sent at all; this lets the
// bridge ignore jobs for a mode without re-installing the addon.
//
// ReplyChance is the percent of incoming lines the mode answers (0..100); the
// rest are skipped on the bridge side without asking the model. Nil means 100.
// MinWords/MaxWords bound the reply length; a random budget in that range is
// handed to the model each turn and enforced as a hard clip.
//
// Personality/Education/Characteristics are optional style keys (see
// protocol.Personalities and friends). Empty means "persona as designed".
type ModeConfig struct {
	Enabled         *bool  `json:"enabled"`
	ReplyChance     *int   `json:"replyChance"`
	MinWords        *int   `json:"minWords"`
	MaxWords        *int   `json:"maxWords"`
	Personality     string `json:"personality"`
	Education       string `json:"education"`
	Characteristics string `json:"characteristics"`
	Slang           string `json:"slang"`
}

// SayModeConfig tunes the surrounding-/say collector.
type SayModeConfig struct {
	Enabled         *bool  `json:"enabled"`        // answer collected /say at all
	ReplyChance     *int   `json:"replyChance"`    // percent of batches that get answered (0..100)
	CollectSeconds  *int   `json:"collectSeconds"` // how long to gather lines before deciding
	MinWords        *int   `json:"minWords"`       // shortest say reply
	MaxWords        *int   `json:"maxWords"`       // longest say reply
	Personality     string `json:"personality"`
	Education       string `json:"education"`
	Characteristics string `json:"characteristics"`
	Slang           string `json:"slang"`
}

// Bool and Int return pointers for the nullable mode fields, whose nil value
// means "not configured, use the default".
func Bool(v bool) *bool { return &v }
func Int(v int) *int    { return &v }

type CaptureConfig struct {
	Enabled      bool   `json:"enabled"`
	ProcessName  string `json:"processName"`
	CellPx       int    `json:"cellPx"`
	CellsPerRow  int    `json:"cellsPerRow"`
	MaxRows      int    `json:"maxRows"`
	IntervalMs   int    `json:"intervalMs"`
	WindowTitle  string `json:"windowTitle"`
}

type LLMConfig struct {
	Provider    string `json:"provider"` // openai | deepseek | anthropic | openai-compatible
	BaseURL     string `json:"baseURL"`
	APIKey      string `json:"apiKey"`
	Model       string `json:"model"`
	AgentName   string `json:"agentName"` // shown in-game (default: api)
	MaxTokens   int    `json:"maxTokens"`
	Temperature float64 `json:"temperature"`
}

func Default() Config {
	return Config{
		TocInterface:       "30300",
		Slots:              200,
		ActMax:             60,
		PresenceMax:        2000,
		PresenceIntervalMs: 30000,
		MaxParallel:        3,
		ProgressWriteMs:    3000,
		MaxInstances:       4,
		Capture: CaptureConfig{
			Enabled:     true,
			ProcessName: "Wow",
			CellPx:      4,
			CellsPerRow: 200,
			MaxRows:     48,
			IntervalMs:  250,
		},
		LLM: LLMConfig{
			Provider:    "openai",
			BaseURL:     "https://api.openai.com/v1",
			Model:       "gpt-4o-mini",
			AgentName:   "api",
			MaxTokens:   2048,
			Temperature: 0.7,
		},
		Modes: ModesConfig{
			Whisper: ModeConfig{Enabled: Bool(true), ReplyChance: Int(100), MinWords: Int(4), MaxWords: Int(13)},
			Party:   ModeConfig{Enabled: Bool(true), ReplyChance: Int(100), MinWords: Int(4), MaxWords: Int(13)},
			Say: SayModeConfig{
				Enabled:        Bool(true),
				ReplyChance:    Int(100),
				CollectSeconds: Int(10),
				MinWords:       Int(4),
				MaxWords:       Int(13),
			},
		},
	}
}

func Path() (string, error) {
	if p := os.Getenv("WOW_AI_GO_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "wow-ai-bridge", "config.json"), nil
}

func Load() (Config, string, error) {
	cfg := Default()
	path, err := Path()
	if err != nil {
		return cfg, "", err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, path, nil
	}
	if err != nil {
		return cfg, path, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, path, err
	}
	cfg.ApplyDefaults()
	return cfg, path, nil
}

// ApplyDefaults fills zero fields with sensible defaults.
func (c *Config) ApplyDefaults() {
	d := Default()
	if c.Slots == 0 {
		c.Slots = d.Slots
	}
	if c.ActMax == 0 {
		c.ActMax = d.ActMax
	}
	if c.PresenceMax == 0 {
		c.PresenceMax = d.PresenceMax
	}
	if c.PresenceIntervalMs == 0 {
		c.PresenceIntervalMs = d.PresenceIntervalMs
	}
	if c.MaxParallel == 0 {
		c.MaxParallel = d.MaxParallel
	}
	if c.ProgressWriteMs == 0 {
		c.ProgressWriteMs = d.ProgressWriteMs
	}
	if c.MaxInstances == 0 {
		c.MaxInstances = d.MaxInstances
	}
	if c.TocInterface == "" {
		c.TocInterface = d.TocInterface
	}
	if c.Capture.ProcessName == "" {
		c.Capture.ProcessName = d.Capture.ProcessName
	}
	if c.Capture.CellPx == 0 {
		c.Capture.CellPx = d.Capture.CellPx
	}
	if c.Capture.CellsPerRow == 0 {
		c.Capture.CellsPerRow = d.Capture.CellsPerRow
	}
	if c.Capture.MaxRows == 0 {
		c.Capture.MaxRows = d.Capture.MaxRows
	}
	if c.Capture.IntervalMs == 0 {
		c.Capture.IntervalMs = d.Capture.IntervalMs
	}
	if c.LLM.Provider == "" {
		c.LLM.Provider = d.LLM.Provider
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = d.LLM.BaseURL
	}
	if c.LLM.Model == "" {
		c.LLM.Model = d.LLM.Model
	}
	if c.LLM.AgentName == "" {
		c.LLM.AgentName = d.LLM.AgentName
	}
	if c.LLM.MaxTokens == 0 {
		c.LLM.MaxTokens = d.LLM.MaxTokens
	}
	if c.AddonDir != "" {
		c.InboxFile = filepath.Join(c.AddonDir, "WoWAI", "Inbox.lua")
	}
	if c.Modes.Whisper.Enabled == nil {
		c.Modes.Whisper.Enabled = d.Modes.Whisper.Enabled
	}
	if c.Modes.Whisper.ReplyChance == nil {
		c.Modes.Whisper.ReplyChance = d.Modes.Whisper.ReplyChance
	}
	if c.Modes.Whisper.MinWords == nil {
		c.Modes.Whisper.MinWords = d.Modes.Whisper.MinWords
	}
	if c.Modes.Whisper.MaxWords == nil {
		c.Modes.Whisper.MaxWords = d.Modes.Whisper.MaxWords
	}
	if c.Modes.Party.Enabled == nil {
		c.Modes.Party.Enabled = d.Modes.Party.Enabled
	}
	if c.Modes.Party.ReplyChance == nil {
		c.Modes.Party.ReplyChance = d.Modes.Party.ReplyChance
	}
	if c.Modes.Party.MinWords == nil {
		c.Modes.Party.MinWords = d.Modes.Party.MinWords
	}
	if c.Modes.Party.MaxWords == nil {
		c.Modes.Party.MaxWords = d.Modes.Party.MaxWords
	}
	if c.Modes.Say.Enabled == nil {
		c.Modes.Say.Enabled = d.Modes.Say.Enabled
	}
	if c.Modes.Say.ReplyChance == nil {
		c.Modes.Say.ReplyChance = d.Modes.Say.ReplyChance
	}
	if c.Modes.Say.CollectSeconds == nil {
		c.Modes.Say.CollectSeconds = d.Modes.Say.CollectSeconds
	}
	if c.Modes.Say.MinWords == nil {
		c.Modes.Say.MinWords = d.Modes.Say.MinWords
	}
	if c.Modes.Say.MaxWords == nil {
		c.Modes.Say.MaxWords = d.Modes.Say.MaxWords
	}
}

// ModeEnabled reports whether a persona mode is switched on. Accepted names are
// "whisper", "party", and "say". Unknown names default to enabled.
func (c Config) ModeEnabled(mode string) bool {
	switch mode {
	case "whisper":
		return boolVal(c.Modes.Whisper.Enabled, true)
	case "party":
		return boolVal(c.Modes.Party.Enabled, true)
	case "say":
		return boolVal(c.Modes.Say.Enabled, true)
	}
	return true
}

// ModeStyle returns the chosen personality/education/characteristics/slang keys for
// a persona mode ("whisper", "party", "say"). Empty strings mean "as designed".
func (c Config) ModeStyle(mode string) (personality, education, characteristics, slang string) {
	switch mode {
	case "whisper":
		m := c.Modes.Whisper
		return m.Personality, m.Education, m.Characteristics, m.Slang
	case "party":
		m := c.Modes.Party
		return m.Personality, m.Education, m.Characteristics, m.Slang
	case "say":
		m := c.Modes.Say
		return m.Personality, m.Education, m.Characteristics, m.Slang
	}
	return "", "", "", ""
}

// ModeReplyChancePct is the percent of incoming lines a persona mode ("whisper",
// "party", "say") answers; the rest are skipped on the bridge side without asking
// the model. Nil means 100. A mode without a setting (assist) is always 100.
func (c Config) ModeReplyChancePct(mode string) int {
	switch mode {
	case "whisper":
		return clampInt(intVal(c.Modes.Whisper.ReplyChance, 100), 0, 100)
	case "party":
		return clampInt(intVal(c.Modes.Party.ReplyChance, 100), 0, 100)
	case "say":
		return clampInt(intVal(c.Modes.Say.ReplyChance, 100), 0, 100)
	}
	return 100
}

// SayReplyChancePct is the percent of collected say batches that get answered.
func (c Config) SayReplyChancePct() int {
	return c.ModeReplyChancePct("say")
}

// SayCollectSeconds is how long the say collector gathers before deciding.
func (c Config) SayCollectSeconds() int {
	return clampInt(intVal(c.Modes.Say.CollectSeconds, 10), 1, 300)
}

// ModeWordRange is the shortest and longest reply for a persona mode, in words.
// The bridge picks a random budget in that range per turn.
func (c Config) ModeWordRange(mode string) (int, int) {
	var lo, hi *int
	switch mode {
	case "whisper":
		lo, hi = c.Modes.Whisper.MinWords, c.Modes.Whisper.MaxWords
	case "party":
		lo, hi = c.Modes.Party.MinWords, c.Modes.Party.MaxWords
	case "say":
		lo, hi = c.Modes.Say.MinWords, c.Modes.Say.MaxWords
	default:
		return 4, 13
	}
	l := clampInt(intVal(lo, 4), 1, 100)
	h := clampInt(intVal(hi, 13), 1, 100)
	if h < l {
		l, h = h, l
	}
	return l, h
}

func intVal(p *int, def int) int {
	if p == nil {
		return def
	}
	return *p
}

func boolVal(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (c Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func (c Config) AgentIDs() []string {
	name := c.LLM.AgentName
	if name == "" {
		name = "api"
	}
	return []string{name}
}
