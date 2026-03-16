package gpu

import (
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"strings"

	"asciishader/pkg/core"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

// RenderImageFrame renders at full pixel resolution and returns a Kitty graphics
// escape sequence that positions and displays the image directly.
// viewRow and viewCol are 1-indexed terminal coordinates of the viewport origin.
func (g *GPURenderer) RenderImageFrame(r *core.RenderConfig, viewRow, viewCol int) string {
	if r.CellPixelW <= 0 || r.CellPixelH <= 0 {
		return ""
	}

	// Render at scaled pixel resolution
	scale := r.ImageScale
	if scale <= 0 {
		scale = 0.5
	}
	subW := max(int(float64(r.CellPixelW)*scale+0.5), 1)
	subH := max(int(float64(r.CellPixelH)*scale+0.5), 1)
	g.renderPass(r, subW, subH)

	// Extract RGB bytes directly from the RGBA pixel buffer (skip alpha).
	npx := g.pixW * g.pixH
	rgbSize := npx * 3
	if cap(g.imgRGB) < rgbSize {
		g.imgRGB = make([]byte, rgbSize)
	}
	g.imgRGB = g.imgRGB[:rgbSize]
	j := 0
	for i := 0; i < npx*4; i += 4 {
		g.imgRGB[j] = g.pixels[i]
		g.imgRGB[j+1] = g.pixels[i+1]
		g.imgRGB[j+2] = g.pixels[i+2]
		j += 3
	}

	// Zlib compress with BestSpeed — LZ77 matching compresses image data well
	// enough to avoid saturating terminal I/O, while being the fastest level
	// that does meaningful compression.
	g.imgZBuf.Reset()
	if g.imgZlib == nil {
		g.imgZlib, _ = zlib.NewWriterLevel(&g.imgZBuf, zlib.BestSpeed)
	} else {
		g.imgZlib.Reset(&g.imgZBuf)
	}
	_, _ = g.imgZlib.Write(g.imgRGB)
	_ = g.imgZlib.Close()

	// Base64 encode
	g.imgB64.Reset()
	b64 := base64.NewEncoder(base64.StdEncoding, &g.imgB64)
	_, _ = g.imgZBuf.WriteTo(b64)
	_ = b64.Close()

	// Detect whether we need a new placement (first frame or layout changed)
	needPlace := !g.imgPlaced ||
		g.imgLastRow != viewRow || g.imgLastCol != viewCol ||
		g.imgLastW != r.Width || g.imgLastH != r.Height

	// Build APC escape with chunking
	var out strings.Builder
	out.Grow(g.imgB64.Len() + 512)

	if needPlace {
		// Delete old placement, position cursor, transmit + put (a=T)
		if g.imgPlaced {
			out.WriteString(ansi.KittyGraphics(nil, "a=d", "d=i", "i=1", "q=2"))
		}
		fmt.Fprintf(&out, "\033[%d;%dH", viewRow, viewCol)
		g.imgPlaced = true
		g.imgLastRow = viewRow
		g.imgLastCol = viewCol
		g.imgLastW = r.Width
		g.imgLastH = r.Height
	}

	// First chunk carries all options.
	// a=T (transmit+put) when we need a new placement, a=t (transmit only)
	// when the placement already exists — existing placements auto-update.
	action := 't'
	if needPlace {
		action = 'T'
	}
	firstOpts := fmt.Sprintf("f=24,s=%d,v=%d,i=1,a=%c,C=1,o=z,q=2,c=%d,r=%d",
		g.pixW, g.pixH, action, r.Width, r.Height)

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

	return out.String()
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
	g.imgPlaced = false
	return ansi.KittyGraphics(nil, "a=d", "d=a", "q=2")
}
