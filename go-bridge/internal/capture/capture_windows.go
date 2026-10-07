//go:build windows

package capture

import (
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/chelinho139/wow-ai/go-bridge/internal/codec"
	"github.com/chelinho139/wow-ai/go-bridge/internal/config"
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	gdi32                          = syscall.NewLazyDLL("gdi32.dll")
	procEnumWindows                = user32.NewProc("EnumWindows")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
	procGetWindowTextW             = user32.NewProc("GetWindowTextW")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procIsIconic                   = user32.NewProc("IsIconic")
	procGetWindowRect              = user32.NewProc("GetWindowRect")
	procGetClientRect              = user32.NewProc("GetClientRect")
	procGetDC                      = user32.NewProc("GetDC")
	procReleaseDC                  = user32.NewProc("ReleaseDC")
	procClientToScreen             = user32.NewProc("ClientToScreen")
	procSetProcessDPIAware         = user32.NewProc("SetProcessDPIAware")
	procPrintWindow                = user32.NewProc("PrintWindow")
	procCreateCompatibleDC         = gdi32.NewProc("CreateCompatibleDC")
	procCreateCompatibleBitmap     = gdi32.NewProc("CreateCompatibleBitmap")
	procSelectObject               = gdi32.NewProc("SelectObject")
	procBitBlt                     = gdi32.NewProc("BitBlt")
	procDeleteObject               = gdi32.NewProc("DeleteObject")
	procDeleteDC                   = gdi32.NewProc("DeleteDC")
	procGetDIBits                  = gdi32.NewProc("GetDIBits")
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procCloseHandle                = kernel32.NewProc("CloseHandle")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
)

var dpiOnce sync.Once

const (
	srcCopy             = 0x00CC0020
	processQueryLimited = 0x1000
	biRGB               = 0
	dibRGBColors        = 0
	pwClientOnly        = 0x1
	pwRenderFullContent = 0x2
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type winInfo struct {
	HWND         syscall.Handle
	PID          uint32
	Left, Top    int32
	Title        string
	Instance     int
}

// WindowsRunner captures every matching WoW window via GDI.
type WindowsRunner struct{}

func NewRunner() Runner { return &WindowsRunner{} }

func (r *WindowsRunner) Run(ctx context.Context, cfg config.CaptureConfig, maxInstances int, out chan<- Event) {
	if maxInstances <= 0 {
		maxInstances = 4
	}
	interval := time.Duration(cfg.IntervalMs) * time.Millisecond
	if interval < 50*time.Millisecond {
		interval = 250 * time.Millisecond
	}
	cell := cfg.CellPx
	if cell <= 0 {
		cell = codec.DefaultCell
	}
	cells := cfg.CellsPerRow
	if cells <= 0 {
		cells = codec.CellsPerRow
	}
	rows := cfg.MaxRows
	if rows <= 0 {
		rows = codec.MaxRows
	}
	// Grab enough for oversized cells (2.8px…~24px). At 4K, bad UI-scale cancel
	// can push cells well past 9px; search + grab must cover that.
	capCell := 24
	if cell > capCell {
		capCell = cell
	}
	w := cells*capCell + capCell
	h := rows*capCell + capCell

	lastKey := map[uintptr]string{}
	var lastWarn time.Time
	var lastHint time.Time
	var lastSig string
	decodeHint := map[uintptr]DecodeHint{}
	clientSize := map[uintptr][2]int{} // hwnd → last client W×H
	gdiCache := map[uintptr]*hwndCapture{}
	defer func() {
		for _, c := range gdiCache {
			c.release()
		}
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	rediscover := time.NewTicker(2 * time.Second)
	defer rediscover.Stop()

	var windows []winInfo

	for {
		select {
		case <-ctx.Done():
			return
		case <-rediscover.C:
			windows = listWindows(cfg.ProcessName, cfg.WindowTitle, maxInstances)
			sig := windowSignature(windows)
			if sig != lastSig {
				lastSig = sig
				if len(windows) == 0 {
					out <- Event{Info: "waiting for " + cfg.ProcessName + " window(s)"}
				} else {
					for _, wi := range windows {
						cx, cy := int32(0), int32(0)
						if x, y, err := clientOrigin(wi.HWND); err == nil {
							cx, cy = x, y
						}
						out <- Event{
							Info: fmt.Sprintf(
								"instance %d: %q pid=%d window=(%d,%d) clientOrigin=(%d,%d) — HWND capture (ok behind other windows)",
								wi.Instance, wi.Title, wi.PID, wi.Left, wi.Top, cx, cy,
							),
							HWND:     uintptr(wi.HWND),
							PID:      wi.PID,
							Left:     wi.Left,
							Top:      wi.Top,
							Title:    wi.Title,
							Instance: wi.Instance,
						}
					}
					out <- Event{Info: fmt.Sprintf("tracking %d client(s)", len(windows))}
				}
			}
		case <-ticker.C:
			if len(windows) == 0 {
				windows = listWindows(cfg.ProcessName, cfg.WindowTitle, maxInstances)
			}
			for _, wi := range windows {
				if !isWindow(wi.HWND) || isIconic(wi.HWND) {
					continue
				}
				hwndKey := uintptr(wi.HWND)
				cw, ch, sizeOK := clientDims(wi.HWND)
				if sizeOK {
					prev := clientSize[hwndKey]
					if prev[0] > 0 && (absInt(cw-prev[0]) > 2 || absInt(ch-prev[1]) > 2) {
						if decodeHint[hwndKey].OK {
							out <- Event{Info: fmt.Sprintf(
								"instance %d: client resized %dx%d → %dx%d, re-locking strip",
								wi.Instance, prev[0], prev[1], cw, ch,
							), Instance: wi.Instance}
						}
						delete(decodeHint, hwndKey)
						if c := gdiCache[hwndKey]; c != nil {
							c.release()
							delete(gdiCache, hwndKey)
						}
						lastHint = time.Time{}
					}
					clientSize[hwndKey] = [2]int{cw, ch}
				}
				grabW, grabH := w, h
				if sizeOK {
					if grabW > cw {
						grabW = cw
					}
					if grabH > ch {
						grabH = ch
					}
				}
				// HWND / PrintWindow only — never desktop BitBlt (that sees covering windows).
				cache := gdiCache[hwndKey]
				if cache == nil {
					cache = &hwndCapture{}
					gdiCache[hwndKey] = cache
				}
				img, err := capturePrintWindowCornerCached(wi.HWND, grabW, grabH, cache)
				if err != nil {
					if time.Since(lastWarn) >= 5*time.Second {
						lastWarn = time.Now()
						out <- Event{Warn: fmt.Sprintf("instance %d HWND capture: %v", wi.Instance, err), Instance: wi.Instance, HWND: hwndKey}
					}
					continue
				}
				msg, err := DecodeFlexible(img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride, true, cell, cells, rows)
				prevHint := decodeHint[hwndKey]
				needFloat := err != nil && (err == codec.ErrChecksum || err == codec.ErrLength || prevHint.OK || time.Since(lastHint) >= 5*time.Second)
				if needFloat {
					searchFrom := prevHint
					if prevHint.OK && (err == codec.ErrChecksum || err == codec.ErrLength || err == codec.ErrNoMagic) {
						searchFrom = DecodeHint{}
					}
					var hint DecodeHint
					hint, msg, err = DecodeRGBAFloatSearchHint(img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride, true, cells, rows, searchFrom)
					if err == nil {
						decodeHint[hwndKey] = hint
						if !prevHint.OK || absFloat(prevHint.Cell-hint.Cell) > 0.15 {
							out <- Event{Info: fmt.Sprintf("instance %d: locked cell=%.2fpx origin=(%.2f,%.2f) via HWND", wi.Instance, hint.Cell, hint.Ox, hint.Oy), Instance: wi.Instance}
						}
					} else if prevHint.OK {
						delete(decodeHint, hwndKey)
					}
					lastHint = time.Now()
				}
				if err != nil {
					if err != codec.ErrNoMagic && time.Since(lastWarn) >= 5*time.Second {
						lastWarn = time.Now()
						out <- Event{Warn: fmt.Sprintf("instance %d strip rejected: %v (HWND)", wi.Instance, err), Instance: wi.Instance}
					} else if err == codec.ErrNoMagic && time.Since(lastHint) >= 5*time.Second {
						lastHint = time.Now()
						black := mostlyBlack(img)
						out <- Event{Info: fmt.Sprintf(
							"instance %d: no strip yet via HWND (black=%v). Leave the color blocks visible in-game, then Probe",
							wi.Instance, black,
						)}
					}
					continue
				}
				key := fmt.Sprintf("%d:%s", msg.ID, string(msg.Payload))
				if lastKey[hwndKey] == key {
					continue
				}
				lastKey[hwndKey] = key
				out <- Event{
					ID:       msg.ID,
					Payload:  append([]byte(nil), msg.Payload...),
					HWND:     hwndKey,
					PID:      wi.PID,
					Left:     wi.Left,
					Top:      wi.Top,
					Title:    wi.Title,
					Instance: wi.Instance,
				}
			}
		}
	}
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func clientDims(hwnd syscall.Handle) (w, h int, ok bool) {
	var cr rect
	r, _, _ := procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&cr)))
	if r == 0 {
		return 0, 0, false
	}
	return int(cr.Right - cr.Left), int(cr.Bottom - cr.Top), true
}

func windowSignature(wins []winInfo) string {
	// HWND only — maximized windows jitter Left/Top by a few pixels every tick.
	s := fmt.Sprintf("%d", len(wins))
	for _, w := range wins {
		s += fmt.Sprintf("|%d", w.HWND)
	}
	return s
}

func listWindows(processName, titleSub string, maxInstances int) []winInfo {
	var found []winInfo
	cb := syscall.NewCallback(func(hwnd syscall.Handle, _ uintptr) uintptr {
		vis, _, _ := procIsWindowVisible.Call(uintptr(hwnd))
		if vis == 0 {
			return 1
		}
		if titleSub != "" {
			t := windowTitle(hwnd)
			if !containsFold(t, titleSub) {
				return 1
			}
		}
		var pid uint32
		procGetWindowThreadProcessId.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pid)))
		if processName != "" {
			name := processBaseName(pid)
			if !equalsFold(name, processName) && !equalsFold(name, processName+".exe") {
				return 1
			}
		}
		var wr rect
		procGetWindowRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&wr)))
		found = append(found, winInfo{
			HWND:  hwnd,
			PID:   pid,
			Left:  wr.Left,
			Top:   wr.Top,
			Title: windowTitle(hwnd),
		})
		return 1
	})
	procEnumWindows.Call(cb, 0)

	sort.Slice(found, func(i, j int) bool {
		if found[i].Left != found[j].Left {
			return found[i].Left < found[j].Left
		}
		if found[i].Top != found[j].Top {
			return found[i].Top < found[j].Top
		}
		return found[i].HWND < found[j].HWND
	})
	if len(found) > maxInstances {
		found = found[:maxInstances]
	}
	for i := range found {
		found[i].Instance = i + 1
	}
	return found
}

func isWindow(h syscall.Handle) bool {
	r, _, _ := procIsWindowVisible.Call(uintptr(h))
	return r != 0
}

func isIconic(h syscall.Handle) bool {
	r, _, _ := procIsIconic.Call(uintptr(h))
	return r != 0
}

func windowTitle(h syscall.Handle) string {
	buf := make([]uint16, 512)
	procGetWindowTextW.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	return syscall.UTF16ToString(buf)
}

func processBaseName(pid uint32) string {
	h, _, _ := procOpenProcess.Call(processQueryLimited, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer procCloseHandle.Call(h)
	buf := make([]uint16, 260)
	size := uint32(len(buf))
	r, _, _ := procQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		return ""
	}
	full := syscall.UTF16ToString(buf)
	for i := len(full) - 1; i >= 0; i-- {
		if full[i] == '\\' || full[i] == '/' {
			return full[i+1:]
		}
	}
	return full
}

func containsFold(s, sub string) bool {
	return len(sub) == 0 || indexFold(s, sub) >= 0
}

func equalsFold(a, b string) bool {
	if len(a) != len(b) {
		if len(a) > 4 && (a[len(a)-4:] == ".exe" || a[len(a)-4:] == ".EXE") {
			a = a[:len(a)-4]
		}
		if len(b) > 4 && (b[len(b)-4:] == ".exe" || b[len(b)-4:] == ".EXE") {
			b = b[:len(b)-4]
		}
	}
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

func indexFold(s, sub string) int {
	ls, lsub := len(s), len(sub)
	if lsub == 0 {
		return 0
	}
	for i := 0; i+lsub <= ls; i++ {
		ok := true
		for j := 0; j < lsub; j++ {
			ca, cb := s[i+j], sub[j]
			if ca >= 'A' && ca <= 'Z' {
				ca += 32
			}
			if cb >= 'A' && cb <= 'Z' {
				cb += 32
			}
			if ca != cb {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

func ensureDPIAware() {
	dpiOnce.Do(func() {
		// Per-monitor DPI (Win10+): ClientToScreen/BitBlt match the real 4K framebuffer.
		if p := user32.NewProc("SetProcessDpiAwarenessContext"); p.Find() == nil {
			const dpiContextPerMonitorAwareV2 = ^uintptr(3) // HANDLE(-4)
			if r, _, _ := p.Call(dpiContextPerMonitorAwareV2); r != 0 {
				return
			}
		}
		procSetProcessDPIAware.Call()
	})
}

func mostlyBlack(img *image.RGBA) bool {
	if img == nil || len(img.Pix) < 16 {
		return true
	}
	// Sample a grid in the top-left 80x40.
	w, h := img.Rect.Dx(), img.Rect.Dy()
	limitW, limitH := w, h
	if limitW > 80 {
		limitW = 80
	}
	if limitH > 40 {
		limitH = 40
	}
	var sum int
	var n int
	for y := 0; y < limitH; y += 4 {
		for x := 0; x < limitW; x += 4 {
			off := y*img.Stride + x*4
			sum += int(img.Pix[off]) + int(img.Pix[off+1]) + int(img.Pix[off+2])
			n++
		}
	}
	if n == 0 {
		return true
	}
	return sum/n < 24 // average channel sum very dark
}

func dibToRGBA(buf []byte, w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < w*h; i++ {
		off := i * 4
		img.Pix[off+0] = buf[off+2]
		img.Pix[off+1] = buf[off+1]
		img.Pix[off+2] = buf[off+0]
		img.Pix[off+3] = 255
	}
	return img
}

func getDIBitsRGBA(memDC, bmp uintptr, w, h int) (*image.RGBA, error) {
	bi := bitmapInfo{}
	bi.Header.Size = uint32(unsafe.Sizeof(bi.Header))
	bi.Header.Width = int32(w)
	bi.Header.Height = -int32(h)
	bi.Header.Planes = 1
	bi.Header.BitCount = 32
	bi.Header.Compression = biRGB
	buf := make([]byte, w*h*4)
	ret, _, _ := procGetDIBits.Call(memDC, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&bi)), dibRGBColors)
	if ret == 0 {
		return nil, fmt.Errorf("GetDIBits failed")
	}
	return dibToRGBA(buf, w, h), nil
}

// hwndCapture holds reusable GDI objects for PrintWindow of one HWND.
type hwndCapture struct {
	hwnd       syscall.Handle
	cw, ch     int
	winDC      uintptr
	memDC      uintptr
	bmp        uintptr
	old        uintptr
	flags      uintptr
	flagsKnown bool
}

func (c *hwndCapture) release() {
	if c.memDC != 0 && c.old != 0 {
		procSelectObject.Call(c.memDC, c.old)
	}
	if c.bmp != 0 {
		procDeleteObject.Call(c.bmp)
		c.bmp = 0
	}
	if c.memDC != 0 {
		procDeleteDC.Call(c.memDC)
		c.memDC = 0
	}
	if c.winDC != 0 {
		procReleaseDC.Call(uintptr(c.hwnd), c.winDC)
		c.winDC = 0
	}
	c.old = 0
	c.cw, c.ch = 0, 0
}

func (c *hwndCapture) ensure(hwnd syscall.Handle, cw, ch int) error {
	if c.hwnd == hwnd && c.cw == cw && c.ch == ch && c.bmp != 0 && c.memDC != 0 {
		return nil
	}
	c.release()
	c.hwnd = hwnd
	c.flagsKnown = false
	winDC, _, _ := procGetDC.Call(uintptr(hwnd))
	if winDC == 0 {
		return fmt.Errorf("GetDC failed")
	}
	memDC, _, _ := procCreateCompatibleDC.Call(winDC)
	if memDC == 0 {
		procReleaseDC.Call(uintptr(hwnd), winDC)
		return fmt.Errorf("CreateCompatibleDC failed")
	}
	bmp, _, _ := procCreateCompatibleBitmap.Call(winDC, uintptr(cw), uintptr(ch))
	if bmp == 0 {
		procDeleteDC.Call(memDC)
		procReleaseDC.Call(uintptr(hwnd), winDC)
		return fmt.Errorf("CreateCompatibleBitmap(%dx%d) failed", cw, ch)
	}
	old, _, _ := procSelectObject.Call(memDC, bmp)
	c.winDC, c.memDC, c.bmp, c.old = winDC, memDC, bmp, old
	c.cw, c.ch = cw, ch
	return nil
}

// captureHwndClientCorner reads the game client's top-left strip rect via the
// window HWND (PrintWindow). Works even when other windows cover the game —
// never uses desktop/screen BitBlt for strip pixels.
func captureHwndClientCorner(hwnd syscall.Handle, w, h int) (*image.RGBA, error) {
	ensureDPIAware()
	return capturePrintWindowCorner(hwnd, w, h)
}

func capturePrintWindowCornerCached(hwnd syscall.Handle, w, h int, cache *hwndCapture) (*image.RGBA, error) {
	var cr rect
	ok, _, _ := procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&cr)))
	if ok == 0 {
		return nil, fmt.Errorf("GetClientRect failed")
	}
	cw := int(cr.Right - cr.Left)
	ch := int(cr.Bottom - cr.Top)
	if cw < 8 || ch < 8 {
		return nil, fmt.Errorf("client too small (%dx%d)", cw, ch)
	}
	if w > cw {
		w = cw
	}
	if h > ch {
		h = ch
	}

	local := cache
	owned := false
	if local == nil {
		local = &hwndCapture{}
		owned = true
	}
	if err := local.ensure(hwnd, cw, ch); err != nil {
		return nil, err
	}
	if owned {
		defer local.release()
	}

	printed := uintptr(0)
	flagOrder := []uintptr{
		pwClientOnly | pwRenderFullContent,
		pwRenderFullContent,
		pwClientOnly,
		0,
	}
	if local.flagsKnown {
		flagOrder = []uintptr{local.flags}
	}
	var printErr error
	for _, flags := range flagOrder {
		printed, _, _ = procPrintWindow.Call(uintptr(hwnd), local.memDC, flags)
		if printed != 0 {
			local.flags = flags
			local.flagsKnown = true
			printErr = nil
			break
		}
		printErr = fmt.Errorf("PrintWindow flags=0x%x failed", flags)
	}
	if printed == 0 {
		r, _, _ := procBitBlt.Call(local.memDC, 0, 0, uintptr(w), uintptr(h), local.winDC, 0, 0, srcCopy)
		if r == 0 {
			if printErr != nil {
				return nil, printErr
			}
			return nil, fmt.Errorf("PrintWindow + window BitBlt failed")
		}
		return getDIBitsRGBA(local.memDC, local.bmp, w, h)
	}

	full, err := getDIBitsRGBA(local.memDC, local.bmp, cw, ch)
	if err != nil {
		return nil, err
	}
	if w == cw && h == ch {
		return full, nil
	}
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		srcOff := y * full.Stride
		dstOff := y * out.Stride
		copy(out.Pix[dstOff:dstOff+w*4], full.Pix[srcOff:srcOff+w*4])
	}
	return out, nil
}

func capturePrintWindowCorner(hwnd syscall.Handle, w, h int) (*image.RGBA, error) {
	return capturePrintWindowCornerCached(hwnd, w, h, nil)
}


func clientOrigin(hwnd syscall.Handle) (int32, int32, error) {
	ensureDPIAware()
	pt := point{X: 0, Y: 0}
	ok, _, _ := procClientToScreen.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&pt)))
	if ok == 0 {
		return 0, 0, fmt.Errorf("ClientToScreen failed")
	}
	return pt.X, pt.Y, nil
}

func probeImpl(cfg config.CaptureConfig, maxInstances int, outDir string) ProbeResult {
	if maxInstances <= 0 {
		maxInstances = 4
	}
	name := cfg.ProcessName
	if name == "" {
		name = "Wow"
	}
	cell, cells, rows := cfg.CellPx, cfg.CellsPerRow, cfg.MaxRows
	if cell <= 0 {
		cell = codec.DefaultCell
	}
	if cells <= 0 {
		cells = codec.CellsPerRow
	}
	if rows <= 0 {
		rows = codec.MaxRows
	}
	w, h := cells*cell+cell, rows*cell+cell
	// Match live capture: room for oversized / 4K post-resize cells.
	capCell := 24
	if cell > capCell {
		capCell = cell
	}
	w, h = cells*capCell+capCell, rows*capCell+capCell

	wins := listWindows(name, cfg.WindowTitle, maxInstances)
	res := ProbeResult{
		ProcessName: name,
		Note:        "Captures the strip rect from the game HWND (PrintWindow). Works when the game is behind other windows — desktop/screen capture is not used.",
	}
	if len(wins) == 0 {
		res.Error = fmt.Sprintf("no visible windows for process %q — check Settings → process name (Task Manager → Details, without .exe)", name)
		return res
	}
	if outDir != "" {
		_ = os.MkdirAll(outDir, 0o755)
	}
	for _, wi := range wins {
		pw := ProbeWindow{
			Instance:   wi.Instance,
			HWND:       fmt.Sprintf("0x%x", wi.HWND),
			PID:        wi.PID,
			Title:      wi.Title,
			WindowLeft: wi.Left,
			WindowTop:  wi.Top,
		}
		if cx, cy, err := clientOrigin(wi.HWND); err == nil {
			pw.ClientScreenX, pw.ClientScreenY = cx, cy
		}
		img, err := captureHwndClientCorner(wi.HWND, w, h)
		if err != nil {
			pw.Decode = "HWND capture: " + err.Error()
			res.Windows = append(res.Windows, pw)
			continue
		}
		if outDir != "" {
			path := filepath.Join(outDir, fmt.Sprintf("probe-i%d-hwnd.png", wi.Instance))
			if f, err := os.Create(path); err == nil {
				_ = png.Encode(f, img)
				_ = f.Close()
				pw.ImagePath = path
			}
		}
		msg, derr := DecodeFlexible(img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride, true, cell, cells, rows)
		if derr != nil {
			_, msg, derr = DecodeRGBAFloatSearchHint(img.Pix, img.Rect.Dx(), img.Rect.Dy(), img.Stride, true, cells, rows, DecodeHint{})
		}
		if derr != nil {
			detail := derr.Error()
			if derr == codec.ErrNoMagic {
				detail = "no magic"
			}
			pw.Decode = fmt.Sprintf("%s via HWND (black=%v)", detail, mostlyBlack(img))
			if pw.ImagePath != "" {
				pw.Decode += " img=" + pw.ImagePath
			}
			pw.Decode += " — leave strip visible in-game"
		} else {
			pw.Decode = fmt.Sprintf("ok via HWND id=%d bytes=%d", msg.ID, len(msg.Payload))
			pw.PayloadPreview = truncateStr(string(msg.Payload), 120)
		}
		res.Windows = append(res.Windows, pw)
	}
	return res
}
