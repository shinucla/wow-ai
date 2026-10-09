package bridge

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/autochat"
	"github.com/chelinho139/wow-ai/go-bridge/internal/capture"
	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
	"github.com/chelinho139/wow-ai/go-bridge/internal/llm"
	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
	"github.com/chelinho139/wow-ai/go-bridge/internal/publish"
	"github.com/chelinho139/wow-ai/go-bridge/internal/signals"
)

// LogFunc receives human-readable status lines for the UI.
type LogFunc func(line string)

type lane struct {
	pub *publish.Publisher
	sig *signals.Writer
}

// Engine is the core bridge loop: multi-window capture → LLM → per-instance slots.
type Engine struct {
	Cfg      config.Config
	CfgPath  string
	Log      LogFunc // always shown (game traffic, errors, warnings)
	LogDebug LogFunc // only when Settings → Debug is on
	LLM      *llm.Client
	Primer   string

	mu      sync.Mutex
	lanes   map[int]*lane
	handled map[string]map[int]bool // session → ids
	history map[string][]llm.Message
	context map[int]string // per-instance game context
	sayBuf  map[int][]string  // per-instance /say lines waiting for the flush
	sayChat map[int]string    // per-instance addon chat id the say lines came from
	sayID   map[int]int       // per-instance id of the newest collected say record
	running bool
	cancel  context.CancelFunc
	sem     chan struct{}
	injectN int
}

// sayBufMax caps collected lines per instance so a chatty zone can't grow it forever.
const sayBufMax = 60

func New(cfg config.Config, cfgPath string, log LogFunc) *Engine {
	if log == nil {
		log = func(string) {}
	}
	primer := ""
	if cfg.PrimerFile != "" {
		if b, err := os.ReadFile(cfg.PrimerFile); err == nil {
			primer = string(b)
		}
	}
	e := &Engine{
		Cfg:     cfg,
		CfgPath: cfgPath,
		Log:     log,
		LLM:     llm.New(cfg.LLM, primer),
		Primer:  primer,
		lanes:   map[int]*lane{},
		handled: map[string]map[int]bool{},
		history: map[string][]llm.Message{},
		context: map[int]string{},
		sayBuf:  map[int][]string{},
		sayChat: map[int]string{},
		sayID:   map[int]int{},
		sem:     make(chan struct{}, max(1, cfg.MaxParallel)),
	}
	e.ensureLanes()
	return e
}

func (e *Engine) debug(line string) {
	if e.LogDebug != nil {
		e.LogDebug(line)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (e *Engine) ensureLanes() {
	n := e.Cfg.MaxInstances
	if n <= 0 {
		n = 4
	}
	agent := e.Cfg.LLM.AgentName
	if agent == "" {
		agent = "api"
	}
	for i := 1; i <= n; i++ {
		if e.lanes[i] != nil {
			continue
		}
		e.lanes[i] = &lane{
			pub: publish.New(e.Cfg.AddonDir, i, e.Cfg.Slots, e.Cfg.DefaultCwd, agent, e.Cfg.AgentIDs(), e.Cfg.ProgressWriteMs),
			sig: &signals.Writer{
				AddonDir:    e.Cfg.AddonDir,
				Instance:    i,
				Slots:       e.Cfg.Slots,
				ActMax:      e.Cfg.ActMax,
				PresenceMax: e.Cfg.PresenceMax,
			},
		}
	}
}

func (e *Engine) lane(inst int) *lane {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inst <= 0 {
		inst = 1
	}
	maxN := e.Cfg.MaxInstances
	if maxN <= 0 {
		maxN = 4
	}
	if inst > maxN {
		inst = maxN
	}
	e.ensureLanes()
	return e.lanes[inst]
}

func (e *Engine) ApplyConfig(cfg config.Config) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Cfg = cfg
	e.LLM.Update(cfg.LLM)
	e.lanes = map[int]*lane{}
	e.ensureLanes()
	e.sem = make(chan struct{}, max(1, cfg.MaxParallel))
}

func (e *Engine) Start() error {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return fmt.Errorf("already running")
	}
	ctx, cancel := context.WithCancel(context.Background())
	e.cancel = cancel
	e.running = true
	e.mu.Unlock()

	e.debug("bridge starting (multi-instance)")
	if e.Cfg.AddonDir == "" {
		e.Log("warning: addonDir is empty — set it in Settings")
	} else if _, err := os.Stat(filepath.Join(e.Cfg.AddonDir, "WoWAI", "WoWAI.toc")); err != nil {
		e.Log("warning: WoWAI addon not found under addonDir")
	}
	e.debug(fmt.Sprintf("max instances: %d — each client needs /wow-ai instance N matching window order (left→right)", max(1, e.Cfg.MaxInstances)))

	// Seed slot clocks so a Connect/hello poll can see the bridge immediately.
	e.ensureLanes()
	for i, l := range e.lanes {
		if l == nil {
			continue
		}
		if err := l.pub.Refresh(); err != nil {
			e.Log(fmt.Sprintf("startup slot refresh i%d: %v", i, err))
		}
	}

	capCh := make(chan capture.Event, 32)
	if e.Cfg.Capture.Enabled {
		proc := e.Cfg.Capture.ProcessName
		if proc == "" {
			proc = "Wow"
		}
		e.debug(fmt.Sprintf("capture on — watching process %q every %dms (Start required; green light alone is not enough)",
			proc, max(50, e.Cfg.Capture.IntervalMs)))
		go capture.NewRunner().Run(ctx, e.Cfg.Capture, e.Cfg.MaxInstances, capCh)
	} else {
		e.Log("capture disabled — in-game strip will never be read")
	}

	go e.presenceLoop(ctx)
	go e.outboxPoll(ctx)
	go e.sayLoop(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case ev := <-capCh:
				e.handleCapture(ctx, ev)
			}
		}
	}()
	return nil
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cancel != nil {
		e.cancel()
		e.cancel = nil
	}
	e.running = false
	e.debug("bridge stopped")
}

func (e *Engine) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *Engine) presenceLoop(ctx context.Context) {
	iv := time.Duration(e.Cfg.PresenceIntervalMs) * time.Millisecond
	if iv < time.Second {
		iv = 30 * time.Second
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	beat := func() {
		e.mu.Lock()
		lanes := make([]*lane, 0, len(e.lanes))
		for _, l := range e.lanes {
			lanes = append(lanes, l)
		}
		e.mu.Unlock()
		for _, l := range lanes {
			l.sig.PresenceBeat()
		}
	}
	beat()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			beat()
		}
	}
}

func (e *Engine) outboxPoll(ctx context.Context) {
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	var lastID int
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			path := e.Cfg.SavedVariablesFile
			if path == "" {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			job := protocol.ParseOutbox(string(data))
			if job == nil || job.ID == 0 || job.ID == lastID {
				continue
			}
			lastID = job.ID
			if job.Instance <= 0 {
				job.Instance = 1
			}
			e.debug(fmt.Sprintf("outbox message #%d via reload (instance %d)", job.ID, job.Instance))
			e.dispatch(ctx, *job)
		}
	}
}

func (e *Engine) handleCapture(ctx context.Context, ev capture.Event) {
	if ev.Info != "" {
		e.debug("capture: " + ev.Info)
		return
	}
	if ev.Warn != "" {
		e.debug("capture: " + ev.Warn)
		return
	}
	if ev.Error != "" {
		e.Log("capture error: " + ev.Error)
		return
	}
	if ev.Payload == nil {
		return
	}

	payload := string(ev.Payload)
	jobs := protocol.JobsFromStrip(int(ev.ID), payload)
	if len(jobs) == 0 {
		e.debug(fmt.Sprintf("capture: strip #%d decoded (%d bytes) but no jobs parsed", ev.ID, len(ev.Payload)))
		return
	}
	for _, j := range jobs {
		if j.Instance <= 0 {
			j.Instance = ev.Instance
		}
		if j.Instance <= 0 {
			j.Instance = 1
		}
		e.dispatch(ctx, j)
	}
}

func truncateDebug(s string, n int) string {
	s = strings.ReplaceAll(s, "\x1e", "\\x1e")
	s = strings.ReplaceAll(s, "\x1f", "\\x1f")
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// Inject simulates a strip message for one instance (testing).
// Each call gets a fresh message id so repeats are not silently dropped.
func (e *Engine) Inject(text string, instance int) (int, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("text required")
	}
	if instance <= 0 {
		instance = 1
	}
	e.mu.Lock()
	e.injectN++
	id := e.injectN
	e.mu.Unlock()

	payload := fmt.Sprintf("inject\x1ftest\x1f%d\x1f\x1fi=%d\x1fTest\x1f%s", id, instance, text)
	jobs := protocol.JobsFromStrip(id, payload)
	if len(jobs) == 0 {
		e.debug("inject: payload did not parse into a job")
		return 0, fmt.Errorf("payload did not parse")
	}
	e.debug(fmt.Sprintf("inject #%d i%d", id, instance))
	ctx := context.Background()
	for _, j := range jobs {
		if j.Instance <= 0 {
			j.Instance = instance
		}
		e.dispatch(ctx, j)
	}
	return id, nil
}

func (e *Engine) already(j protocol.Job) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := fmt.Sprintf("%d:%s", j.Instance, j.Session)
	h := e.handled[key]
	if h == nil {
		return false
	}
	return h[j.ID]
}

func (e *Engine) mark(j protocol.Job) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := fmt.Sprintf("%d:%s", j.Instance, j.Session)
	h := e.handled[key]
	if h == nil {
		h = map[int]bool{}
		e.handled[key] = h
	}
	h[j.ID] = true
}

func (e *Engine) dispatch(ctx context.Context, j protocol.Job) {
	if j.Instance <= 0 {
		j.Instance = 1
	}
	ln := e.lane(j.Instance)
	if e.already(j) {
		// Still ACK so the addon clears the strip / stops "Sending try N/3".
		_ = ln.sig.Ack(j.ID)
		return
	}
	e.mark(j)
	if err := ln.sig.Ack(j.ID); err != nil {
		e.Log(fmt.Sprintf("ack #%d failed: %v (addonDir=%s)", j.ID, err, e.Cfg.AddonDir))
	}

	if j.Context {
		e.mu.Lock()
		e.context[j.Instance] = j.Ctx
		e.mu.Unlock()
	}
	if j.Hello {
		// Match Node: ack + refresh slots with a fresh bridge clock so the addon's
		// hello poll (LoadAddOn slot) can NoteBridge even when WAV signals fail.
		if err := ln.pub.Refresh(); err != nil {
			e.Log(fmt.Sprintf("hello i%d: slot refresh failed: %v", j.Instance, err))
		}
		ln.sig.PresenceBeat()
		return
	}
	if j.Forget {
		key := protocol.ChatKey(j)
		e.mu.Lock()
		delete(e.history, key)
		e.mu.Unlock()
		return
	}
	if j.NewSession {
		key := protocol.ChatKey(j)
		e.mu.Lock()
		e.history[key] = nil
		e.mu.Unlock()
	}

	// Say mode: don't answer each line. Collect it and let sayLoop hand the batch
	// to the LLM once the configured window elapses. Ack already happened above,
	// so the strip clears.
	if j.CoachFor() == protocol.CoachSay {
		e.mu.Lock()
		if len(e.sayBuf[j.Instance]) < sayBufMax {
			e.sayBuf[j.Instance] = append(e.sayBuf[j.Instance], j.Text)
		}
		e.sayChat[j.Instance] = j.Chat
		e.sayID[j.Instance] = j.ID
		e.mu.Unlock()
		e.logGame("say", "from", j.Text)
		return
	}

	e.logGame(jobChannel(j), "from", j.Text)
	go e.runJob(ctx, j)
}

// configSnapshot returns a copy of the current config under the lock, for
// goroutines that need a setting without holding e.mu for long.
func (e *Engine) configSnapshot() config.Config {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Cfg
}

// sayLoop flushes collected /say lines to the LLM on the configured cadence.
// The interval is re-read after every flush, so a settings change takes effect
// from the next round without a restart.
func (e *Engine) sayLoop(ctx context.Context) {
	for {
		t := time.NewTimer(time.Duration(e.configSnapshot().SayCollectSeconds()) * time.Second)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			e.flushSay(ctx)
		}
	}
}

// flushSay takes whatever /say lines have collected, one batch per instance. The
// buffer is cleared first so new lines collect while a reply is being written.
// Each batch is only answered SayReplyChance percent of the time; otherwise it is
// dropped, so the character speaks up now and then instead of constantly.
func (e *Engine) flushSay(ctx context.Context) {
	e.mu.Lock()
	if len(e.sayBuf) == 0 {
		e.mu.Unlock()
		return
	}
	batches := make(map[int][]string, len(e.sayBuf))
	for inst, lines := range e.sayBuf {
		if len(lines) > 0 {
			batches[inst] = lines
		}
	}
	e.sayBuf = map[int][]string{}
	e.mu.Unlock()

	cfg := e.configSnapshot()
	for inst, lines := range batches {
		if !cfg.ModeEnabled("say") {
			e.debug(fmt.Sprintf("say i%d: %d line(s) collected, say mode is off — cleared", inst, len(lines)))
			e.dropSay(inst)
			continue
		}
		if rand.Intn(100) >= cfg.SayReplyChancePct() {
			e.debug(fmt.Sprintf("say i%d: %d line(s) collected, rolled not-to-reply — cleared", inst, len(lines)))
			e.dropSay(inst)
			continue
		}
		go e.runSay(ctx, inst, lines)
	}
}

// dropSay clears the addon's wait for this batch without speaking anything, so it
// stops polling for an answer that will never come.
func (e *Engine) dropSay(inst int) {
	e.mu.Lock()
	replyID := e.sayID[inst]
	chatID := e.sayChat[inst]
	e.mu.Unlock()
	if replyID == 0 {
		return
	}
	ln := e.lane(inst)
	ln.pub.Publish(fmt.Sprintf("say:%d", inst), protocol.Reply{
		Chat: chatID, ID: replyID, Status: "done", Text: "",
		Cwd: e.Cfg.DefaultCwd, Agent: e.LLM.AgentName(), Instance: inst,
	}, true)
	_ = ln.sig.Sig(replyID)
}

// runSay asks the scholar persona about one batch of surrounding /say lines and
// publishes the answer for the addon to speak in /s.
func (e *Engine) runSay(ctx context.Context, inst int, lines []string) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return
	}

	ln := e.lane(inst)
	var b strings.Builder
	for i, l := range lines {
		fmt.Fprintf(&b, "%d: %s\n", i+1, l)
	}
	prompt := strings.TrimRight(b.String(), "\n")

	e.mu.Lock()
	gameCtx := e.context[inst]
	key := fmt.Sprintf("say:%d", inst)
	replyID := e.sayID[inst]
	chatID := e.sayChat[inst]
	hist := append([]llm.Message(nil), e.history[key]...)
	e.mu.Unlock()

	// Random word budget each turn (min..max from settings) so replies vary
	// instead of all matching.
	sayCfg := e.configSnapshot()
	lo, hi := sayCfg.SayWordRange()
	maxWords := lo + rand.Intn(hi-lo+1)
	res, err := autochat.DecideSay(ctx, e.LLM, prompt, gameCtx, hist, maxWords, styleFor(sayCfg, "say"))
	if err != nil {
		e.Log(fmt.Sprintf("say error i%d: %v", inst, err))
		return
	}
	// Show what actually came back, so a "why did it skip?" is answerable from the log.
	e.debug(fmt.Sprintf("say i%d: %d line(s), maxWords=%d, style=%s, raw reply: %s",
		inst, len(lines), maxWords, describeStyle(sayCfg, "say"), truncateDebug(res.Raw, 300)))
	if res.Decision.Action != "reply" || strings.TrimSpace(res.Decision.Text) == "" {
		e.logGame("say", "to", "(skip)")
		return
	}
	text := res.Decision.Text
	e.logGame("say", "to", text)

	e.mu.Lock()
	e.history[key] = append(e.history[key],
		llm.Message{Role: "user", Content: prompt},
		llm.Message{Role: "assistant", Content: text},
	)
	if len(e.history[key]) > 40 {
		e.history[key] = e.history[key][len(e.history[key])-40:]
	}
	e.mu.Unlock()

	// Reply id = the newest collected say record, which the addon already knows,
	// so its signal check pulls this answer immediately. Fall back to the clock.
	if replyID == 0 {
		replyID = int(time.Now().Unix())
	}
	ln.pub.Publish(key, protocol.Reply{
		Chat: chatID, ID: replyID, Status: "done", Text: text,
		Cwd: e.Cfg.DefaultCwd, Agent: e.LLM.AgentName(), Instance: inst,
		SayText: text,
	}, true)
	ln.sig.Sig(replyID)
}

func (e *Engine) runJob(ctx context.Context, j protocol.Job) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return
	}

	ln := e.lane(j.Instance)
	key := protocol.ChatKey(j)
	agent := e.LLM.AgentName()
	cwd := protocol.ResolveCwd(j.Cwd, e.Cfg.DefaultCwd)

	ln.pub.Publish(key, protocol.Reply{
		Chat: j.Chat, ID: j.ID, Status: "working", Text: "…", Cwd: cwd, Agent: agent, Instance: j.Instance,
	}, true)
	ln.sig.ResetBeats(j.ID)
	ln.sig.Beat(j.ID, 1)

	e.mu.Lock()
	// Prefer this message's context snapshot; else last known for the instance
	// (addon only re-sends context when it changes).
	gameCtx := strings.TrimSpace(j.Ctx)
	if gameCtx == "" {
		gameCtx = e.context[j.Instance]
	}
	coach := j.CoachFor()
	hist := append([]llm.Message(nil), e.history[key]...)
	e.mu.Unlock()

	// Bridge-side master switch for the persona modes. With the mode off we do
	// not call the LLM; we answer "skip" so the addon stops waiting on this id.
	if (coach == protocol.CoachWhisper || coach == protocol.CoachParty) && !e.configSnapshot().ModeEnabled(jobChannel(j)) {
		channel := jobChannel(j)
		e.debug(fmt.Sprintf("#%d [%s] auto-reply is off in bridge settings — skipped", j.ID, channel))
		ln.pub.Publish(key, protocol.Reply{
			Chat: j.Chat, ID: j.ID, Status: "done", Text: "(auto-reply off)",
			Cwd: cwd, Agent: agent, Instance: j.Instance, WhisperAction: "skip",
		}, true)
		ln.sig.Sig(j.ID)
		return
	}

	var text string
	var whisperAction, whisperText string
	var err error
	switch coach {
	case protocol.CoachSay:
		// Say mode is owned by the collector (dispatch + sayLoop). A single say line
		// must never be answered here; reaching this point means a bug upstream.
		e.debug(fmt.Sprintf("#%d [say] single say line reached runJob — ignored (collector owns say mode)", j.ID))
		return
	case protocol.CoachWhisper, protocol.CoachParty:
		// Auto-whisper / auto-party — persona JSON coaches.
		mode := jobChannel(j)
		modeCfg := e.configSnapshot()
		e.debug(fmt.Sprintf("#%d [%s] persona coach (not assist), style=%s", j.ID, mode, describeStyle(modeCfg, mode)))
		var res autochat.Result
		res, err = autochat.Decide(ctx, e.LLM, coach, j.Text, gameCtx, hist, styleFor(modeCfg, mode))
		if err == nil {
			text = res.Text
			whisperAction = res.Decision.Action
			whisperText = res.Decision.Text
			if res.Decision.Action == "reply" {
				e.logGame(mode, "to", res.Decision.Text)
			} else {
				e.logGame(mode, "to", "(skip)")
			}
		}
	default:
		// In-addon assist (original wow-ai): game context + assist prompt → full reply to addon.
		mode := "assist"
		if j.WhisperHelp || j.PartyHelp {
			// Defensive: CoachFor should have caught these.
			mode = "assist?"
		}
		if gameCtx == "" {
			e.debug(fmt.Sprintf("#%d [%s] no game context yet — answer may lack location/character facts", j.ID, mode))
		} else {
			e.debug(fmt.Sprintf("#%d [%s] context %d bytes", j.ID, mode, len(gameCtx)))
		}
		sys := protocol.SystemPrompt(gameCtx, e.Primer, protocol.CoachNone, protocol.Style{})
		text, err = e.LLM.Chat(ctx, sys, j.Text, hist)
	}
	if err != nil {
		e.Log(fmt.Sprintf("error #%d: %v", j.ID, err))
		ln.pub.Publish(key, protocol.Reply{
			Chat: j.Chat, ID: j.ID, Status: "error", Text: err.Error(), Cwd: cwd, Agent: agent, Instance: j.Instance,
		}, true)
		ln.sig.Sig(j.ID)
		return
	}

	text, macros, _ := protocol.ExtractMacros(text)
	full, summary := protocol.SplitSummary(text)
	if coach != protocol.CoachNone {
		full = text
		summary = text
		macros = nil
	} else {
		out := full
		if summary != "" {
			out = summary
		}
		e.logGame("chat", "to", out)
	}

	e.mu.Lock()
	e.history[key] = append(e.history[key],
		llm.Message{Role: "user", Content: j.Text},
		llm.Message{Role: "assistant", Content: full},
	)
	if len(e.history[key]) > 40 {
		e.history[key] = e.history[key][len(e.history[key])-40:]
	}
	e.mu.Unlock()

	ln.pub.Publish(key, protocol.Reply{
		Chat: j.Chat, ID: j.ID, Status: "done", Text: full, Summary: summary,
		Cwd: cwd, Agent: agent, Macros: macros, Instance: j.Instance,
		WhisperAction: whisperAction, WhisperText: whisperText,
	}, true)
	ln.sig.Sig(j.ID)
}

func jobChannel(j protocol.Job) string {
	switch j.CoachFor() {
	case protocol.CoachWhisper:
		return "whisper"
	case protocol.CoachParty:
		return "party"
	case protocol.CoachSay:
		return "say"
	default:
		return "chat"
	}
}

// styleFor turns the settings-UI dropdowns for a mode into a protocol.Style.
func styleFor(cfg config.Config, mode string) protocol.Style {
	personality, education, characteristics := cfg.ModeStyle(mode)
	return protocol.Style{
		Personality:     personality,
		Education:       education,
		Characteristics: characteristics,
	}
}

// describeStyle is a compact "personality/education/characteristics" string for
// the debug log, so "why did it talk like that?" is answerable from the log.
func describeStyle(cfg config.Config, mode string) string {
	s := styleFor(cfg, mode)
	if s.IsZero() {
		return "default"
	}
	part := func(label, key string) string {
		if key == "" {
			return label + "=default"
		}
		return label + "=" + key
	}
	return part("p", s.Personality) + " " + part("e", s.Education) + " " + part("c", s.Characteristics)
}

// logGame writes one UI line: [HH:MM:SS] [chat|whisper|party] [from|to] game: <raw>
func (e *Engine) logGame(channel, dir, text string) {
	text = strings.Join(strings.Fields(strings.ReplaceAll(strings.ReplaceAll(text, "\r", " "), "\n", " ")), " ")
	e.Log(fmt.Sprintf("[%s] [%s] [%s] game: %s", time.Now().Format("15:04:05"), channel, dir, text))
}

func short(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:8]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
