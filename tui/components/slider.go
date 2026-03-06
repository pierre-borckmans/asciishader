package components

import (
	"fmt"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const SliderLabelWidth = 12

// Slider is a widget for adjusting a numeric parameter with mouse support.
type Slider struct {
	Label  string
	Value  float64
	Min    float64
	Max    float64
	Step   float64
	Format string // e.g. "%.2f" or "%.0f"

	// Mouse state
	dragging   bool
	thumbHover bool

	// Render state (set during Render, used for mouse math)
	screenX int // X of the left edge of this slider on screen
	barW    int // bar width in columns
}

// Increase advances the value by one step.
func (s *Slider) Increase() {
	s.Value = math.Min(s.Value+s.Step, s.Max)
}

// Decrease reduces the value by one step.
func (s *Slider) Decrease() {
	s.Value = math.Max(s.Value-s.Step, s.Min)
}

// IsDragging returns whether a drag is in progress.
func (s *Slider) IsDragging() bool {
	return s.dragging
}

// frac returns the normalized 0–1 position of the value.
func (s *Slider) frac() float64 {
	f := (s.Value - s.Min) / (s.Max - s.Min)
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}

// thumbScreenX returns the screen X of the thumb.
func (s *Slider) thumbScreenX() int {
	pos := int(s.frac() * float64(s.barW-1))
	if pos >= s.barW {
		pos = s.barW - 1
	}
	return s.screenX + 1 + SliderLabelWidth + pos
}

// barStartX returns the screen X where the bar begins.
func (s *Slider) barStartX() int {
	return s.screenX + 1 + SliderLabelWidth
}

// HandleMouse processes a mouse event for this slider.
// The slider must be in bounds (caller checks zone). Returns true if handled.
func (s *Slider) HandleMouse(msg tea.MouseMsg) bool {
	mouse := msg.Mouse()

	if s.dragging {
		switch msg.(type) {
		case tea.MouseMotionMsg:
			s.setValueFromX(mouse.X)
			return true
		case tea.MouseReleaseMsg:
			s.setValueFromX(mouse.X)
			s.dragging = false
			return true
		}
	}

	switch msg.(type) {
	case tea.MouseMotionMsg:
		oldHover := s.thumbHover
		s.thumbHover = mouse.X == s.thumbScreenX()
		return s.thumbHover != oldHover

	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft {
			bStart := s.barStartX()
			bEnd := bStart + s.barW
			if mouse.X >= bStart && mouse.X < bEnd {
				s.dragging = true
				s.setValueFromX(mouse.X)
				return true
			}
		}
	}

	return false
}

// setValueFromX sets the value from a screen X coordinate.
func (s *Slider) setValueFromX(x int) {
	bStart := s.barStartX()
	frac := float64(x-bStart) / float64(s.barW-1)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	s.Value = s.Min + frac*(s.Max-s.Min)
	if s.Step > 0 {
		s.Value = math.Round(s.Value/s.Step) * s.Step
	}
	s.Value = math.Max(s.Min, math.Min(s.Max, s.Value))
}

// ClearHover resets hover state (call when mouse leaves the zone).
func (s *Slider) ClearHover() {
	s.thumbHover = false
}

// SetScreenX sets the screen X position for mouse coordinate math.
// Call this after zone.Scan() determines the actual position.
func (s *Slider) SetScreenX(x int) {
	s.screenX = x
}

// Render returns the slider as a styled string fitting in width columns.
func (s *Slider) Render(width int, focused bool) string {
	valStr := fmt.Sprintf(s.Format, s.Value)

	// Layout: " Label       ━━━●─── val"
	valW := len(valStr)
	barW := width - SliderLabelWidth - valW - 3
	if barW < 4 {
		barW = 4
	}

	s.barW = barW

	thumbPos := int(s.frac() * float64(barW-1))
	if thumbPos >= barW {
		thumbPos = barW - 1
	}

	leftWidth := thumbPos
	rightWidth := barW - thumbPos - 1
	if rightWidth < 0 {
		rightWidth = 0
	}

	// Build bar
	var bar strings.Builder
	thumbColor := "39" // Cyan
	if focused || s.thumbHover || s.dragging {
		thumbColor = "226" // Yellow
	}
	bar.WriteString("\x1b[38;5;39m")
	bar.WriteString(strings.Repeat("━", leftWidth))
	bar.WriteString("\x1b[38;5;" + thumbColor + "m●\x1b[38;5;39m")
	bar.WriteString(strings.Repeat("─", rightWidth))
	bar.WriteString("\x1b[0m")

	// Pad label
	label := s.Label
	for len(label) < SliderLabelWidth {
		label += " "
	}
	if len(label) > SliderLabelWidth {
		label = label[:SliderLabelWidth]
	}

	line := fmt.Sprintf(" %s%s %s", label, bar.String(), valStr)

	// Pad to full width
	visibleWidth := 1 + SliderLabelWidth + barW + 1 + valW
	if visibleWidth < width {
		line += strings.Repeat(" ", width-visibleWidth)
	}

	if focused {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("240"))
		return style.Render(line)
	}
	return line
}
