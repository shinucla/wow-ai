package main

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/chelinho139/wow-ai/go-bridge/internal/codec"
)

func main() {
	f, _ := os.Open(os.Args[1])
	defer f.Close()
	img, _ := png.Decode(f)
	b := img.Bounds()
	rgba := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			rgba.Set(x, y, img.At(x, y))
		}
	}
	// dump first 40 cell values at cell=8 ox=0..2 oy=0..2
	for _, cell := range []float64{7.0, 7.5, 7.75, 8.0, 8.25, 8.5, 9.0} {
		for ox := 0.0; ox <= 2.0; ox += 0.25 {
			for oy := 0.0; oy <= 2.0; oy += 0.25 {
				vals := make([]byte, 16)
				for i := range vals {
					x := int(ox + (float64(i)+0.5)*cell)
					y := int(oy + 0.5*cell)
					if x >= rgba.Rect.Dx() || y >= rgba.Rect.Dy() {
						continue
					}
					off := y*rgba.Stride + x*4
					vals[i] = codec.CellValue(codec.Sample{R: rgba.Pix[off], G: rgba.Pix[off+1], B: rgba.Pix[off+2]})
				}
				if vals[0] == 6 && vals[1] == 1 && vals[2] == 6 && vals[3] == 1 {
					msg, err := try(rgba, cell, ox, oy)
					fmt.Printf("pattern cell=%.2f ox=%.2f oy=%.2f cells=%v err=%v\n", cell, ox, oy, vals, err)
					if msg != nil {
						fmt.Printf("  OK id=%d %q\n", msg.ID, trunc(string(msg.Payload), 100))
					}
				}
			}
		}
	}
}

func try(img *image.RGBA, cell, ox, oy float64) (*codec.Message, error) {
	const cells, maxRows = 200, 48
	samples := make([]codec.Sample, cells*maxRows)
	for r := 0; r < maxRows; r++ {
		for c := 0; c < cells; c++ {
			x := int(ox + (float64(c)+0.5)*cell)
			y := int(oy + (float64(r)+0.5)*cell)
			if x < 0 || y < 0 || x >= img.Rect.Dx() || y >= img.Rect.Dy() {
				continue
			}
			off := y*img.Stride + x*4
			samples[r*cells+c] = codec.Sample{R: img.Pix[off], G: img.Pix[off+1], B: img.Pix[off+2]}
		}
	}
	return codec.DecodeSamples(samples, cells, maxRows)
}
func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
