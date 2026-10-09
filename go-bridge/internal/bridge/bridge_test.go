package bridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

// fakeLLM is a minimal OpenAI-compatible endpoint that records the prompts it sees.
type fakeLLM struct {
	mu      sync.Mutex
	prompts []string
	systems []string
}

func (f *fakeLLM) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.prompts)
}

func (f *fakeLLM) lastPrompt() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.prompts) == 0 {
		return ""
	}
	return f.prompts[len(f.prompts)-1]
}

func (f *fakeLLM) lastSystem() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.systems) == 0 {
		return ""
	}
	return f.systems[len(f.systems)-1]
}

func (f *fakeLLM) server() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &req)
		prompt, system := "", ""
		for i := len(req.Messages) - 1; i >= 0; i-- {
			if prompt == "" && req.Messages[i].Role == "user" {
				prompt = req.Messages[i].Content
			}
			if req.Messages[i].Role == "system" {
				system = req.Messages[i].Content
				break
			}
		}
		f.mu.Lock()
		f.prompts = append(f.prompts, prompt)
		f.systems = append(f.systems, system)
		f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": `{"action":"reply","text":"Paris."}`}},
			},
		})
	})
	return httptest.NewServer(mux)
}

func newTestEngine(t *testing.T, baseURL string) (*Engine, string) {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Default()
	cfg.AddonDir = dir
	cfg.Slots = 5
	cfg.MaxInstances = 1
	cfg.MaxParallel = 2
	cfg.DefaultCwd = dir
	cfg.LLM.Provider = "openai"
	cfg.LLM.BaseURL = baseURL
	cfg.LLM.APIKey = "test-key"
	cfg.LLM.Model = "test-model"
	e := New(cfg, filepath.Join(dir, "config.json"), func(string) {})
	return e, dir
}

func sayJob(id int, text string) protocol.Job {
	return protocol.Job{
		Session: "sess", Chat: "saychat", ID: id, Cwd: "",
		Text: text, SayHelp: true, Instance: 1, Via: "pixel",
	}
}

// A say line must be collected, not answered. The LLM is untouched until the flush.
func TestSayJobsAreCollectedNotAnswered(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, _ := newTestEngine(t, srv.URL)

	e.dispatch(context.Background(), sayJob(1, "what is the capital of France?"))
	e.dispatch(context.Background(), sayJob(2, "and of Italy?"))

	if got := fake.calls(); got != 0 {
		t.Fatalf("say lines must not reach the LLM before the flush, got %d calls", got)
	}
	if len(e.sayBuf[1]) != 2 {
		t.Fatalf("expected 2 collected say lines, got %d", len(e.sayBuf[1]))
	}
}

// flushSay ships one batch, clears the buffer, and publishes the answer as sayText.
func TestSayFlushShipsBatchAndClears(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, dir := newTestEngine(t, srv.URL)

	e.dispatch(context.Background(), sayJob(1, "what is the capital of France?"))
	e.dispatch(context.Background(), sayJob(2, "and of Italy?"))

	// Force the "answer" roll for this test.
	e.Cfg.Modes.Say.ReplyChance = config.Int(100)
	e.flushSay(context.Background())

	deadline := time.Now().Add(3 * time.Second)
	for fake.calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fake.calls() != 1 {
		t.Fatalf("expected exactly one batched LLM call, got %d", fake.calls())
	}
	p := fake.lastPrompt()
	if !strings.Contains(p, "what is the capital of France?") || !strings.Contains(p, "and of Italy?") {
		t.Fatalf("batch must carry every collected line, got:\n%s", p)
	}
	if len(e.sayBuf[1]) != 0 {
		t.Fatalf("buffer must be cleared after the flush, still %d", len(e.sayBuf[1]))
	}

	// The answer must land in the slot file as sayText, so the addon speaks it in /s.
	slot := filepath.Join(dir, "WoWAI_I01_S001", "Inbox.lua")
	deadline = time.Now().Add(3 * time.Second)
	for {
		b, err := os.ReadFile(slot)
		if err == nil && strings.Contains(string(b), `sayText = "Paris."`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("sayText never reached the slot file (err=%v)", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// When the roll says "stay quiet", the batch is dropped: no LLM call, buffer
// cleared, and a done reply goes out so the addon stops waiting.
func TestSayFlushCanDropWithoutAnswering(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, dir := newTestEngine(t, srv.URL)

	e.Cfg.Modes.Say.ReplyChance = config.Int(0)

	e.dispatch(context.Background(), sayJob(1, "anyone got spare gold?"))
	e.flushSay(context.Background())

	time.Sleep(50 * time.Millisecond)
	if fake.calls() != 0 {
		t.Fatalf("a dropped batch must not reach the LLM, got %d calls", fake.calls())
	}
	if len(e.sayBuf[1]) != 0 {
		t.Fatalf("dropped batch must be cleared, still %d", len(e.sayBuf[1]))
	}
	slot := filepath.Join(dir, "WoWAI_I01_S001", "Inbox.lua")
	b, err := os.ReadFile(slot)
	if err != nil || !strings.Contains(string(b), `status = "done"`) {
		t.Fatalf("a dropped batch should still publish a done reply so the addon stops waiting (err=%v):\n%s", err, b)
	}
	if strings.Contains(string(b), `sayText = `) {
		t.Fatalf("a dropped batch must not carry sayText:\n%s", b)
	}
}

// A normal chat job must still be answered at once: say collection must not gate it.
func TestChatJobIsUnaffectedBySayCollection(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, _ := newTestEngine(t, srv.URL)

	e.dispatch(context.Background(), sayJob(1, "some nearby say"))
	e.dispatch(context.Background(), protocol.Job{
		Session: "sess", Chat: "chat1", ID: 2, Text: "hello ai", Instance: 1, Via: "pixel",
	})

	deadline := time.Now().Add(3 * time.Second)
	for fake.calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fake.calls() != 1 {
		t.Fatalf("the chat message should be answered right away, got %d calls", fake.calls())
	}
	if got := fake.lastPrompt(); got != "hello ai" {
		t.Fatalf("chat job prompt got mixed with say lines: %q", got)
	}
	if len(e.sayBuf[1]) != 1 {
		t.Fatalf("the say line must stay collected, buffer=%d", len(e.sayBuf[1]))
	}
}

// With say mode switched off, even a certain reply roll must not reach the LLM;
// the batch is dropped and the addon is told the batch is done.
func TestSayModeOffDropsWithoutAnswering(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, dir := newTestEngine(t, srv.URL)

	e.Cfg.Modes.Say.Enabled = config.Bool(false)
	e.Cfg.Modes.Say.ReplyChance = config.Int(100)

	e.dispatch(context.Background(), sayJob(1, "anyone about?"))
	e.flushSay(context.Background())

	time.Sleep(50 * time.Millisecond)
	if fake.calls() != 0 {
		t.Fatalf("say mode off must not reach the LLM, got %d calls", fake.calls())
	}
	slot := filepath.Join(dir, "WoWAI_I01_S001", "Inbox.lua")
	b, err := os.ReadFile(slot)
	if err != nil || !strings.Contains(string(b), `status = "done"`) {
		t.Fatalf("say mode off should publish a done reply so the addon stops waiting (err=%v):\n%s", err, b)
	}
	if strings.Contains(string(b), `sayText = `) {
		t.Fatalf("say mode off must not carry sayText:\n%s", b)
	}
}

// With whisper mode switched off, the bridge answers "skip" without calling the LLM.
func TestWhisperModeOffSkipsWithoutLLM(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, dir := newTestEngine(t, srv.URL)

	e.Cfg.Modes.Whisper.Enabled = config.Bool(false)
	e.runJob(context.Background(), whisperJob(1, "hey are you there?"))

	if fake.calls() != 0 {
		t.Fatalf("whisper mode off must not reach the LLM, got %d calls", fake.calls())
	}
	slot := filepath.Join(dir, "WoWAI_I01_S001", "Inbox.lua")
	b, err := os.ReadFile(slot)
	if err != nil {
		t.Fatalf("whisper reply was never written: %v", err)
	}
	if !strings.Contains(string(b), `whisperAction = "skip"`) {
		t.Fatalf("whisper mode off must publish a skip action:\n%s", b)
	}
	if strings.Contains(string(b), `whisperText = `) {
		t.Fatalf("a skip must not carry whisperText:\n%s", b)
	}
}

func whisperJob(id int, text string) protocol.Job {
	return protocol.Job{
		Session: "sess", Chat: "wchat", ID: id, Cwd: "",
		Text: text, WhisperHelp: true, Instance: 1, Via: "pixel",
	}
}

// The settings-UI personality dropdowns must reach the model's system prompt.
func TestWhisperStyleReachesLLM(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, _ := newTestEngine(t, srv.URL)

	e.Cfg.Modes.Whisper.Personality = "sarcastic"
	e.Cfg.Modes.Whisper.Education = "phd"
	e.runJob(context.Background(), whisperJob(1, "hey"))

	sys := fake.lastSystem()
	if !strings.Contains(sys, "dry and sarcastic") || !strings.Contains(sys, "doctoral-level expert") {
		t.Fatalf("whisper style did not reach the system prompt:\n%s", sys)
	}
	if !strings.Contains(sys, "cocky alpha-male") {
		t.Fatal("style must not replace the whisper persona")
	}
}

// With the dropdowns left on Default, the prompt must carry no style block.
func TestDefaultStyleLeavesPromptAlone(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, _ := newTestEngine(t, srv.URL)

	e.runJob(context.Background(), whisperJob(1, "hey"))

	if sys := fake.lastSystem(); strings.Contains(sys, "Reply style for this character") {
		t.Fatalf("default style should add nothing:\n%s", sys)
	}
}

// The say collector must apply its own style selection.
func TestSayStyleReachesLLM(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, _ := newTestEngine(t, srv.URL)

	e.Cfg.Modes.Say.ReplyChance = config.Int(100)
	e.Cfg.Modes.Say.Personality = "witty"
	e.dispatch(context.Background(), sayJob(1, "what is the capital of France?"))
	e.flushSay(context.Background())

	deadline := time.Now().Add(3 * time.Second)
	for fake.calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if sys := fake.lastSystem(); !strings.Contains(sys, "quick and witty") {
		t.Fatalf("say style did not reach the system prompt:\n%s", sys)
	}
}

// A batch must keep each line's speaker label, both in the prompt the model sees
// and in the history kept for the next turn.
func TestSaySpeakerLabelReachesModelAndHistory(t *testing.T) {
	fake := &fakeLLM{}
	srv := fake.server()
	defer srv.Close()
	e, _ := newTestEngine(t, srv.URL)

	e.Cfg.Modes.Say.ReplyChance = config.Int(100)
	e.dispatch(context.Background(), sayJob(1, "Bob: where is the trainer?"))
	e.dispatch(context.Background(), sayJob(2, "Jane: depends on your faction"))
	e.flushSay(context.Background())

	deadline := time.Now().Add(3 * time.Second)
	for fake.calls() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if fake.calls() != 1 {
		t.Fatalf("expected one batched call, got %d", fake.calls())
	}
	p := fake.lastPrompt()
	if !strings.Contains(p, "1: Bob: where is the trainer?") || !strings.Contains(p, "2: Jane: depends on your faction") {
		t.Fatalf("the numbered batch must carry each speaker:\n%s", p)
	}

	// The same labelled lines are the history the next turn builds on.
	deadline = time.Now().Add(2 * time.Second)
	for {
		e.mu.Lock()
		hist := append([]llm.Message(nil), e.history["say:1"]...)
		e.mu.Unlock()
		if len(hist) > 0 {
			if hist[0].Role != "user" || !strings.Contains(hist[0].Content, "Bob: where is the trainer?") {
				t.Fatalf("history must keep the speakers, got %+v", hist)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("say history was never recorded")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
