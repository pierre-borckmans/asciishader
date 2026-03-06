package views

import (
	"fmt"
	"strings"

	"asciishader/tui/components"

	"charm.land/lipgloss/v2"
)

// GalleryView is a scrollable scene browser list.
type GalleryView struct {
	Selected   int // highlighted scene index
	scrollView *components.ScrollableView
	width      int
	height     int
}

// NewGalleryView creates a new gallery view.
func NewGalleryView() *GalleryView {
	return &GalleryView{
		scrollView: components.NewScrollableView(),
	}
}

// ScrollView returns the underlying scrollable view for mouse handling.
func (g *GalleryView) ScrollView() *components.ScrollableView {
	return g.scrollView
}

// SetSize updates the available display area.
func (g *GalleryView) SetSize(width, height int) {
	g.width = width
	g.height = height
	g.scrollView.SetSize(width, height)
}

// HandleKey processes a key press. Returns the selected scene index (>=0) if Enter was pressed, else -1.
func (g *GalleryView) HandleKey(key string, sceneCount int) int {
	switch key {
	case "up", "k":
		g.Selected--
		if g.Selected < 0 {
			g.Selected = sceneCount - 1
		}
		g.scrollView.EnsureLineVisible(g.Selected)
	case "down", "j":
		g.Selected++
		if g.Selected >= sceneCount {
			g.Selected = 0
		}
		g.scrollView.EnsureLineVisible(g.Selected)
	case "enter":
		return g.Selected
	}
	return -1
}

// Render returns the gallery view content as a string.
func (g *GalleryView) Render(width, height int, sceneNames []string) string {
	g.width = width
	g.height = height

	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("218")).
		Bold(true)
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)

	var lines []string

	lines = append(lines, headerStyle.Render(pad(" Scenes", width)))
	lines = append(lines, dimStyle.Render(pad(" ───────────────────────────", width)))
	lines = append(lines, "")

	for i, name := range sceneNames {
		label := fmt.Sprintf("  %2d. %s", i+1, name)
		label = pad(label, width)
		if i == g.Selected {
			label = activeStyle.Render(label)
		} else {
			label = normalStyle.Render(label)
		}
		lines = append(lines, label)
	}

	content := strings.Join(lines, "\n")
	g.scrollView.SetSize(width, height)
	return g.scrollView.RenderContent(content)
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
