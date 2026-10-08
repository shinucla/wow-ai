package capture

import (
	"bytes"
	"context"
	"fmt"
	"sort"

	"github.com/chelinho139/wow-ai/go-bridge/internal/codec"
	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
)

// Event is one line of capture status or a decoded message from one client window.
type Event struct {
	Info     string
	Warn     string
	Error    string
	ID       uint16
	Payload  []byte
	HWND     uintptr
	PID      uint32
	Left     int32
	Top      int32
	Title    string
	Instance int // 1-based, by screen position (left→right, top→bottom)
}

// Runner continuously captures game window corners and emits events.
type Runner interface {
	Run(ctx context.Context, cfg config.CaptureConfig, maxInstances int, out chan<- Event)
}

// ProbeWindow is one found game window for diagnostics.
type ProbeWindow struct {
	Instance       int    `json:"instance"`
	HWND           string `json:"hwnd"`
	PID            uint32 `json:"pid"`
	Title          string `json:"title"`
	WindowLeft     int32  `json:"windowLeft"`
	WindowTop      int32  `json:"windowTop"`
	ClientScreenX  int32  `json:"clientScreenX"`
	ClientScreenY  int32  `json:"clientScreenY"`
	ImagePath      string `json:"imagePath,omitempty"`
	Decode         string `json:"decode"`
	PayloadPreview string `json:"payloadPreview,omitempty"`
}

// ProbeResult is a one-shot capture diagnostic.
type ProbeResult struct {
	ProcessName string        `json:"processName"`
	Windows     []ProbeWindow `json:"windows"`
	Note        string        `json:"note,omitempty"`
	Error       string        `json:"error,omitempty"`
}

// Probe finds WoW windows, grabs each client top-left, optionally saves PNGs, and tries decode.
func Probe(cfg config.CaptureConfig, maxInstances int, outDir string) ProbeResult {
	return probeImpl(cfg, maxInstances, outDir)
}

// DecodeRGBA samples a BGRA/RGBA buffer as cell centers and decodes the strip.
// pix is row-major, stride bytes per row, bpp = 4, RGB order if rgb=true else BGR.
func DecodeRGBA(pix []byte, width, height, stride int, rgb bool, cell, cells, maxRows int) (*codec.Message, error) {
	return decodeRGBAAt(pix, width, height, stride, rgb, cell, cells, maxRows, 0, 0)
}

// DecodeRGBASearch tries a small origin offset (like capture_x11.py) so a
// slightly misaligned client corner still finds the magic.
// Only offsets that fit in the buffer are tried; a too-small buffer for an
// offset is skipped (not reported as truncated), so a normal "no strip" frame
// stays ErrNoMagic instead of a false truncated warning.
func DecodeRGBASearch(pix []byte, width, height, stride int, rgb bool, cell, cells, maxRows int) (*codec.Message, error) {
	if cell <= 0 {
		cell = codec.DefaultCell
	}
	if cells <= 0 {
		cells = codec.CellsPerRow
	}
	if maxRows <= 0 {
		maxRows = codec.MaxRows
	}
	originErr := error(codec.ErrNoMagic)
	for oy := 0; oy <= cell; oy++ {
		for ox := 0; ox <= cell; ox++ {
			// Need room for at least a magic-sized prefix at this origin.
			if width < ox+6*cell || height < oy+cell {
				continue
			}
			msg, err := decodeRGBAAt(pix, width, height, stride, rgb, cell, cells, maxRows, ox, oy)
			if err == nil {
				return msg, nil
			}
			if ox == 0 && oy == 0 {
				originErr = err
			}
		}
	}
	return nil, originErr
}

func decodeRGBAAt(pix []byte, width, height, stride int, rgb bool, cell, cells, maxRows, ox, oy int) (*codec.Message, error) {
	if cell <= 0 {
		cell = codec.DefaultCell
	}
	if cells <= 0 {
		cells = codec.CellsPerRow
	}
	if maxRows <= 0 {
		maxRows = codec.MaxRows
	}
	colsFit := (width - ox) / cell
	rowsFit := (height - oy) / cell
	if colsFit < 6 || rowsFit < 1 {
		return nil, codec.ErrTruncated
	}
	cols, rows := cells, maxRows
	if colsFit < cells {
		cols, rows = colsFit, 1
	} else if rowsFit < maxRows {
		rows = rowsFit
	}
	samples := make([]codec.Sample, 0, cols*rows)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			x := ox + c*cell + cell/2
			y := oy + r*cell + cell/2
			off := y*stride + x*4
			if off+3 >= len(pix) {
				return nil, codec.ErrTruncated
			}
			var s codec.Sample
			if rgb {
				s.R, s.G, s.B = pix[off], pix[off+1], pix[off+2]
			} else {
				s.B, s.G, s.R = pix[off], pix[off+1], pix[off+2]
			}
			samples = append(samples, s)
		}
	}
	return codec.DecodeSamples(samples, cells, maxRows)
}

// DecodeFlexible tries the configured cell size first, then nearby integer sizes.
func DecodeFlexible(pix []byte, width, height, stride int, rgb bool, cell, cells, maxRows int) (*codec.Message, error) {
	if cell <= 0 {
		cell = codec.DefaultCell
	}
	tried := map[int]bool{}
	var last error = codec.ErrNoMagic
	for _, c := range []int{cell, 4, 3, 5, 2, 6, 7, 8, 9, 10, 12} {
		if c <= 0 || tried[c] {
			continue
		}
		tried[c] = true
		msg, err := DecodeRGBASearch(pix, width, height, stride, rgb, c, cells, maxRows)
		if err == nil {
			return msg, nil
		}
		if err != codec.ErrNoMagic {
			last = err
		}
	}
	return nil, last
}

// DecodeHint holds a previously successful float sampling grid.
type DecodeHint struct {
	Cell, Ox, Oy float64
	OK           bool
}

// DecodeRGBAFloatSearch tries fractional cell sizes (and sub-pixel origins).
func DecodeRGBAFloatSearch(pix []byte, width, height, stride int, rgb bool, cells, maxRows int) (*codec.Message, error) {
	_, msg, err := DecodeRGBAFloatSearchHint(pix, width, height, stride, rgb, cells, maxRows, DecodeHint{})
	return msg, err
}

func sampleRGB(pix []byte, width, height, stride int, rgb bool, x, y int) codec.Sample {
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x >= width {
		x = width - 1
	}
	if y >= height {
		y = height - 1
	}
	off := y*stride + x*4
	if rgb {
		return codec.Sample{R: pix[off], G: pix[off+1], B: pix[off+2]}
	}
	return codec.Sample{B: pix[off], G: pix[off+1], R: pix[off+2]}
}

// estimateCellFromEdges finds strip cell size from color transitions near the top.
// Much cheaper than a blind float grid when PrintWindow anti-aliases cell borders.
func estimateCellFromEdges(pix []byte, width, height, stride int, rgb bool) (cell float64, ok bool) {
	if width < 32 || height < 4 {
		return 0, false
	}
	bestY, bestScore := 1, -1
	limY := 24
	if limY > height-1 {
		limY = height - 1
	}
	limX := width
	if limX > 400 {
		limX = 400
	}
	for y := 1; y <= limY; y++ {
		score := 0
		prev := sampleRGB(pix, width, height, stride, rgb, 0, y)
		for x := 1; x < limX; x++ {
			s := sampleRGB(pix, width, height, stride, rgb, x, y)
			d := absInt(int(s.R)-int(prev.R)) + absInt(int(s.G)-int(prev.G)) + absInt(int(s.B)-int(prev.B))
			if d > 90 {
				score++
			}
			prev = s
		}
		if score > bestScore {
			bestScore, bestY = score, y
		}
	}
	if bestScore < 4 {
		return 0, false
	}
	var gaps []int
	prev := sampleRGB(pix, width, height, stride, rgb, 0, bestY)
	lastEdge := 0
	for x := 1; x < limX; x++ {
		s := sampleRGB(pix, width, height, stride, rgb, x, bestY)
		d := absInt(int(s.R)-int(prev.R)) + absInt(int(s.G)-int(prev.G)) + absInt(int(s.B)-int(prev.B))
		if d > 90 {
			gap := x - lastEdge
			if gap >= 2 && gap <= 30 {
				gaps = append(gaps, gap)
			}
			lastEdge = x
		}
		prev = s
	}
	if len(gaps) < 3 {
		return 0, false
	}
	// Anti-aliased borders often produce pairs of close edges; keep larger spacings.
	var spaced []int
	for _, g := range gaps {
		if g >= 3 {
			spaced = append(spaced, g)
		}
	}
	if len(spaced) < 2 {
		spaced = gaps
	}
	sort.Ints(spaced)
	med := spaced[len(spaced)/2]
	if med < 2 {
		return 0, false
	}
	return float64(med), true
}

// DecodeRGBAFloatSearchHint tries hint first, then edge estimate + tight refine,
// then a coarse band scan. Designed to lock in well under a second on typical grabs.
func DecodeRGBAFloatSearchHint(pix []byte, width, height, stride int, rgb bool, cells, maxRows int, hint DecodeHint) (DecodeHint, *codec.Message, error) {
	if cells <= 0 {
		cells = codec.CellsPerRow
	}
	if maxRows <= 0 {
		maxRows = codec.MaxRows
	}

	hasMagic := func(cell, ox, oy float64) bool {
		if cell < 1.5 {
			return false
		}
		const n = 8
		samples := make([]codec.Sample, 0, n)
		for c := 0; c < n; c++ {
			x := int(ox + (float64(c)+0.5)*cell)
			y := int(oy + 0.5*cell)
			if x < 0 || y < 0 || x >= width || y >= height {
				return false
			}
			samples = append(samples, sampleRGB(pix, width, height, stride, rgb, x, y))
		}
		_, err := codec.DecodeSamples(samples, cells, maxRows)
		return err != codec.ErrNoMagic
	}

	try := func(cell, ox, oy float64) (*codec.Message, error) {
		if cell < 1.5 {
			return nil, codec.ErrTruncated
		}
		colsFit := int((float64(width) - ox) / cell)
		rowsFit := int((float64(height) - oy) / cell)
		if colsFit < 6 || rowsFit < 1 {
			return nil, codec.ErrTruncated
		}
		cols := cells
		if colsFit < cells {
			cols = colsFit
		}
		maxR := maxRows
		if rowsFit < maxR {
			maxR = rowsFit
		}
		if colsFit < cells {
			maxR = 1
		}
		var last error = codec.ErrTruncated
		for rows := 1; rows <= maxR; rows++ {
			samples := make([]codec.Sample, 0, cols*rows)
			for r := 0; r < rows; r++ {
				for c := 0; c < cols; c++ {
					x := int(ox + (float64(c)+0.5)*cell)
					y := int(oy + (float64(r)+0.5)*cell)
					if x < 0 || y < 0 || x >= width || y >= height {
						return nil, codec.ErrTruncated
					}
					samples = append(samples, sampleRGB(pix, width, height, stride, rgb, x, y))
				}
			}
			msg, err := codec.DecodeSamples(samples, cells, maxRows)
			if err == nil {
				return msg, nil
			}
			last = err
			if err != codec.ErrTruncated {
				return nil, err
			}
		}
		return nil, last
	}

	tryOK := func(cell, ox, oy float64) (*codec.Message, error) {
		msg, err := try(cell, ox, oy)
		if err != nil {
			return nil, err
		}
		if !plausibleStrip(msg) {
			return nil, codec.ErrChecksum
		}
		return msg, nil
	}

	// oy=0 first — PrintWindow strips are usually flush to the client top.
	refineFromEst := func(est float64) (DecodeHint, *codec.Message, bool) {
		cellLo, cellHi := est-0.75, est+0.75
		if cellLo < 1.5 {
			cellLo = 1.5
		}
		for oy := 0.0; oy <= 2.0+1e-9; oy += 0.1 {
			for ox := 0.0; ox <= 3.0+1e-9; ox += 0.1 {
				for cell := cellLo; cell <= cellHi+1e-9; cell += 0.02 {
					if !hasMagic(cell, ox, oy) {
						continue
					}
					if msg, err := tryOK(cell, ox, oy); err == nil {
						return DecodeHint{Cell: cell, Ox: ox, Oy: oy, OK: true}, msg, true
					}
				}
			}
		}
		return DecodeHint{}, nil, false
	}

	refineNear := func(cell0, ox0, oy0 float64) (DecodeHint, *codec.Message, bool) {
		for dc := -0.2; dc <= 0.2+1e-9; dc += 0.01 {
			cell := cell0 + dc
			if cell < 1.5 {
				continue
			}
			for dx := -0.4; dx <= 0.4+1e-9; dx += 0.05 {
				for dy := -0.4; dy <= 0.4+1e-9; dy += 0.05 {
					ox, oy := ox0+dx, oy0+dy
					if ox < 0 || oy < 0 {
						continue
					}
					if msg, err := tryOK(cell, ox, oy); err == nil {
						return DecodeHint{Cell: cell, Ox: ox, Oy: oy, OK: true}, msg, true
					}
				}
			}
		}
		return DecodeHint{}, nil, false
	}

	if hint.OK && hint.Cell > 0 {
		if msg, err := tryOK(hint.Cell, hint.Ox, hint.Oy); err == nil {
			return hint, msg, nil
		}
		if h, msg, ok := refineNear(hint.Cell, hint.Ox, hint.Oy); ok {
			return h, msg, nil
		}
	}

	if est, ok := estimateCellFromEdges(pix, width, height, stride, rgb); ok {
		if h, msg, ok := refineFromEst(est); ok {
			return h, msg, nil
		}
		for _, d := range []float64{-1.0, 1.0} {
			if h, msg, ok := refineFromEst(est + d); ok {
				return h, msg, nil
			}
		}
	}

	for _, cell := range []float64{4, 5, 6, 6.5, 7, 7.5, 8, 9, 3, 2.5, 10, 12} {
		if h, msg, ok := refineFromEst(cell); ok {
			return h, msg, nil
		}
	}
	return DecodeHint{}, nil, codec.ErrNoMagic
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// plausibleStrip rejects Fletcher collisions that decode as binary garbage.
// Real strip payloads are US-separated wire records with mostly printable text.
func plausibleStrip(msg *codec.Message) bool {
	if msg == nil || len(msg.Payload) < 3 {
		return false
	}
	p := msg.Payload
	if bytes.Count(p, []byte{0x1f}) < 2 && bytes.Count(p, []byte{0x1e}) < 1 {
		// Single-field hello-ish payloads still need printable text.
		printable := 0
		for _, b := range p {
			if b >= 32 && b < 127 {
				printable++
			}
		}
		return printable*100/len(p) >= 85
	}
	printable := 0
	for _, b := range p {
		if b == 0x1f || b == 0x1e || b == '\n' || b == '\t' || (b >= 32 && b < 127) {
			printable++
		}
	}
	return printable*100/len(p) >= 70
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func fmtErr(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprint(err)
}
