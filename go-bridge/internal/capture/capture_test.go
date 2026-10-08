package capture_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/chelinho139/wow-ai/go-bridge/internal/capture"
	"github.com/chelinho139/wow-ai/go-bridge/internal/codec"
)

func TestDecodeRGBASearchExactBufferNoMagicIsNoMagic(t *testing.T) {
	cell, cells, rows := 4, 200, 48
	w, h := cells*cell, rows*cell
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	// Fill with mid-grey game-like pixels (no strip magic).
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{40, 40, 40, 255})
		}
	}
	_, err := capture.DecodeRGBASearch(img.Pix, w, h, img.Stride, true, cell, cells, rows)
	if err != codec.ErrNoMagic {
		t.Fatalf("want ErrNoMagic, got %v", err)
	}
}

func TestDecodeRGBASearchPaddedRoundTrip(t *testing.T) {
	cell, cells, rows := 4, 200, 48
	payload := []byte("sess\x1fchat\x1f1\x1f\x1fi=1\x1fname\x1fhello")
	vals := codec.Encode(7, payload)
	// Capture buffer padded by one cell for origin search.
	w, h := cells*cell+cell, rows*cell+cell
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, v := range vals {
		c := i % cells
		r := i / cells
		if r >= rows {
			break
		}
		cr, cg, cb := codec.CellColor(v)
		col := color.RGBA{uint8(cr * 255), uint8(cg * 255), uint8(cb * 255), 255}
		for dy := 0; dy < cell; dy++ {
			for dx := 0; dx < cell; dx++ {
				img.SetRGBA(c*cell+dx, r*cell+dy, col)
			}
		}
	}
	msg, err := capture.DecodeRGBASearch(img.Pix, w, h, img.Stride, true, cell, cells, rows)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != 7 || string(msg.Payload) != string(payload) {
		t.Fatalf("got id=%d payload=%q", msg.ID, msg.Payload)
	}
}
