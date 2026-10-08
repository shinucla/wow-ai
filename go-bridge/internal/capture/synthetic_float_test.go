package capture_test

import (
	"image"
	"image/color"
	"testing"

	"github.com/chelinho139/wow-ai/go-bridge/internal/capture"
	"github.com/chelinho139/wow-ai/go-bridge/internal/codec"
)

func TestFloatDecodeSynthetic288(t *testing.T) {
	payload := []byte("sess\x1fchat\x1f9\x1f\x1fi=1\x1fName\x1fhello world")
	vals := codec.Encode(9, payload)
	cellF := 2.88
	const cells, rows = 200, 48
	w := int(float64(cells)*cellF) + 8
	h := int(float64(rows)*cellF) + 8
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i, v := range vals {
		c := i % cells
		r := i / cells
		if r >= rows {
			break
		}
		cr, cg, cb := codec.CellColor(v)
		col := color.RGBA{uint8(cr * 255), uint8(cg * 255), uint8(cb * 255), 255}
		x0 := int(float64(c) * cellF)
		y0 := int(float64(r) * cellF)
		x1 := int(float64(c+1) * cellF)
		y1 := int(float64(r+1) * cellF)
		for y := y0; y < y1; y++ {
			for x := x0; x < x1; x++ {
				img.SetRGBA(x, y, col)
			}
		}
	}
	msg, err := capture.DecodeRGBAFloatSearch(img.Pix, w, h, img.Stride, true, cells, rows)
	if err != nil {
		t.Fatal(err)
	}
	if msg.ID != 9 || string(msg.Payload) != string(payload) {
		t.Fatalf("got id=%d payload=%q", msg.ID, msg.Payload)
	}
}
