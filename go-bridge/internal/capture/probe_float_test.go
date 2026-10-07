package capture_test

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/chelinho139/wow-ai/go-bridge/internal/capture"
)

func TestWarmaneProbeFloatDecode(t *testing.T) {
	candidates, err := filepath.Glob("/mnt/c/Users/*/AppData/Roaming/wow-ai-bridge/probes/probe-i1-*.png")
	if err != nil || len(candidates) == 0 {
		t.Skip("no local probe png")
	}
	var path string
	var f *os.File
	for _, p := range candidates {
		f, err = os.Open(p)
		if err == nil {
			path = p
			break
		}
	}
	if f == nil {
		t.Skip(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	// Old probes were 804px wide (ok for ~3px cells). Oversized ~7px cells need ~1400px.
	if rgba.Rect.Dx() < 1000 {
		t.Skip("probe too narrow for current oversized-cell fixture; live capture uses a wider grab")
	}
	msg, err := capture.DecodeRGBAFloatSearch(rgba.Pix, rgba.Rect.Dx(), rgba.Rect.Dy(), rgba.Stride, true, 200, 48)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	if msg.ID == 0 || len(msg.Payload) < 10 {
		t.Fatalf("id=%d payload=%q", msg.ID, msg.Payload)
	}
}
