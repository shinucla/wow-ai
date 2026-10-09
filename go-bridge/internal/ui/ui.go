package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/bridge"
	"github.com/chelinho139/wow-ai/go-bridge/internal/capture"
	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
	"github.com/chelinho139/wow-ai/go-bridge/internal/publish"
)

// App hosts a local Status + Settings UI in the browser.
type App struct {
	Eng     *bridge.Engine
	Cfg     *config.Config
	CfgPath string

	mu   sync.Mutex
	logs []string
}

func New(eng *bridge.Engine, cfg *config.Config, cfgPath string) *App {
	a := &App{Eng: eng, Cfg: cfg, CfgPath: cfgPath}
	eng.Log = a.appendLog
	eng.LogDebug = a.appendDebug
	return a
}

func (a *App) appendLog(line string) {
	a.writeLog(line, false)
}

func (a *App) appendDebug(line string) {
	a.writeLog(line, true)
}

func (a *App) writeLog(line string, debug bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if debug && (a.Cfg == nil || !a.Cfg.Debug) {
		return
	}
	line = oneLine(line)
	if line == "" {
		return
	}
	// Game traffic lines already include [HH:MM:SS]; system lines get a stamp here.
	if !hasTimeStamp(line) {
		line = time.Now().Format("[15:04:05]") + " " + line
	}
	a.logs = append(a.logs, line)
	if len(a.logs) > 500 {
		a.logs = a.logs[len(a.logs)-500:]
	}
}

func hasTimeStamp(line string) bool {
	return len(line) >= 10 && line[0] == '[' && line[3] == ':' && line[6] == ':' && line[9] == ']'
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.Join(strings.Fields(s), " ")
}

func (a *App) snapshotLogs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.logs))
	copy(out, a.logs)
	return out
}

// Run listens on an ephemeral localhost port, opens the browser, and blocks.
func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	addr := "http://" + ln.Addr().String()
	a.appendDebug("UI at " + addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/api/status", a.handleStatus)
	mux.HandleFunc("/api/config", a.handleConfig)
	mux.HandleFunc("/api/start", a.handleStart)
	mux.HandleFunc("/api/stop", a.handleStop)
	mux.HandleFunc("/api/install-slots", a.handleInstallSlots)
	mux.HandleFunc("/api/inject", a.handleInject)
	mux.HandleFunc("/api/probe", a.handleProbe)
	mux.HandleFunc("/api/style-options", a.handleStyleOptions)

	srv := &http.Server{Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()

	go openBrowser(addr)
	a.appendDebug("ready — open Settings, set addonDir + API key, then Start")
	return srv.Serve(ln)
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func (a *App) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = pageTmpl.Execute(w, nil)
}

func (a *App) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"running": a.Eng.Running(),
		"logs":    a.snapshotLogs(),
		"cfgPath": a.CfgPath,
	})
}

func (a *App) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, a.Cfg)
	case http.MethodPost:
		var cfg config.Config
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		cfg.ApplyDefaults()
		*a.Cfg = cfg
		if err := cfg.Save(a.CfgPath); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		a.Eng.ApplyConfig(cfg)
		a.appendLog("settings saved to " + a.CfgPath)
		writeJSON(w, map[string]string{"ok": "saved"})
	default:
		http.Error(w, "method", 405)
	}
}

func (a *App) handleStart(w http.ResponseWriter, r *http.Request) {
	if err := a.Eng.Start(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]string{"ok": "started"})
}

func (a *App) handleStop(w http.ResponseWriter, r *http.Request) {
	a.Eng.Stop()
	writeJSON(w, map[string]string{"ok": "stopped"})
}

func (a *App) handleInstallSlots(w http.ResponseWriter, r *http.Request) {
	made, kept, err := publish.InstallSlots(a.Cfg.AddonDir, a.Cfg.TocInterface, a.Cfg.Slots, a.Cfg.ActMax, a.Cfg.PresenceMax, a.Cfg.MaxInstances)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	a.appendDebug(fmt.Sprintf("slots: created %d, kept %d (instances 1..%d) — fully quit and relaunch WoW", made, kept, max(1, a.Cfg.MaxInstances)))
	writeJSON(w, map[string]interface{}{"made": made, "kept": kept})
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (a *App) handleInject(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Text     string `json:"text"`
		Chat     string `json:"chat"`
		Instance int    `json:"instance"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if body.Text == "" {
		http.Error(w, "text required", 400)
		return
	}
	inst := body.Instance
	if inst <= 0 {
		inst = 1
	}
	// Capture/presence need Start; inject can still run LLM + write slots without it,
	// but auto-start so replies and presence work the same as a game strip.
	if !a.Eng.Running() {
		if err := a.Eng.Start(); err != nil {
			http.Error(w, "start failed: "+err.Error(), 400)
			return
		}
		a.appendDebug("auto-started for inject")
	}
	id, err := a.Eng.Inject(body.Text, inst)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": "injected", "id": id, "instance": inst})
}

// handleStyleOptions serves the personality/education/characteristics catalogs
// straight from the protocol package, so the UI dropdowns and the prompt builder
// can never disagree about what a key means.
func (a *App) handleStyleOptions(w http.ResponseWriter, r *http.Request) {
	toJSON := func(list []protocol.StyleOption) []map[string]string {
		out := make([]map[string]string, 0, len(list))
		for _, o := range list {
			out = append(out, map[string]string{"key": o.Key, "label": o.Label})
		}
		return out
	}
	writeJSON(w, map[string]interface{}{
		"personalities":   toJSON(protocol.Personalities),
		"education":       toJSON(protocol.EducationLevels),
		"characteristics": toJSON(protocol.Characteristics),
	})
}

func (a *App) handleProbe(w http.ResponseWriter, r *http.Request) {
	dir := filepath.Join(filepath.Dir(a.CfgPath), "probes")
	if a.CfgPath == "" {
		dir = filepath.Join(os.TempDir(), "wow-ai-bridge-probes")
	}
	res := capture.Probe(a.Cfg.Capture, a.Cfg.MaxInstances, dir)
	if res.Error != "" {
		a.appendLog("probe: " + res.Error)
	} else if len(res.Windows) == 0 {
		a.appendDebug("probe: no windows")
	} else {
		for _, wi := range res.Windows {
			a.appendDebug(fmt.Sprintf(
				"probe i%d hwnd=%s window=(%d,%d) clientOrigin=(%d,%d) decode=%s img=%s",
				wi.Instance, wi.HWND, wi.WindowLeft, wi.WindowTop, wi.ClientScreenX, wi.ClientScreenY, wi.Decode, wi.ImagePath,
			))
			if wi.PayloadPreview != "" {
				a.appendDebug("probe payload: " + wi.PayloadPreview)
			}
		}
	}
	writeJSON(w, res)
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
