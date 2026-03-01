package main

import (
	"fmt"
	"math"

	"github.com/charmbracelet/lipgloss"
)

// Slider is a render-only widget for adjusting a numeric parameter.
type Slider struct {
	Label  string
	Value  float64
	Min    float64
	Max    float64
	Step   float64
	Format string // e.g. "%.2f" or "%.0f"
}

// Increase advances the value by one step.
func (s *Slider) Increase() {
	s.Value = math.Min(s.Value+s.Step, s.Max)
}

// Decrease reduces the value by one step.
func (s *Slider) Decrease() {
	s.Value = math.Max(s.Value-s.Step, s.Min)
}

// Render returns the slider as a styled string fitting in width columns.
// focused highlights the row.
func (s *Slider) Render(width int, focused bool) string {
	valStr := fmt.Sprintf(s.Format, s.Value)

	// Layout: " Label   [═══│───]  val"
	labelW := 12
	valW := len(valStr)
	barW := width - labelW - valW - 5 // 5 = spaces + brackets
	if barW < 4 {
		barW = 4
	}

	// Compute fill position
	frac := (s.Value - s.Min) / (s.Max - s.Min)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	fillPos := int(frac * float64(barW))
	if fillPos > barW {
		fillPos = barW
	}

	// Build bar
	bar := make([]byte, barW)
	for i := 0; i < barW; i++ {
		if i < fillPos {
			bar[i] = '='
		} else if i == fillPos {
			bar[i] = '|'
		} else {
			bar[i] = '-'
		}
	}

	// Pad label
	label := s.Label
	for len(label) < labelW {
		label += " "
	}
	if len(label) > labelW {
		label = label[:labelW]
	}

	line := fmt.Sprintf(" %s[%s] %s", label, string(bar), valStr)

	// Pad to full width
	for len(line) < width {
		line += " "
	}

	if focused {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("240"))
		return style.Render(line)
	}
	return line
}
