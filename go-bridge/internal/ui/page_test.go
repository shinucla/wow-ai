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
		`data-tab="myself"`,
		`data-tab="logging"`,
		`id="tab-say"`,
		`id="tab-myself"`,
		`id="profileAbout"`,
		`id="profileInterests"`,
		`id="profileOpinions"`,
		`id="whisperSlang"`,
		`id="partySlang"`,
		`id="saySlang"`,
		`id="sayReplyChance"`,
		`id="partyReplyChance"`,
		`id="whisperReplyChance"`,
		`id="whisperMinWords"`,
		`id="whisperMaxWords"`,
		`id="partyMinWords"`,
		`id="partyMaxWords"`,
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

	// Every tab with editable fields must be in SETTINGS_TABS, or its Save button
	// (the shared form-actions bar) never shows. Myself was missing once.
	i := strings.Index(page, "SETTINGS_TABS = [")
	if i < 0 {
		t.Fatal("SETTINGS_TABS not found in the page script")
	}
	tabs := page[i:]
	if j := strings.Index(tabs, "]"); j >= 0 {
		tabs = tabs[:j]
	}
	for _, want := range []string{"'game'", "'llm'", "'whisper'", "'party'", "'say'", "'myself'", "'logging'"} {
		if !strings.Contains(tabs, want) {
			t.Fatalf("SETTINGS_TABS is missing %s, so that tab has no Save button: %s", want, tabs)
		}
	}
}
