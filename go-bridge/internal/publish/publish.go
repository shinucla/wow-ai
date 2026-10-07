package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

// Publisher writes instance-scoped Inbox.lua and WoWAI_I##S###/Inbox.lua atomically.
type Publisher struct {
	AddonDir  string
	Instance  int // 1-based
	Slots     int
	Cwd       string
	Agent     string
	Agents    []string

	mu         sync.Mutex
	live       map[string]protocol.Reply
	lastWrite  time.Time
	throttleMs int
	timer      *time.Timer
}

func New(addonDir string, instance, slots int, cwd, agent string, agents []string, throttleMs int) *Publisher {
	if throttleMs <= 0 {
		throttleMs = 3000
	}
	if slots <= 0 {
		slots = 200
	}
	if instance <= 0 {
		instance = 1
	}
	return &Publisher{
		AddonDir:   addonDir,
		Instance:   instance,
		Slots:      slots,
		Cwd:        cwd,
		Agent:      agent,
		Agents:     agents,
		live:       map[string]protocol.Reply{},
		throttleMs: throttleMs,
	}
}

func (p *Publisher) slotPrefix() string {
	return fmt.Sprintf("WoWAI_I%02d_S", p.Instance)
}

func (p *Publisher) inboxPath() string {
	return filepath.Join(p.AddonDir, "WoWAI", fmt.Sprintf("i%02d", p.Instance), "Inbox.lua")
}

// Refresh writes current live replies (or an empty table) with a fresh bridge
// clock so the addon can NoteBridge from a slot load — same as Node publishNow
// on hello, which is required when PlaySoundFile signals are unavailable.
func (p *Publisher) Refresh() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
	return p.writeNow()
}

func (p *Publisher) Publish(key string, r protocol.Reply, urgent bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	r.Instance = p.Instance
	p.live[key] = r
	if urgent {
		if p.timer != nil {
			p.timer.Stop()
			p.timer = nil
		}
		_ = p.writeNow()
		return
	}
	wait := time.Duration(p.throttleMs)*time.Millisecond - time.Since(p.lastWrite)
	if wait <= 0 {
		_ = p.writeNow()
		return
	}
	if p.timer == nil {
		p.timer = time.AfterFunc(wait, func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.timer = nil
			_ = p.writeNow()
		})
	}
}

func (p *Publisher) writeNow() error {
	if p.AddonDir == "" {
		return fmt.Errorf("addonDir is empty")
	}
	p.lastWrite = time.Now()
	records := make([]protocol.Reply, 0, len(p.live))
	for _, r := range p.live {
		records = append(records, r)
	}
	if len(records) > 30 {
		records = records[len(records)-30:]
	}
	opts := protocol.LuaOpts{
		Now:    time.Now(),
		Cwd:    p.Cwd,
		Agent:  p.Agent,
		Agents: p.Agents,
	}
	inbox := protocol.LuaTable("WoWAI_Inbox", records, opts)
	if err := atomicWrite(p.inboxPath(), []byte(inbox)); err != nil {
		return err
	}
	body := protocol.LuaTable("WoWAI_SlotData", records, opts)
	prefix := p.slotPrefix()
	var firstErr error
	wrote := 0
	for i := 1; i <= p.Slots; i++ {
		path := filepath.Join(p.AddonDir, prefix+protocol.Pad3(i), "Inbox.lua")
		if err := atomicWrite(path, []byte(body)); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		wrote++
	}
	if wrote == 0 && firstErr != nil {
		return firstErr
	}
	return nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// InstallSlots creates per-instance slot addons and signal files.
func InstallSlots(addonDir, tocInterface string, slots, actMax, presenceMax, maxInstances int) (made, kept int, err error) {
	if tocInterface == "" {
		tocInterface = "30300"
	}
	if slots <= 0 {
		slots = 200
	}
	if actMax <= 0 {
		actMax = 60
	}
	if presenceMax <= 0 {
		presenceMax = 2000
	}
	if maxInstances <= 0 {
		maxInstances = 4
	}
	toc := filepath.Join(addonDir, "WoWAI", "WoWAI.toc")
	if _, err := os.Stat(toc); err != nil {
		return 0, 0, err
	}
	ensure := func(path string, content []byte) {
		if _, e := os.Stat(path); e == nil {
			kept++
			return
		}
		_ = os.MkdirAll(filepath.Dir(path), 0o755)
		if e := os.WriteFile(path, content, 0o644); e == nil {
			made++
		}
	}
	ensure(filepath.Join(addonDir, "WoWAI", "ctl", "empty.wav"), nil)
	ensure(filepath.Join(addonDir, "WoWAI", "ctl", "valid.wav"), protocol.SilentWAV)

	for inst := 1; inst <= maxInstances; inst++ {
		prefix := fmt.Sprintf("WoWAI_I%02d_S", inst)
		iroot := filepath.Join(addonDir, "WoWAI", fmt.Sprintf("i%02d", inst))
		ensure(filepath.Join(iroot, "Inbox.lua"), []byte("WoWAI_Inbox = nil\n"))
		for i := 1; i <= slots; i++ {
			name := prefix + protocol.Pad3(i)
			dir := filepath.Join(addonDir, name)
			tocBody := "## Interface: " + tocInterface + "\n" +
				fmt.Sprintf("## Title: WoW AI i%02d slot %s\n", inst, protocol.Pad3(i)) +
				"## Notes: Instance-scoped reply slot for WoW AI. Load-on-demand; leave it enabled.\n" +
				"## LoadOnDemand: 1\n" +
				"## Dependencies: WoWAI\n\nInbox.lua\n"
			ensure(filepath.Join(dir, name+".toc"), []byte(tocBody))
			ensure(filepath.Join(dir, "Inbox.lua"), []byte("WoWAI_SlotData = nil\n"))
			ensure(filepath.Join(iroot, "sig", protocol.Pad3(i)+".wav"), nil)
			ensure(filepath.Join(iroot, "ack", protocol.Pad3(i)+".wav"), nil)
			for k := 1; k <= actMax; k++ {
				ensure(filepath.Join(iroot, "act", protocol.Pad3(i), protocol.Pad2(k)+".wav"), nil)
			}
		}
		for k := 1; k <= presenceMax; k++ {
			ensure(filepath.Join(iroot, "presence", protocol.Pad4(k)+".wav"), nil)
		}
	}
	return made, kept, nil
}
