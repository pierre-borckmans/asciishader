package components

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"asciishader/tui/styles"
)

// TooltipPlacement controls where a tooltip appears relative to its zone.
type TooltipPlacement int

const (
	TooltipBelow TooltipPlacement = iota
	TooltipRight
	TooltipAbove
)

// ZonePosition holds the screen coordinates of a zone.
type ZonePosition struct {
	StartX, StartY, EndX, EndY int
}

// Tooltip represents a floating label to overlay on the rendered output.
type Tooltip struct {
	Text    string      // Display text
	Row     int         // 0-indexed row where the content appears
	Col     int         // 0-indexed column where the tooltip starts
	BgColor color.Color // Optional background color (nil uses default)
}

var (
	tooltipBg color.Color = lipgloss.Color("240")
	tooltipFg color.Color = lipgloss.Color("255")
)

// OverlayTooltips renders tooltips on top of existing output without affecting layout.
// Each tooltip is 3 rows: half-block top edge, content row, half-block bottom edge.
func OverlayTooltips(output string, tooltips []Tooltip) string {
	if len(tooltips) == 0 {
		return output
	}

	lines := strings.Split(output, "\n")

	for _, tip := range tooltips {
		bg := tooltipBg
		if tip.BgColor != nil {
			bg = tip.BgColor
		}
		fg := tooltipContrastFG(bg)
		tipStyle := lipgloss.NewStyle().Background(bg).Foreground(fg).PaddingLeft(1).PaddingRight(1)
		edgeTop := lipgloss.NewStyle().Foreground(bg).Background(styles.TermBg)
		edgeBottom := lipgloss.NewStyle().Foreground(styles.TermBg).Background(bg)
		edgeLeft := lipgloss.NewStyle().Foreground(bg).Background(styles.TermBg)
		edgeRight := lipgloss.NewStyle().Foreground(bg).Background(styles.TermBg)

		content := tipStyle.Render(tip.Text)
		contentWidth := lipgloss.Width(content)
		fullWidth := contentWidth + 2 // +2 for ▐ and ▌

		leftEdge := edgeLeft.Render("\u2590")   // ▐
		rightEdge := edgeRight.Render("\u258c") // ▌
		topEdge := " " + edgeTop.Render(strings.Repeat("\u2584", contentWidth)) + " "
		bottomEdge := " " + edgeBottom.Render(strings.Repeat("\u2584", contentWidth)) + " "
		contentRow := leftEdge + content + rightEdge

		type splice struct {
			row      int
			rendered string
			width    int
		}
		splices := []splice{
			{tip.Row - 1, topEdge, fullWidth},
			{tip.Row, contentRow, fullWidth},
			{tip.Row + 1, bottomEdge, fullWidth},
		}

		for _, s := range splices {
			if s.row < 0 || s.row >= len(lines) {
				continue
			}
			line := lines[s.row]
			lineWidth := visibleWidth(line)
			if lineWidth < tip.Col+s.width {
				line += strings.Repeat(" ", tip.Col+s.width-lineWidth)
			}
			lines[s.row] = spliceLine(line, tip.Col, s.rendered, s.width)
		}
	}

	return strings.Join(lines, "\n")
}

// newTooltipAtZone creates a tooltip positioned relative to a zone based on placement.
func newTooltipAtZone(zi ZonePosition, text string, placement TooltipPlacement) *Tooltip {
	tipStyle := lipgloss.NewStyle().Background(tooltipBg).Foreground(tooltipFg).PaddingLeft(1).PaddingRight(1)
	contentWidth := lipgloss.Width(tipStyle.Render(text)) + 2 // +2 for half-block side edges

	switch placement {
	case TooltipRight:
		row := zi.StartY + (zi.EndY-zi.StartY)/2
		return &Tooltip{
			Text: text,
			Row:  row,
			Col:  zi.EndX + 2,
		}
	case TooltipAbove:
		zoneCenter := zi.StartX + (zi.EndX-zi.StartX+1)/2
		col := zoneCenter - contentWidth/2
		if col < 0 {
			col = 0
		}
		return &Tooltip{
			Text: text,
			Row:  zi.StartY - 2,
			Col:  col,
		}
	default: // TooltipBelow
		zoneCenter := zi.StartX + (zi.EndX-zi.StartX+1)/2
		col := zoneCenter - contentWidth/2
		if col < 0 {
			col = 0
		}
		return &Tooltip{
			Text: text,
			Row:  zi.EndY + 2,
			Col:  col,
		}
	}
}

// tooltipContrastFG picks black or white text based on the background color luminance.
func tooltipContrastFG(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA()
	// RGBA returns 16-bit values, scale to 8-bit
	rf, gf, bf := float64(r>>8), float64(g>>8), float64(b>>8)
	lum := 0.299*rf + 0.587*gf + 0.114*bf
	if lum > 140 {
		return lipgloss.Color("0")
	}
	return lipgloss.Color("255")
}
