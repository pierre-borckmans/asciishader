package components

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// Color picker action constants returned by HandleKey.
const (
	ColorPickerContinue = iota
	ColorPickerConfirm
	ColorPickerCancel
)

// Focus sections within the picker.
const (
	cpFocusSV  = 0
	cpFocusHue = 1
	cpFocusHex = 2
)

// Grid dimensions.
const (
	cpGridW    = 20 // saturation steps (columns)
	cpGridRows = 10 // value steps (rows); looks ~square with 2:1 terminal chars
)

// Drag regions.
const (
	cpDragNone = 0
	cpDragSV   = 1
	cpDragHue  = 2
)

// ColorPicker is a floating HSV color editor.
//
// Layout:
//
//	╭─ Color ──────────────────╮
//	│ ████████████████████ █   │   SV grid + vertical hue strip
//	│ ████████████████████ █◀  │   ◀ marks current hue
//	│ ████████████████████ █   │
//	│ ...                      │
//	│ ██ #1a1a2e               │   swatch + hex input
//	╰──────────────────────────╯
type ColorPicker struct {
	H, S, V float64 // H: 0–360, S: 0–1, V: 0–1

	focus int // cpFocusSV, cpFocusHue, cpFocusHex

	// SV grid cursor (character-cell coordinates)
	svX int // 0..cpGridW-1  (left=S0 → right=S1)
	svY int // 0..cpGridRows-1  (top=V1 → bottom=V0)

	// Hue strip cursor
	hueY int // 0..cpGridRows-1

	// Hex input
	hexBuf []rune
	hexPos int

	// Mouse drag state
	dragging int // cpDragNone, cpDragSV, cpDragHue
}

// NewColorPicker creates a floating color picker initialized to the given RGB.
func NewColorPicker(r, g, b uint8) *ColorPicker {
	h, s, v := rgbToHSV(r, g, b)
	cp := &ColorPicker{H: h, S: s, V: v}
	cp.svX = clampInt(int(math.Round(s*float64(cpGridW-1))), 0, cpGridW-1)
	cp.svY = clampInt(int(math.Round((1-v)*float64(cpGridRows-1))), 0, cpGridRows-1)
	cp.hueY = clampInt(int(math.Round(h/360.0*float64(cpGridRows-1))), 0, cpGridRows-1)
	cp.syncHex()
	return cp
}

// Width returns the panel width including borders.
func (cp *ColorPicker) Width() int {
	// │ (pad) grid (gap) hue marker (pad) │
	return 2 + cpGridW + 1 + 1 + 1 + 2
}

// Height returns the panel height including borders.
func (cp *ColorPicker) Height() int {
	// top border + grid rows + hex row + bottom border
	return 1 + cpGridRows + 1 + 1
}

// RGB returns the current color as 8-bit RGB.
func (cp *ColorPicker) RGB() (uint8, uint8, uint8) {
	return hsvToRGB(cp.H, cp.S, cp.V)
}

// HexString returns the current color as "#rrggbb".
func (cp *ColorPicker) HexString() string {
	r, g, b := cp.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// HandleKey processes a key press and returns an action constant.
func (cp *ColorPicker) HandleKey(key string) int {
	switch key {
	case "enter":
		return ColorPickerConfirm
	case "esc":
		return ColorPickerCancel
	case "tab":
		cp.focus = (cp.focus + 1) % 3
		return ColorPickerContinue
	case "shift+tab":
		cp.focus = (cp.focus + 2) % 3
		return ColorPickerContinue
	}

	switch cp.focus {
	case cpFocusSV:
		cp.handleSVKey(key)
	case cpFocusHue:
		cp.handleHueKey(key)
	case cpFocusHex:
		cp.handleHexKey(key)
	}
	return ColorPickerContinue
}

// ColorPickerMouseMiss indicates the mouse event was outside the picker.
const ColorPickerMouseMiss = -1

// HandleMouse processes a mouse event given the picker's top-left screen position.
// Returns ColorPickerContinue if handled inside, ColorPickerMouseMiss if outside.
func (cp *ColorPicker) HandleMouse(msg tea.MouseMsg, pickerX, pickerY int) int {
	mouse := msg.Mouse()
	relX := mouse.X - pickerX
	relY := mouse.Y - pickerY

	// During drag, track motion even outside bounds
	if cp.dragging != cpDragNone {
		switch msg.(type) {
		case tea.MouseMotionMsg:
			cp.applyDrag(relX, relY)
			return ColorPickerContinue
		case tea.MouseReleaseMsg:
			cp.applyDrag(relX, relY)
			cp.dragging = cpDragNone
			return ColorPickerContinue
		}
	}

	// Outside picker bounds
	if relX < 0 || relX >= cp.Width() || relY < 0 || relY >= cp.Height() {
		return ColorPickerMouseMiss
	}

	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return ColorPickerContinue
		}
		// Grid area: rows 1..cpGridRows
		if relY >= 1 && relY <= cpGridRows {
			gridRow := relY - 1
			// SV grid: cols 2..cpGridW+1
			if relX >= 2 && relX < 2+cpGridW {
				cp.svX = relX - 2
				cp.svY = gridRow
				cp.focus = cpFocusSV
				cp.dragging = cpDragSV
				cp.syncFromCursor()
				return ColorPickerContinue
			}
			// Hue strip: col 2+cpGridW+1 = 23
			if relX == 2+cpGridW+1 {
				cp.hueY = gridRow
				cp.focus = cpFocusHue
				cp.dragging = cpDragHue
				cp.syncFromCursor()
				return ColorPickerContinue
			}
		}
		// Hex row
		if relY == 1+cpGridRows {
			cp.focus = cpFocusHex
			return ColorPickerContinue
		}

	case tea.MouseWheelMsg:
		// Scroll wheel on hue strip
		if mouse.Button == tea.MouseWheelUp && cp.hueY > 0 {
			cp.hueY--
			cp.syncFromCursor()
		} else if mouse.Button == tea.MouseWheelDown && cp.hueY < cpGridRows-1 {
			cp.hueY++
			cp.syncFromCursor()
		}
		return ColorPickerContinue
	}

	return ColorPickerContinue
}

// applyDrag updates the cursor from a drag position, clamping to grid bounds.
func (cp *ColorPicker) applyDrag(relX, relY int) {
	gridRow := clampInt(relY-1, 0, cpGridRows-1)
	switch cp.dragging {
	case cpDragSV:
		cp.svX = clampInt(relX-2, 0, cpGridW-1)
		cp.svY = gridRow
		cp.focus = cpFocusSV
		cp.syncFromCursor()
	case cpDragHue:
		cp.hueY = gridRow
		cp.focus = cpFocusHue
		cp.syncFromCursor()
	}
}

func (cp *ColorPicker) handleSVKey(key string) {
	switch key {
	case "left", "h":
		if cp.svX > 0 {
			cp.svX--
		}
	case "right", "l":
		if cp.svX < cpGridW-1 {
			cp.svX++
		}
	case "up", "k":
		if cp.svY > 0 {
			cp.svY--
		}
	case "down", "j":
		if cp.svY < cpGridRows-1 {
			cp.svY++
		}
	case "shift+left", "H":
		cp.svX = max(cp.svX-4, 0)
	case "shift+right", "L":
		cp.svX = min(cp.svX+4, cpGridW-1)
	case "shift+up", "K":
		cp.svY = max(cp.svY-4, 0)
	case "shift+down", "J":
		cp.svY = min(cp.svY+4, cpGridRows-1)
	default:
		return
	}
	cp.syncFromCursor()
}

func (cp *ColorPicker) handleHueKey(key string) {
	switch key {
	case "up", "k":
		if cp.hueY > 0 {
			cp.hueY--
		}
	case "down", "j":
		if cp.hueY < cpGridRows-1 {
			cp.hueY++
		}
	case "shift+up", "K":
		cp.hueY = max(cp.hueY-4, 0)
	case "shift+down", "J":
		cp.hueY = min(cp.hueY+4, cpGridRows-1)
	default:
		return
	}
	cp.syncFromCursor()
}

func (cp *ColorPicker) handleHexKey(key string) {
	switch key {
	case "left":
		if cp.hexPos > 0 {
			cp.hexPos--
		}
	case "right":
		if cp.hexPos < len(cp.hexBuf) {
			cp.hexPos++
		}
	case "home", "ctrl+a":
		cp.hexPos = 0
	case "end", "ctrl+e":
		cp.hexPos = len(cp.hexBuf)
	case "backspace":
		if cp.hexPos > 0 {
			cp.hexBuf = append(cp.hexBuf[:cp.hexPos-1], cp.hexBuf[cp.hexPos:]...)
			cp.hexPos--
			cp.tryParseHex()
		}
	case "delete":
		if cp.hexPos < len(cp.hexBuf) {
			cp.hexBuf = append(cp.hexBuf[:cp.hexPos], cp.hexBuf[cp.hexPos+1:]...)
			cp.tryParseHex()
		}
	default:
		if len(key) == 1 && key[0] >= ' ' && key[0] <= '~' {
			cp.hexBuf = append(cp.hexBuf[:cp.hexPos], append([]rune{rune(key[0])}, cp.hexBuf[cp.hexPos:]...)...)
			cp.hexPos++
			cp.tryParseHex()
		}
	}
}

// ── internal state sync ─────────────────────────────────────────────

func (cp *ColorPicker) syncFromCursor() {
	cp.S = float64(cp.svX) / float64(cpGridW-1)
	cp.V = 1.0 - float64(cp.svY)/float64(cpGridRows-1)
	cp.H = float64(cp.hueY) / float64(cpGridRows-1) * 360.0
	if cp.H >= 360 {
		cp.H = 359.99
	}
	cp.syncHex()
}

func (cp *ColorPicker) syncHex() {
	cp.hexBuf = []rune(cp.HexString())
	cp.hexPos = len(cp.hexBuf)
}

func (cp *ColorPicker) tryParseHex() {
	hex := string(cp.hexBuf)
	if len(hex) == 7 && hex[0] == '#' {
		var r, g, b uint8
		if n, err := fmt.Sscanf(hex, "#%02x%02x%02x", &r, &g, &b); err == nil && n == 3 {
			h, s, v := rgbToHSV(r, g, b)
			cp.H, cp.S, cp.V = h, s, v
			cp.svX = clampInt(int(math.Round(s*float64(cpGridW-1))), 0, cpGridW-1)
			cp.svY = clampInt(int(math.Round((1-v)*float64(cpGridRows-1))), 0, cpGridRows-1)
			cp.hueY = clampInt(int(math.Round(h/360.0*float64(cpGridRows-1))), 0, cpGridRows-1)
		}
	}
}

// ── rendering ───────────────────────────────────────────────────────

// Render returns the complete floating panel as a multi-line string.
func (cp *ColorPicker) Render() string {
	innerW := cp.Width() - 2 // inside border columns
	var b strings.Builder

	// ── top border ──
	b.WriteString("\x1b[38;5;245m╭─\x1b[0m\x1b[38;5;252m Color \x1b[0m\x1b[38;5;245m")
	b.WriteString(strings.Repeat("─", innerW-8))
	b.WriteString("╮\x1b[0m")

	// ── grid rows ──
	for row := 0; row < cpGridRows; row++ {
		b.WriteByte('\n')
		b.WriteString("\x1b[38;5;245m│\x1b[0m ")

		// SV grid cells
		for col := 0; col < cpGridW; col++ {
			s := float64(col) / float64(cpGridW-1)
			v := 1.0 - float64(row)/float64(cpGridRows-1)
			cr, cg, cb := hsvToRGB(cp.H, s, v)

			if col == cp.svX && row == cp.svY {
				// Cursor — contrasting dot on cell background
				fr, fg, fb := contrastBW(cr, cg, cb)
				if cp.focus == cpFocusSV {
					fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm◆\x1b[0m", cr, cg, cb, fr, fg, fb)
				} else {
					fmt.Fprintf(&b, "\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm◇\x1b[0m", cr, cg, cb, fr, fg, fb)
				}
			} else {
				fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm█\x1b[0m", cr, cg, cb)
			}
		}

		// Gap + hue strip
		b.WriteByte(' ')
		hue := float64(row) / float64(cpGridRows-1) * 360.0
		hr, hg, hb := hsvToRGB(hue, 1.0, 1.0)
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm█\x1b[0m", hr, hg, hb)

		// Hue marker
		if row == cp.hueY {
			if cp.focus == cpFocusHue {
				b.WriteString("\x1b[97m◀\x1b[0m")
			} else {
				b.WriteString("\x1b[38;5;245m◂\x1b[0m")
			}
		} else {
			b.WriteByte(' ')
		}

		// Right padding + border
		b.WriteString(" \x1b[38;5;245m│\x1b[0m")
	}

	// ── hex row ──
	b.WriteByte('\n')
	b.WriteString("\x1b[38;5;245m│\x1b[0m ")
	r, g, bv := cp.RGB()
	fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm██\x1b[0m ", r, g, bv)

	hexContentW := 4 // " ██ " visible chars already written (pad + swatch + gap)
	if cp.focus == cpFocusHex {
		// Editable hex with cursor
		b.WriteString("\x1b[93m")
		pos := cp.hexPos
		if pos > len(cp.hexBuf) {
			pos = len(cp.hexBuf)
		}
		b.WriteString(string(cp.hexBuf[:pos]))
		b.WriteString("\x1b[7m")
		if pos < len(cp.hexBuf) {
			b.WriteRune(cp.hexBuf[pos])
		} else {
			b.WriteByte(' ')
		}
		b.WriteString("\x1b[27m")
		if pos < len(cp.hexBuf) {
			b.WriteString(string(cp.hexBuf[pos+1:]))
		}
		b.WriteString("\x1b[0m")
		hexContentW += len(cp.hexBuf)
		if pos >= len(cp.hexBuf) {
			hexContentW++ // trailing cursor space
		}
	} else {
		fmt.Fprintf(&b, "\x1b[38;5;252m%s\x1b[0m", string(cp.hexBuf))
		hexContentW += len(cp.hexBuf)
	}

	// Pad hex row to inner width
	pad := innerW - hexContentW
	if pad > 0 {
		b.WriteString(strings.Repeat(" ", pad))
	}
	b.WriteString("\x1b[38;5;245m│\x1b[0m")

	// ── bottom border ──
	b.WriteByte('\n')
	b.WriteString("\x1b[38;5;245m╰")
	b.WriteString(strings.Repeat("─", innerW))
	b.WriteString("╯\x1b[0m")

	return b.String()
}

// ── HSV ↔ RGB ───────────────────────────────────────────────────────

func hsvToRGB(h, s, v float64) (uint8, uint8, uint8) {
	if s == 0 {
		c := uint8(math.Round(v * 255))
		return c, c, c
	}
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	h /= 60
	i := int(h)
	f := h - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))

	var r, g, b float64
	switch i {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return uint8(math.Round(r * 255)), uint8(math.Round(g * 255)), uint8(math.Round(b * 255))
}

func rgbToHSV(r, g, b uint8) (float64, float64, float64) {
	rf := float64(r) / 255
	gf := float64(g) / 255
	bf := float64(b) / 255

	maxC := math.Max(rf, math.Max(gf, bf))
	minC := math.Min(rf, math.Min(gf, bf))
	delta := maxC - minC

	var h float64
	switch {
	case delta == 0:
		h = 0
	case maxC == rf:
		h = 60 * math.Mod((gf-bf)/delta, 6)
	case maxC == gf:
		h = 60 * ((bf-rf)/delta + 2)
	default:
		h = 60 * ((rf-gf)/delta + 4)
	}
	if h < 0 {
		h += 360
	}

	var s float64
	if maxC != 0 {
		s = delta / maxC
	}

	return h, s, maxC
}

func contrastBW(r, g, b uint8) (uint8, uint8, uint8) {
	lum := 0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)
	if lum > 128 {
		return 0, 0, 0
	}
	return 255, 255, 255
}
