package recorder

import (
	"fmt"
	"strings"

	"asciishader/tui/components"

	"github.com/charmbracelet/lipgloss"
)

// RegionSelector handles the interactive region selection overlay.
type RegionSelector struct {
	X, Y      int // top-left corner in viewport coordinates
	W, H      int // dimensions
	MaxW      int // viewport width limit
	MaxH      int // viewport height limit
	Active    bool
	Recording bool   // true when showing overlay during live recording
	RecLabel  string // e.g. "● REC 2.3s" — shown in top border while recording

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
	}
	rs.clamp()
}

// IsDragging returns whether a region drag is in progress.
func (rs *RegionSelector) IsDragging() bool {
	return rs.dragMode != dragNone
}

// HandleMouseRelease ends any drag.
func (rs *RegionSelector) HandleMouseRelease() {
	rs.dragMode = dragNone
}

// RenderOverlay renders the region selection overlay on top of the viewport frame lines.
// vpLines should be the split viewport lines (before joining). Returns modified lines.
// Content inside the selection preserves ANSI colors; everything outside is dimmed.
func (rs *RegionSelector) RenderOverlay(vpLines []string) []string {
	if len(vpLines) == 0 {
		return vpLines
	}

	borderStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FF6600")).
		Bold(true)

	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("#555555"))

	result := make([]string, len(vpLines))

	for y, line := range vpLines {
		vis := []rune(components.StripANSI(line))
		// Pad visible chars to cover the selection area
		for len(vis) < rs.X+rs.W+1 {
			vis = append(vis, ' ')
		}

		if y < rs.Y || y >= rs.Y+rs.H {
			// Fully outside the selection — dim everything
			result[y] = dimStyle.Render(string(vis))
			continue
		}

		// Left and right dim portions (from stripped visible text)
		left := dimStyle.Render(string(vis[:rs.X]))
		rightStart := rs.X + rs.W
		if rightStart > len(vis) {
			rightStart = len(vis)
		}
		right := dimStyle.Render(string(vis[rightStart:]))

		innerW := rs.W - 2
		if innerW < 0 {
			innerW = 0
		}

		if y == rs.Y {
			// Top border with size info or recording label
			info := fmt.Sprintf(" %dx%d ", rs.W, rs.H)
			if rs.Recording && rs.RecLabel != "" {
				info = " " + rs.RecLabel + " "
			}
			infoW := len([]rune(info))
			var border string
			if infoW <= innerW {
				ld := (innerW - infoW) / 2
				rd := innerW - ld - infoW
				border = "╭" + strings.Repeat("─", ld) + info + strings.Repeat("─", rd) + "╮"
			} else {
				border = "╭" + strings.Repeat("─", innerW) + "╮"
			}
			result[y] = left + borderStyle.Render(border) + right
		} else if y == rs.Y+rs.H-1 {
			// Bottom border with hints
			hint := " Enter:rec  1-4:preset  Esc:cancel "
			if rs.Recording {
				hint = " o:stop recording "
			}
			hintW := len([]rune(hint))
			var border string
			if hintW <= innerW {
				ld := (innerW - hintW) / 2
				rd := innerW - ld - hintW
				border = "╰" + strings.Repeat("─", ld) + hint + strings.Repeat("─", rd) + "╯"
			} else {
				border = "╰" + strings.Repeat("─", innerW) + "╯"
			}
			result[y] = left + borderStyle.Render(border) + right
		} else {
			// Side borders — preserve original ANSI colors inside
			mid := ansiCut(line, rs.X+1, rs.X+rs.W-1)
			result[y] = left + borderStyle.Render("│") + mid + borderStyle.Render("│") + right
		}
	}

	return result
}

// ansiCut extracts visible columns [start, end) from an ANSI string,
// preserving ANSI escape sequences that affect those columns.
func ansiCut(s string, start, end int) string {
	var buf strings.Builder
	col := 0
	runes := []rune(s)
	i := 0

	for i < len(runes) {
		if runes[i] == '\x1b' && i+1 < len(runes) && runes[i+1] == '[' {
			// ANSI escape sequence — collect until terminator letter
			j := i + 2
			for j < len(runes) {
				if (runes[j] >= 'A' && runes[j] <= 'Z') || (runes[j] >= 'a' && runes[j] <= 'z') {
					j++
					break
				}
				j++
			}
			// Include escape sequences that precede or fall within the range
			if col < end {
				for k := i; k < j; k++ {
					buf.WriteRune(runes[k])
				}
			}
			i = j
		} else {
			if col >= start && col < end {
				buf.WriteRune(runes[i])
			}
			col++
			i++
		}
	}
	buf.WriteString("\033[0m")
	return buf.String()
}

