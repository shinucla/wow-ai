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
}

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
	Provider    string `json:"provider"` // openai | anthropic | openai-compatible
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
