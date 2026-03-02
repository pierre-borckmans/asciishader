package main

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// RegionSelector handles the interactive region selection overlay.
type RegionSelector struct {
	X, Y   int // top-left corner in viewport coordinates
	W, H   int // dimensions
	MaxW   int // viewport width limit
	MaxH   int // viewport height limit
	Active bool

	// Drag state
	dragMode   regionDragMode
	dragStartX int
	dragStartY int
	dragOrigX  int
	dragOrigY  int
	dragOrigW  int
	dragOrigH  int
}

type regionDragMode int

const (
	dragNone regionDragMode = iota
	dragMove
	dragResizeNW
	dragResizeNE
	dragResizeSW
	dragResizeSE
	dragResizeN
	dragResizeS
	dragResizeW
	dragResizeE
)

// NewRegionSelector creates a region selector centered in the viewport.
func NewRegionSelector(vpW, vpH int) *RegionSelector {
	w, h := 40, 20
	if w > vpW {
		w = vpW
	}
	if h > vpH {
		h = vpH
	}
	return &RegionSelector{
		X:    (vpW - w) / 2,
		Y:    (vpH - h) / 2,
		W:    w,
		H:    h,
		MaxW: vpW,
		MaxH: vpH,
	}
}

// SetPreset applies a preset region size.
func (rs *RegionSelector) SetPreset(preset int, vpW, vpH int) {
	switch preset {
	case 1:
		rs.W, rs.H = 20, 10
	case 2:
		rs.W, rs.H = 40, 20
	case 3:
		rs.W, rs.H = 60, 30
	case 4:
		rs.W, rs.H = vpW, vpH
	}
	if rs.W > vpW {
		rs.W = vpW
	}
	if rs.H > vpH {
		rs.H = vpH
	}
	// Re-center
	rs.X = (vpW - rs.W) / 2
	rs.Y = (vpH - rs.H) / 2
	rs.clamp()
}

// UpdateViewportSize updates the max bounds when viewport resizes.
func (rs *RegionSelector) UpdateViewportSize(vpW, vpH int) {
	rs.MaxW = vpW
	rs.MaxH = vpH
	rs.clamp()
}

func (rs *RegionSelector) clamp() {
	if rs.W < 4 {
		rs.W = 4
	}
	if rs.H < 4 {
		rs.H = 4
	}
	if rs.W > rs.MaxW {
		rs.W = rs.MaxW
	}
	if rs.H > rs.MaxH {
		rs.H = rs.MaxH
	}
	if rs.X < 0 {
		rs.X = 0
	}
	if rs.Y < 0 {
		rs.Y = 0
	}
	if rs.X+rs.W > rs.MaxW {
		rs.X = rs.MaxW - rs.W
	}
	if rs.Y+rs.H > rs.MaxH {
		rs.Y = rs.MaxH - rs.H
	}
}

// HandleMousePress processes a mouse press in viewport-relative coordinates.
// Returns true if the event was consumed.
func (rs *RegionSelector) HandleMousePress(x, y int) bool {
	// Check corners (3x2 hit area)
	cornerW, cornerH := 3, 2
	inLeft := x >= rs.X && x < rs.X+cornerW
	inRight := x >= rs.X+rs.W-cornerW && x < rs.X+rs.W
	inTop := y >= rs.Y && y < rs.Y+cornerH
	inBottom := y >= rs.Y+rs.H-cornerH && y < rs.Y+rs.H

	rs.dragStartX = x
	rs.dragStartY = y
	rs.dragOrigX = rs.X
	rs.dragOrigY = rs.Y
	rs.dragOrigW = rs.W
	rs.dragOrigH = rs.H

	switch {
	case inLeft && inTop:
		rs.dragMode = dragResizeNW
	case inRight && inTop:
		rs.dragMode = dragResizeNE
	case inLeft && inBottom:
		rs.dragMode = dragResizeSW
	case inRight && inBottom:
		rs.dragMode = dragResizeSE
	case inTop && x >= rs.X && x < rs.X+rs.W:
		rs.dragMode = dragResizeN
	case inBottom && x >= rs.X && x < rs.X+rs.W:
		rs.dragMode = dragResizeS
	case inLeft && y >= rs.Y && y < rs.Y+rs.H:
		rs.dragMode = dragResizeW
	case inRight && y >= rs.Y && y < rs.Y+rs.H:
		rs.dragMode = dragResizeE
	case x >= rs.X && x < rs.X+rs.W && y >= rs.Y && y < rs.Y+rs.H:
		rs.dragMode = dragMove
	default:
		rs.dragMode = dragNone
		return false
	}
	return true
}

// HandleMouseDrag processes a mouse drag in viewport-relative coordinates.
func (rs *RegionSelector) HandleMouseDrag(x, y int) {
	dx := x - rs.dragStartX
	dy := y - rs.dragStartY

	switch rs.dragMode {
	case dragMove:
		rs.X = rs.dragOrigX + dx
		rs.Y = rs.dragOrigY + dy
	case dragResizeNW:
		rs.X = rs.dragOrigX + dx
		rs.Y = rs.dragOrigY + dy
		rs.W = rs.dragOrigW - dx
		rs.H = rs.dragOrigH - dy
	case dragResizeNE:
		rs.Y = rs.dragOrigY + dy
		rs.W = rs.dragOrigW + dx
		rs.H = rs.dragOrigH - dy
	case dragResizeSW:
		rs.X = rs.dragOrigX + dx
		rs.W = rs.dragOrigW - dx
		rs.H = rs.dragOrigH + dy
	case dragResizeSE:
		rs.W = rs.dragOrigW + dx
		rs.H = rs.dragOrigH + dy
	case dragResizeN:
		rs.Y = rs.dragOrigY + dy
		rs.H = rs.dragOrigH - dy
	case dragResizeS:
		rs.H = rs.dragOrigH + dy
	case dragResizeW:
		rs.X = rs.dragOrigX + dx
		rs.W = rs.dragOrigW - dx
	case dragResizeE:
		rs.W = rs.dragOrigW + dx
	}
	rs.clamp()
}

// HandleMouseRelease ends any drag.
func (rs *RegionSelector) HandleMouseRelease() {
	rs.dragMode = dragNone
}

// RenderOverlay renders the region selection overlay on top of the viewport frame lines.
// vpLines should be the split viewport lines (before joining). Returns modified lines.
func (rs *RegionSelector) RenderOverlay(vpLines []string) []string {
	if len(vpLines) == 0 {
		return vpLines
	}

	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6600")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#666666"))

	infoStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6600")).
		Bold(true)

	// Draw dim overlay outside the region by replacing chars
	result := make([]string, len(vpLines))
	for y, line := range vpLines {
		runes := []rune(line)
		_ = runes // We'll work with the raw ANSI string

		// For simplicity, overlay the border characters directly
		// We render a new line with the overlay applied
		if y < rs.Y || y >= rs.Y+rs.H {
			// Outside region vertically — dim the line
			result[y] = dimStyle.Render(stripANSI(line))
		} else {
			result[y] = line
		}
	}

	// Draw border
	for y := rs.Y; y < rs.Y+rs.H && y < len(result); y++ {
		if y == rs.Y {
			// Top border
			top := "+" + strings.Repeat("-", rs.W-2) + "+"
			// Add size info in the middle
			info := fmt.Sprintf(" %dx%d ", rs.W, rs.H)
			if len(info) < rs.W-2 {
				mid := (rs.W - 2 - len(info)) / 2
				top = "+" + strings.Repeat("-", mid) + info + strings.Repeat("-", rs.W-2-mid-len(info)) + "+"
			}
			result[y] = padToCol(result[y], rs.X, borderStyle.Render(top))
		} else if y == rs.Y+rs.H-1 {
			// Bottom border
			hint := " Enter:record  1-4:preset  Esc:cancel "
			bottom := "+" + strings.Repeat("-", rs.W-2) + "+"
			if len(hint) < rs.W-2 {
				mid := (rs.W - 2 - len(hint)) / 2
				bottom = "+" + strings.Repeat("-", mid) + hint + strings.Repeat("-", rs.W-2-mid-len(hint)) + "+"
			}
			result[y] = padToCol(result[y], rs.X, infoStyle.Render(bottom))
		} else {
			// Side borders only
			leftBorder := borderStyle.Render("|")
			rightBorder := borderStyle.Render("|")
			result[y] = padToCol(result[y], rs.X, leftBorder)
			result[y] = padToCol(result[y], rs.X+rs.W-1, rightBorder)
		}
	}

	return result
}

// padToCol overwrites content at column col in the line.
func padToCol(line string, col int, content string) string {
	// Strip ANSI to count visible width
	vis := stripANSI(line)
	visRunes := []rune(vis)

	// Ensure line is wide enough
	for len(visRunes) <= col+lipgloss.Width(content) {
		visRunes = append(visRunes, ' ')
	}

	// Build: prefix (plain) + content + suffix (plain)
	prefix := string(visRunes[:col])
	contentWidth := lipgloss.Width(content)
	suffixStart := col + contentWidth
	if suffixStart > len(visRunes) {
		suffixStart = len(visRunes)
	}
	suffix := string(visRunes[suffixStart:])

	return prefix + content + suffix
}

// stripANSI is defined in scrollable_view.go
