package ui

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// The dropdown catalogs are served from the protocol package so labels cannot
// drift from the clauses the prompt builder actually uses.
func TestStyleOptionsEndpoint(t *testing.T) {
	rec := httptest.NewRecorder()
	(&App{}).handleStyleOptions(rec, httptest.NewRequest("GET", "/api/style-options", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Personalities []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
		} `json:"personalities"`
		Education       []map[string]string `json:"education"`
		Characteristics []map[string]string `json:"characteristics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if len(body.Personalities) < 5 || len(body.Education) < 3 || len(body.Characteristics) < 5 {
		t.Fatalf("too few options: %d/%d/%d",
			len(body.Personalities), len(body.Education), len(body.Characteristics))
	}
	// The first entry of each list must be the "leave it as designed" default.
	for _, list := range [][]map[string]string{body.Education, body.Characteristics} {
		if list[0]["key"] != "" {
			t.Fatalf("first option should be the empty default, got %q", list[0]["key"])
		}
	}
	if body.Personalities[0].Key != "" {
		t.Fatalf("first personality should be the empty default, got %q", body.Personalities[0].Key)
	}
}
