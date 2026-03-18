package gpu

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"asciishader/pkg/core"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
	"github.com/klauspost/compress/zlib"
)

// ImageTiming holds per-frame timing breakdown for image mode.
type ImageTiming struct {
	GPU    time.Duration
	Zlib   time.Duration
	Base64 time.Duration
}

// imgEncResult is the output of the background encode goroutine.
type imgEncResult struct {
	transmit string
	zlib     time.Duration
	b64      time.Duration
}

// RenderImageFrame renders at full pixel resolution and returns a Kitty graphics
// escape sequence. GPU rendering and compression are pipelined: the returned
// string is from the PREVIOUS frame (one frame of latency), while the current
// frame's compression runs in the background.
func (g *GPURenderer) RenderImageFrame(r *core.RenderConfig, viewRow, viewCol int) (string, ImageTiming) {
	var timing ImageTiming
	if r.CellPixelW <= 0 || r.CellPixelH <= 0 {
		return "", timing
	}

	// --- GPU stage (main thread) ---
	scale := r.ImageScale
	if scale <= 0 {
		scale = 0.5
	}
	subW := max(int(float64(r.CellPixelW)*scale+0.5), 1)
	subH := max(int(float64(r.CellPixelH)*scale+0.5), 1)

	t0 := time.Now()
	g.renderPass(r, subW, subH)
	timing.GPU = time.Since(t0)

	// Extract RGB into the current double-buffer slot
	buf := g.imgBufIdx
	g.imgBufIdx ^= 1

	npx := g.pixW * g.pixH
	rgbSize := npx * 3
	if cap(g.imgRGBBufs[buf]) < rgbSize {
		g.imgRGBBufs[buf] = make([]byte, rgbSize)
	}
	rgb := g.imgRGBBufs[buf][:rgbSize]
	j := 0
	for i := 0; i < npx*4; i += 4 {
		rgb[j] = g.pixels[i]
		rgb[j+1] = g.pixels[i+1]
		rgb[j+2] = g.pixels[i+2]
		j += 3
	}

	// Determine placement (main thread only)
	needPlace := !g.imgPlaced ||
		g.imgLastRow != viewRow || g.imgLastCol != viewCol ||
		g.imgLastW != r.Width || g.imgLastH != r.Height

	action := byte('t')
	cursorSeq := ""
	if needPlace {
		action = 'T'
		if g.imgPlaced {
			cursorSeq = ansi.KittyGraphics(nil, "a=d", "d=i", "i=1", "q=2")
		}
		cursorSeq += fmt.Sprintf("\033[%d;%dH", viewRow, viewCol)
		g.imgPlaced = true
		g.imgLastRow = viewRow
		g.imgLastCol = viewCol
		g.imgLastW = r.Width
		g.imgLastH = r.Height
	}

	// --- Wait for previous encode ---
	var result string
	if g.imgEncPending {
		prev := <-g.imgEncOut
		result = prev.transmit
		timing.Zlib = prev.zlib
		timing.Base64 = prev.b64
	}

	// --- Start new encode in background ---
	g.imgEncPending = true
	pixW, pixH := g.pixW, g.pixH
	width, height := r.Width, r.Height
	go g.encodeImage(rgb, pixW, pixH, width, height, action, cursorSeq)

	return result, timing
}

// encodeImage compresses and builds the Kitty APC escape. Runs on a background
// goroutine. Sends result on g.imgEncOut when done.
func (g *GPURenderer) encodeImage(rgb []byte, pixW, pixH, cols, rows int, action byte, cursorSeq string) {
	var res imgEncResult

	// Zlib compress
	t1 := time.Now()
	g.imgZBuf.Reset()
	if g.imgZlib == nil {
		g.imgZlib, _ = zlib.NewWriterLevel(&g.imgZBuf, zlib.BestSpeed)
	} else {
		g.imgZlib.Reset(&g.imgZBuf)
	}
	_, _ = g.imgZlib.Write(rgb)
	_ = g.imgZlib.Close()
	res.zlib = time.Since(t1)

	// Base64 encode
	t2 := time.Now()
	g.imgB64.Reset()
	b64 := base64.NewEncoder(base64.StdEncoding, &g.imgB64)
	_, _ = g.imgZBuf.WriteTo(b64)
	_ = b64.Close()

	// Build APC escape with chunking
	var out strings.Builder
	out.Grow(g.imgB64.Len() + 512)
	out.WriteString(cursorSeq)

	firstOpts := fmt.Sprintf("f=24,s=%d,v=%d,i=1,a=%c,C=1,o=z,q=2,c=%d,r=%d",
		pixW, pixH, action, cols, rows)

	payload := g.imgB64.Bytes()
	first := true
	for len(payload) > 0 {
		chunk := payload
		if len(chunk) > kitty.MaxChunkSize {
			chunk = payload[:kitty.MaxChunkSize]
		}
		payload = payload[len(chunk):]
		more := len(payload) > 0

		if first {
			if more {
				out.WriteString(ansi.KittyGraphics(chunk, firstOpts, "m=1"))
			} else {
				out.WriteString(ansi.KittyGraphics(chunk, firstOpts))
			}
			first = false
		} else if more {
			out.WriteString(ansi.KittyGraphics(chunk, "q=2", "m=1"))
		} else {
			out.WriteString(ansi.KittyGraphics(chunk, "q=2", "m=0"))
		}
	}

	res.transmit = out.String()
	res.b64 = time.Since(t2)
	g.imgEncOut <- res
}

// BlankFrame returns a string of spaces sized to fill the viewport.
func BlankFrame(cols, rows int) string {
	line := strings.Repeat(" ", cols)
	lines := make([]string, rows)
	for i := range lines {
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

// CleanupImage deletes any lingering Kitty images and resets placement state.
// Returns a Kitty delete-all escape sequence.
func (g *GPURenderer) CleanupImage() string {
	// Drain any pending encode
	if g.imgEncPending {
		<-g.imgEncOut
		g.imgEncPending = false
	}
	g.imgPlaced = false
	return ansi.KittyGraphics(nil, "a=d", "d=a", "q=2")
}
