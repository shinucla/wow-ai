package signals

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/chelinho139/wow-ai/go-bridge/internal/protocol"
)

// Writer flips empty/valid WAV signal files the addon polls with PlaySoundFile.
// Paths are instance-scoped: WoWAI/i01/ack/NNN.wav so two clients sharing one
// Install folder cannot steal each other's acks/sigs.
type Writer struct {
	AddonDir    string
	Instance    int // 1-based
	Slots       int
	ActMax      int
	PresenceMax int
	Presence    int
}

func (w *Writer) root() string {
	n := w.Instance
	if n <= 0 {
		n = 1
	}
	return filepath.Join(w.AddonDir, "WoWAI", fmt.Sprintf("i%02d", n))
}

func (w *Writer) path(kind string, n int, pad int) string {
	name := protocol.Pad3(n)
	if pad == 4 {
		name = protocol.Pad4(n)
	}
	return filepath.Join(w.root(), kind, name+".wav")
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

func (w *Writer) Signal(kind string, id int, on bool) error {
	n := protocol.SlotNumber(id, w.Slots)
	var data []byte
	if on {
		data = protocol.SilentWAV
	}
	return atomicWrite(w.path(kind, n, 3), data)
}

// AckPath is the on-disk ack file for logging/diagnostics.
func (w *Writer) AckPath(id int) string {
	return w.path("ack", protocol.SlotNumber(id, w.Slots), 3)
}

func (w *Writer) Ack(id int) error      { return w.Signal("ack", id, true) }
func (w *Writer) Sig(id int) error      { return w.Signal("sig", id, true) }
func (w *Writer) ClearAck(id int) error { return w.Signal("ack", id, false) }
func (w *Writer) ClearSig(id int) error { return w.Signal("sig", id, false) }

func (w *Writer) ResetBeats(id int) {
	n := protocol.SlotNumber(id, w.Slots)
	dir := filepath.Join(w.root(), "act", protocol.Pad3(n))
	for k := 1; k <= w.ActMax; k++ {
		_ = atomicWrite(filepath.Join(dir, protocol.Pad2(k)+".wav"), nil)
	}
}

func (w *Writer) Beat(id, k int) {
	if k < 1 || k > w.ActMax {
		return
	}
	n := protocol.SlotNumber(id, w.Slots)
	path := filepath.Join(w.root(), "act", protocol.Pad3(n), protocol.Pad2(k)+".wav")
	_ = atomicWrite(path, protocol.SilentWAV)
}

func (w *Writer) PresenceBeat() {
	dir := filepath.Join(w.root(), "presence")
	if _, err := os.Stat(dir); err != nil {
		return
	}
	if w.PresenceMax <= 0 {
		w.PresenceMax = 2000
	}
	w.Presence = (w.Presence % w.PresenceMax) + 1
	k := w.Presence
	_ = atomicWrite(filepath.Join(dir, protocol.Pad4(k)+".wav"), protocol.SilentWAV)
	for j := 1; j <= 50; j++ {
		n := ((k - 1 + j) % w.PresenceMax) + 1
		_ = atomicWrite(filepath.Join(dir, protocol.Pad4(n)+".wav"), nil)
	}
}
