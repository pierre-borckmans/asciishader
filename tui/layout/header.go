package layout

import (
	"strings"

	"asciishader/tui/styles"

	"github.com/charmbracelet/lipgloss"
)

// ComposeHeader creates a full-width header bar with title and right-aligned info.
// Returns 3 lines: top edge, title bar, bottom edge (with junction colors for sidebar/right panel).
func ComposeHeader(title string, rightInfo string, width int, sidebarWidth int, rightPanelWidth int) string {
	if width == 0 {
		return title
	}

	bgColor := styles.ChromeBg
	bgStyle := lipgloss.NewStyle().Background(bgColor)
	brightStyle := lipgloss.NewStyle().Foreground(styles.ChromeFg).Background(bgColor).Bold(true)
	mutedStyle := lipgloss.NewStyle().Foreground(styles.ChromeFgMuted).Background(bgColor)

	titleText := brightStyle.Render("  " + title)
	rightText := mutedStyle.Render(rightInfo + "  ")

	titleWidth := lipgloss.Width(titleText)
	rightWidth := lipgloss.Width(rightText)
	middlePadding := width - titleWidth - rightWidth
	if middlePadding < 1 {
		middlePadding = 1
	}

	titleBar := titleText + bgStyle.Render(strings.Repeat(" ", middlePadding)) + rightText

	// Top edge: half blocks in header bg color on terminal bg
	innerWidth := width - 2
	if innerWidth < 0 {
		innerWidth = 0
	}
	topEdgeStyle := lipgloss.NewStyle().Foreground(bgColor).Background(styles.TermBg)
	topEdge := topEdgeStyle.Render("▄" + strings.Repeat("▄", innerWidth) + "▄")

	// Bottom edge: junction colors where sidebar and right panel meet
	sidebarJunctionStyle := lipgloss.NewStyle().Foreground(styles.ChromeBgLight).Background(bgColor)
	rightJunctionStyle := lipgloss.NewStyle().Foreground(styles.ChromeBgDark).Background(bgColor)
	restStyle := lipgloss.NewStyle().Foreground(styles.TermBg).Background(bgColor)

	var bottomEdge string
	if sidebarWidth > 0 || rightPanelWidth > 0 {
		lw := sidebarWidth
		if lw > width {
			lw = width
		}
		rw := rightPanelWidth
		if rw > width {
			rw = width
		}
		middleWidth := width - lw - rw
		if middleWidth < 0 {
			middleWidth = 0
		}
		left := ""
		if lw > 0 {
			left = sidebarJunctionStyle.Render(strings.Repeat("▄", lw))
		}
		mid := restStyle.Render(strings.Repeat("▄", middleWidth))
		right := ""
		if rw > 0 {
			right = rightJunctionStyle.Render(strings.Repeat("▄", rw))
		}
		bottomEdge = left + mid + right
	} else {
		bottomEdge = restStyle.Render(strings.Repeat("▄", width))
	}

	return topEdge + "\n" + titleBar + "\n" + bottomEdge
}
