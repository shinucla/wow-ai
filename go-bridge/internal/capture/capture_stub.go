//go:build !windows

package capture

import (
	"context"
	"time"

	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
)

// StubRunner is used on non-Windows builds (dev/test). Use --inject for messages.
type StubRunner struct{}

func NewRunner() Runner { return &StubRunner{} }

func (r *StubRunner) Run(ctx context.Context, cfg config.CaptureConfig, maxInstances int, out chan<- Event) {
	out <- Event{Info: "capture: Windows GDI only — use --inject or build with GOOS=windows"}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			out <- Event{Info: "waiting for Windows capture (this build has no GDI)"}
		}
	}
}

func probeImpl(cfg config.CaptureConfig, maxInstances int, outDir string) ProbeResult {
	return ProbeResult{
		ProcessName: cfg.ProcessName,
		Error:       "probe only works in the Windows .exe build (this binary has no GDI capture)",
	}
}
