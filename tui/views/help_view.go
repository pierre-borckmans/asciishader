package views

import (
	"strings"

	"asciishader/tui/components"

	"github.com/charmbracelet/lipgloss"
)

// HelpView is a scrollable keybinding reference.
type HelpView struct {
	scrollView *components.ScrollableView
	width      int
	height     int
}

// NewHelpView creates a new help view.
func NewHelpView() *HelpView {
	return &HelpView{
		scrollView: components.NewScrollableView(),
	}
}

// ScrollView returns the underlying scrollable view for mouse handling.
func (h *HelpView) ScrollView() *components.ScrollableView {
	return h.scrollView
}

// SetSize updates the available display area.
func (h *HelpView) SetSize(width, height int) {
	h.width = width
	h.height = height
	h.scrollView.SetSize(width, height)
}

// HandleKey processes a key press. Returns true if consumed.
func (h *HelpView) HandleKey(key string) bool {
	switch key {
	case "up", "k":
		h.scrollView.ScrollUp(1)
		return true
	case "down", "j":
		h.scrollView.ScrollDown(1)
		return true
	case "g":
		h.scrollView.ScrollToTop()
		return true
	case "G":
		h.scrollView.ScrollToBottom()
		return true
	}
	return false
}

// Render returns the help view content as a string.
func (h *HelpView) Render(width, height int) string {
	h.width = width
	h.height = height

	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("218")).
		Bold(true)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	binding := func(key, desc string) string {
		k := keyStyle.Render(pad("  "+key, 20))
		d := descStyle.Render(desc)
		return k + d
	}

	sep := dimStyle.Render(pad(" ───────────────────────────", width))

	var lines []string

	// Views
	lines = append(lines, headerStyle.Render(pad(" Views", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("F1", "Shader view"))
	lines = append(lines, binding("F2", "Player view"))
	lines = append(lines, binding("F3", "Gallery view"))
	lines = append(lines, binding("F4", "Help view"))
	lines = append(lines, "")

	// Navigation
	lines = append(lines, headerStyle.Render(pad(" Navigation", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("arrows/hjkl", "Rotate camera"))
	lines = append(lines, binding("+/-", "Zoom in/out"))
	lines = append(lines, binding("right-click", "Pan camera"))
	lines = append(lines, binding("scroll", "Zoom in/out"))
	lines = append(lines, binding("r", "Reset camera & params"))
	lines = append(lines, binding("a", "Toggle auto-rotate"))
	lines = append(lines, "")

	// Scenes
	lines = append(lines, headerStyle.Render(pad(" Scenes", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("n", "Next scene"))
	lines = append(lines, binding("N", "Previous scene"))
	lines = append(lines, "")

	// Rendering
	lines = append(lines, headerStyle.Render(pad(" Rendering", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("m", "Cycle render mode"))
	lines = append(lines, binding("g", "Toggle GPU/CPU"))
	lines = append(lines, binding("space", "Pause/resume"))
	lines = append(lines, binding("[/]", "Contrast -/+"))
	lines = append(lines, binding("1/!", "Spread +/-"))
	lines = append(lines, binding("2/@", "ExtDist +/-"))
	lines = append(lines, binding("3/#", "Ambient +/-"))
	lines = append(lines, binding("4/$", "SpecPower +/-"))
	lines = append(lines, binding("5/%", "Shadow steps +/-"))
	lines = append(lines, binding("6/^", "AO steps +/-"))
	lines = append(lines, binding("p", "Toggle CPU profile"))
	lines = append(lines, "")

	// Panels
	lines = append(lines, headerStyle.Render(pad(" Panels", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("s", "Toggle controls panel"))
	lines = append(lines, binding("e", "Toggle GLSL editor"))
	lines = append(lines, binding("tab", "Cycle focus"))
	lines = append(lines, binding("esc", "Return focus to viewport"))
	lines = append(lines, "")

	// Recording
	lines = append(lines, headerStyle.Render(pad(" Recording", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("o", "Start/stop recording"))
	lines = append(lines, binding("1-4", "Region presets (in select)"))
	lines = append(lines, binding("enter", "Confirm region"))
	lines = append(lines, binding("esc", "Cancel selection"))
	lines = append(lines, "")

	// General
	lines = append(lines, headerStyle.Render(pad(" General", width)))
	lines = append(lines, sep)
	lines = append(lines, binding("q/esc", "Quit"))
	lines = append(lines, binding("ctrl+c", "Force quit"))

	content := strings.Join(lines, "\n")
	h.scrollView.SetSize(width, height)
	return h.scrollView.RenderContent(content)
}
