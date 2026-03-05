package layout

import (
	"strings"

	"asciishader/tui/components"
	"asciishader/tui/styles"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// BottomPanel renders a panel at the bottom of the content area.
type BottomPanel struct {
	expanded bool
	height   int // fixed height when expanded
	title    string

	// Animation
	animator *components.PanelAnimator

	// Drag-to-resize
	resizer *components.PanelResizer
}

// Bottom panel styles
var (
	bottomPanelBg = styles.ChromeBgDark

	bottomPanelBgANSI = "\x1b[48;5;233m"

	bottomPanelSepStyle = lipgloss.NewStyle().
				Foreground(bottomPanelBg)

	bottomPanelEdgeActiveStyle = lipgloss.NewStyle().
					Foreground(styles.ChromeResizeEdge).
					Background(bottomPanelBg)

	bottomPanelRowBg = lipgloss.NewStyle().
				Background(bottomPanelBg)

	bottomPanelTitleStyle = lipgloss.NewStyle().
				Foreground(styles.ChromeFgAccent).
				Background(bottomPanelBg).
				Bold(true)

	bottomPanelHintStyle = lipgloss.NewStyle().
				Foreground(styles.ChromeFgMuted).
				Background(bottomPanelBg)
)

// NewBottomPanel creates a new bottom panel.
func NewBottomPanel() *BottomPanel {
	return &BottomPanel{
		expanded: false,
		height:   12,
		title:    "",
		animator: components.NewPanelAnimator("bottom-panel", 6),
		resizer:  components.NewPanelResizer(components.ResizeVertical, 6),
	}
}

// ToggleExpanded toggles between expanded and collapsed states with animation.
func (bp *BottomPanel) ToggleExpanded() tea.Cmd {
	startHeight := bp.Height()
	bp.animator.Stop()
	bp.expanded = !bp.expanded
	targetHeight := bp.fullHeight()
	return bp.animator.Start(startHeight, targetHeight)
}

// AnimTick advances the animation by one frame.
func (bp *BottomPanel) AnimTick() tea.Cmd {
	return bp.animator.Tick()
}

// IsExpanded returns whether the bottom panel is expanded.
func (bp *BottomPanel) IsExpanded() bool {
	return bp.expanded
}

// Animating returns whether the panel is currently animating.
func (bp *BottomPanel) Animating() bool {
	return bp.animator.Animating()
}

// SetExpanded explicitly sets the expanded state (no animation).
func (bp *BottomPanel) SetExpanded(expanded bool) {
	if bp.expanded != expanded {
		bp.expanded = expanded
		bp.animator.Stop()
	}
}

// SetTitle sets the title displayed in the bottom panel header bar.
func (bp *BottomPanel) SetTitle(title string) {
	bp.title = title
}

// SetHeight sets the height of the bottom panel when expanded.
func (bp *BottomPanel) SetHeight(height int) {
	bp.height = height
}

// Resizer returns the panel resizer for external mouse handling.
func (bp *BottomPanel) Resizer() *components.PanelResizer {
	return bp.resizer
}

// HandleResizeEvent processes a mouse event for edge drag-to-resize.
func (bp *BottomPanel) HandleResizeEvent(msg tea.MouseMsg, screenHeight int) bool {
	if !bp.expanded && !bp.animator.Animating() {
		return false
	}
	newHeight, handled := bp.resizer.HandleMouse(msg, bp.height, screenHeight)
	if handled && newHeight != bp.height {
		bp.height = newHeight
	}
	return handled
}

// fullHeight returns the non-animated total height.
func (bp *BottomPanel) fullHeight() int {
	if !bp.expanded {
		return 0
	}
	return bp.height + 1 // +1 for top edge
}

// Height returns the total height of the bottom panel including the top edge.
func (bp *BottomPanel) Height() int {
	if bp.animator.Animating() {
		return bp.animator.Value()
	}
	return bp.fullHeight()
}

// ContentHeight returns the height available for content (excluding separator and title).
func (bp *BottomPanel) ContentHeight() int {
	h := bp.height - 1 // -1 for title row
	if h < 1 {
		h = 1
	}
	return h
}

// Render renders the bottom panel with the given width and content string.
func (bp *BottomPanel) Render(width int, content string) string {
	if !bp.expanded && !bp.animator.Animating() {
		return ""
	}
	if width <= 0 {
		return ""
	}

	edgeActive := bp.resizer.IsHovering() || bp.resizer.IsDragging()
	bg := bottomPanelRowBg

	// Top edge separator
	var topEdge string
	if edgeActive {
		mid := width / 2
		leftPart := strings.Repeat("╌", mid)
		rightPart := strings.Repeat("╌", width-mid-1)
		topEdge = bottomPanelEdgeActiveStyle.Render(leftPart + "↕" + rightPart)
	} else {
		topEdge = bottomPanelSepStyle.Background(styles.TermBg).Render(strings.Repeat("▄", width))
	}

	// Title bar row
	titleRow := bp.renderTitleRow(width)

	// Content height = total height - 1 (title row); separator is extra
	contentHeight := bp.height - 1
	if contentHeight < 1 {
		contentHeight = 1
	}

	contentLines := strings.Split(content, "\n")

	rows := make([]string, 0, bp.height+1)
	rows = append(rows, topEdge)
	rows = append(rows, titleRow)

	for i := 0; i < contentHeight; i++ {
		var row string
		if i < len(contentLines) {
			line := contentLines[i]
			lineWidth := lipgloss.Width(line)
			padding := width - lineWidth - 1
			if padding < 0 {
				padding = 0
			}
			lineWithBg := strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+bottomPanelBgANSI)
			row = bottomPanelBgANSI + " " + lineWithBg + strings.Repeat(" ", padding) + "\x1b[0m"
		} else {
			row = bg.Render(strings.Repeat(" ", width))
		}
		rows = append(rows, row)
	}

	// During animation, clip to animated height
	if bp.animator.Animating() {
		animH := bp.animator.Value()
		if animH < len(rows) {
			rows = rows[:animH]
		}
	}

	return strings.Join(rows, "\n")
}

// renderTitleRow renders the title bar.
func (bp *BottomPanel) renderTitleRow(width int) string {
	bg := bottomPanelRowBg

	if bp.title == "" {
		return bg.Render(strings.Repeat(" ", width))
	}

	title := bottomPanelTitleStyle.Render(" " + bp.title)
	hint := bottomPanelHintStyle.Render("  ctrl+r: compile ")

	titleWidth := lipgloss.Width(title)
	hintWidth := lipgloss.Width(hint)

	padding := width - titleWidth - hintWidth
	if padding < 0 {
		padding = 0
	}

	return title + bg.Render(strings.Repeat(" ", padding)) + hint
}
