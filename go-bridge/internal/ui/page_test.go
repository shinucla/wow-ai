package ui

import (
	"bytes"
	"strings"
	"testing"
)

// The settings page is one big html/template literal; executing it here catches
// quoting/escaping mistakes long before a browser would.
func TestPageTemplateRendersTabs(t *testing.T) {
	var buf bytes.Buffer
	if err := pageTmpl.Execute(&buf, nil); err != nil {
		t.Fatalf("executing page template: %v", err)
	}
	page := buf.String()

	for _, want := range []string{
		`data-tab="status"`,
		`data-tab="game"`,
		`data-tab="llm"`,
		`data-tab="whisper"`,
		`data-tab="party"`,
		`data-tab="say"`,
		`data-tab="logging"`,
		`id="tab-say"`,
		`id="sayReplyChance"`,
		`id="sayCollectSeconds"`,
		`id="whisperEnabled"`,
		`id="partyEnabled"`,
		`id="whisperPersonality"`,
		`id="whisperEducation"`,
		`id="whisperCharacteristics"`,
		`id="partyPersonality"`,
		`id="sayPersonality"`,
		`id="sayEducation"`,
		`id="sayCharacteristics"`,
		`value="deepseek"`,
	} {
		if !strings.Contains(page, want) {
			t.Fatalf("settings page is missing %s", want)
		}
	}

	// The AddOns path must reach the browser with escaped backslashes so the JS
	// string keeps single backslashes.
	if !strings.Contains(page, `'\\WoWAI\\Inbox.lua'`) {
		t.Fatalf("inbox file path lost its backslash escaping")
	}
}
