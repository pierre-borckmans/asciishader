package layout

import (
	"strings"

	"asciishader/tui/components"
	"asciishader/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// RightPanel renders a panel on the right side of the TUI.
type RightPanel struct {
	expanded bool
	width    int // fixed width when expanded

	// Scrollable view for content
	scrollView *components.ScrollableView

	// Animation
	animator *components.PanelAnimator

	// Drag-to-resize
	resizer *components.PanelResizer
}

// Right panel styles
var (
	rightPanelBg = styles.ChromeBgDark

	rightPanelBgANSI = "\x1b[48;5;233m"

	rightPanelSepStyle = lipgloss.NewStyle().
				Foreground(rightPanelBg).
				Background(rightPanelBg)

	rightPanelEdgeActiveStyle = lipgloss.NewStyle().
					Foreground(styles.ChromeResizeEdge).
					Background(rightPanelBg)

	rightPanelRowBg = lipgloss.NewStyle().
			Background(rightPanelBg)
)

// NewRightPanel creates a new right panel.
func NewRightPanel() *RightPanel {
	sv := components.NewScrollableView()
	sv.SetTruncate(true)
	return &RightPanel{
		expanded:   true,
		width:      42,
		scrollView: sv,
		animator:   components.NewPanelAnimator("right-panel", 6),
		resizer:    components.NewPanelResizer(components.ResizeHorizontal, 28),
	}
}

// ToggleExpanded toggles between expanded and collapsed states with animation.
func (rp *RightPanel) ToggleExpanded() tea.Cmd {
	startWidth := rp.Width()
	rp.animator.Stop()
	rp.expanded = !rp.expanded
	targetWidth := rp.fullWidth()
	return rp.animator.Start(startWidth, targetWidth)
}

// AnimTick advances the animation by one frame.
func (rp *RightPanel) AnimTick() tea.Cmd {
	return rp.animator.Tick()
}

// IsExpanded returns whether the right panel is expanded.
func (rp *RightPanel) IsExpanded() bool {
	return rp.expanded
}

// Animating returns whether the panel is currently animating.
func (rp *RightPanel) Animating() bool {
	return rp.animator.Animating()
}

// SetExpanded explicitly sets the expanded state (no animation).
func (rp *RightPanel) SetExpanded(expanded bool) {
	if rp.expanded != expanded {
		rp.expanded = expanded
		rp.animator.Stop()
	}
}

// SetWidth sets the width of the right panel when expanded.
func (rp *RightPanel) SetWidth(width int) {
	rp.width = width
}

// fullWidth returns the non-animated total width.
func (rp *RightPanel) fullWidth() int {
	if !rp.expanded {
		return 0
	}
	return rp.width + 1 // +1 for left edge █
}

// Width returns the total width including the edge.
func (rp *RightPanel) Width() int {
	if rp.animator.Animating() {
		return rp.animator.Value()
	}
	return rp.fullWidth()
}

// InnerWidth returns the usable content width (without the separator edge).
func (rp *RightPanel) InnerWidth() int {
	return rp.width
}

// ScrollView returns the scrollable view for external mouse/key handling.
func (rp *RightPanel) ScrollView() *components.ScrollableView {
	return rp.scrollView
}

// Resizer returns the panel resizer for external mouse handling.
func (rp *RightPanel) Resizer() *components.PanelResizer {
	return rp.resizer
}

// HandleResizeEvent processes a mouse event for edge drag-to-resize.
func (rp *RightPanel) HandleResizeEvent(msg tea.MouseMsg, screenWidth int) bool {
	if !rp.expanded && !rp.animator.Animating() {
		return false
	}
	newWidth, handled := rp.resizer.HandleMouse(msg, rp.width, screenWidth)
	if handled && newWidth != rp.width {
		rp.width = newWidth
	}
	return handled
}

// HandleMouseEvent forwards mouse events to the scroll view.
func (rp *RightPanel) HandleMouseEvent(msg tea.MouseMsg) bool {
	if !rp.expanded {
		return false
	}

	if _, ok := msg.(tea.MouseWheelMsg); ok {
		mouse := msg.Mouse()
		sv := rp.scrollView
		relX := mouse.X - sv.ScreenX()
		if relX < -1 || relX >= rp.width {
			return false
		}
	}

	return rp.scrollView.HandleMouse(msg)
}

// Render renders the right panel with the given height and content.
func (rp *RightPanel) Render(height int, content string) string {
	if !rp.expanded && !rp.animator.Animating() {
		return ""
	}
	if height <= 0 {
		return ""
	}

	edgeActive := rp.resizer.IsHovering() || rp.resizer.IsDragging()

	iw := rp.width
	edgeNormal := rightPanelSepStyle.Render("█")
	edgeHover := rightPanelEdgeActiveStyle.Render("╏")
	edgeArrow := rightPanelEdgeActiveStyle.Render("↔")
	midRow := height / 2
	bg := rightPanelRowBg

	// Update scroll view dimensions
	contentWidth := iw - 1 // -1 for left padding space
	if contentWidth < 1 {
		contentWidth = 1
	}
	rp.scrollView.SetSize(contentWidth, height)

	// Render content through the scrollable view
	scrolledContent := rp.scrollView.RenderContent(content)
	contentLines := strings.Split(scrolledContent, "\n")

	rows := make([]string, 0, height)
	for i := 0; i < height; i++ {
		// Pick edge character for this row
		var leftEdge string
		if edgeActive {
			if i == midRow {
				leftEdge = edgeArrow
			} else {
				leftEdge = edgeHover
			}
		} else {
			leftEdge = edgeNormal
		}

		var row string
		if i < len(contentLines) {
			line := contentLines[i]
			lineWithBg := strings.ReplaceAll(line, "\x1b[0m", "\x1b[0m"+rightPanelBgANSI)
			row = leftEdge + rightPanelBgANSI + " " + lineWithBg + "\x1b[0m"
		} else {
			row = leftEdge + bg.Render(strings.Repeat(" ", iw))
		}
		rows = append(rows, row)
	}

	// During animation, clip each row to the animated width
	if rp.animator.Animating() {
		animW := rp.animator.Value()
		clipStyle := lipgloss.NewStyle().MaxWidth(animW).Background(rightPanelBg)
		for i, row := range rows {
			rows[i] = clipStyle.Render(row)
		}
	}

	return strings.Join(rows, "\n")
}
