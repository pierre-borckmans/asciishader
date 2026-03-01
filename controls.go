package main

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
)

// ControlsTab manages the 7 parameter sliders + scene selector + GPU toggle.
type ControlsTab struct {
	sliders  []*Slider
	focus    int // which item is focused (0-8: 7 sliders + scene + gpu)
	numItems int
}

const (
	ctrlContrast    = 0
	ctrlSpread      = 1
	ctrlExtDist     = 2
	ctrlAmbient     = 3
	ctrlSpecPower   = 4
	ctrlShadowSteps = 5
	ctrlAOSteps     = 6
	ctrlScene       = 7
	ctrlGPU         = 8
)

// NewControlsTab creates the controls tab with default slider values.
func NewControlsTab() *ControlsTab {
	ct := &ControlsTab{
		numItems: 9,
		sliders: []*Slider{
			{Label: "Contrast", Min: 0.5, Max: 5.0, Step: 0.25, Format: "%.2f"},
			{Label: "Spread", Min: 0.25, Max: 3.0, Step: 0.25, Format: "%.2f"},
			{Label: "ExtDist", Min: 0.25, Max: 3.0, Step: 0.25, Format: "%.2f"},
			{Label: "Ambient", Min: 0.0, Max: 0.5, Step: 0.05, Format: "%.2f"},
			{Label: "SpecPower", Min: 4, Max: 128, Step: 0, Format: "%.0f"}, // uses *1.5 / /1.5
			{Label: "Shadows", Min: 0, Max: 48, Step: 4, Format: "%.0f"},
			{Label: "AO Steps", Min: 0, Max: 10, Step: 1, Format: "%.0f"},
		},
	}
	return ct
}

// SyncFromRenderer reads current values from the renderer into sliders.
func (ct *ControlsTab) SyncFromRenderer(r *Renderer) {
	ct.sliders[ctrlContrast].Value = r.Contrast
	ct.sliders[ctrlSpread].Value = r.Spread
	ct.sliders[ctrlExtDist].Value = r.ExtDist
	ct.sliders[ctrlAmbient].Value = r.Ambient
	ct.sliders[ctrlSpecPower].Value = r.SpecPower
	ct.sliders[ctrlShadowSteps].Value = float64(r.ShadowSteps)
	ct.sliders[ctrlAOSteps].Value = float64(r.AOSteps)
}

// SyncToRenderer writes slider values back to the renderer.
func (ct *ControlsTab) SyncToRenderer(r *Renderer) {
	r.Contrast = ct.sliders[ctrlContrast].Value
	r.Spread = ct.sliders[ctrlSpread].Value
	r.ExtDist = ct.sliders[ctrlExtDist].Value
	r.Ambient = ct.sliders[ctrlAmbient].Value
	r.SpecPower = ct.sliders[ctrlSpecPower].Value
	r.ShadowSteps = int(ct.sliders[ctrlShadowSteps].Value)
	r.AOSteps = int(ct.sliders[ctrlAOSteps].Value)
}

// HandleKey processes a key press. Returns true if consumed.
func (ct *ControlsTab) HandleKey(key string, m *model) bool {
	switch key {
	case "up", "k":
		ct.focus--
		if ct.focus < 0 {
			ct.focus = ct.numItems - 1
		}
		return true
	case "down", "j":
		ct.focus++
		if ct.focus >= ct.numItems {
			ct.focus = 0
		}
		return true
	case "left", "h":
		return ct.adjustValue(-1, m)
	case "right", "l":
		return ct.adjustValue(1, m)
	}
	return false
}

func (ct *ControlsTab) adjustValue(dir int, m *model) bool {
	switch ct.focus {
	case ctrlScene:
		if dir > 0 {
			m.scene = (m.scene + 1) % len(scenes)
		} else {
			m.scene = (m.scene - 1 + len(scenes)) % len(scenes)
		}
		m.time = 0
		return true
	case ctrlGPU:
		if m.gpu != nil {
			m.gpuMode = !m.gpuMode
		}
		return true
	case ctrlSpecPower:
		// SpecPower uses multiplicative steps
		s := ct.sliders[ctrlSpecPower]
		if dir > 0 {
			s.Value = clamp(s.Value*1.5, s.Min, s.Max)
		} else {
			s.Value = clamp(s.Value/1.5, s.Min, s.Max)
		}
		ct.SyncToRenderer(m.renderer)
		return true
	default:
		if ct.focus >= 0 && ct.focus < len(ct.sliders) {
			s := ct.sliders[ct.focus]
			if dir > 0 {
				s.Increase()
			} else {
				s.Decrease()
			}
			ct.SyncToRenderer(m.renderer)
			return true
		}
	}
	return false
}

// Render returns the controls tab content as a string.
func (ct *ControlsTab) Render(width int, m *model) string {
	var lines string

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)

	// Section header
	lines += headerStyle.Render(pad(" Parameters", width)) + "\n"
	lines += dimStyle.Render(pad(" ───────────────────────────", width)) + "\n"

	// Sliders
	for i, s := range ct.sliders {
		lines += s.Render(width, ct.focus == i) + "\n"
	}

	lines += "\n"
	lines += headerStyle.Render(pad(" Scene", width)) + "\n"
	lines += dimStyle.Render(pad(" ───────────────────────────", width)) + "\n"

	// Scene selector
	sceneStr := fmt.Sprintf(" < %s >", scenes[m.scene].Name)
	sceneLine := pad(sceneStr, width)
	if ct.focus == ctrlScene {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("240"))
		lines += style.Render(sceneLine) + "\n"
	} else {
		lines += sceneLine + "\n"
	}

	lines += "\n"
	lines += headerStyle.Render(pad(" Renderer", width)) + "\n"
	lines += dimStyle.Render(pad(" ───────────────────────────", width)) + "\n"

	// GPU/CPU toggle
	gpuLabel := " CPU"
	if m.gpuMode && m.gpu != nil {
		gpuLabel = " GPU"
	}
	gpuLine := pad(gpuLabel, width)
	if ct.focus == ctrlGPU {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("240"))
		lines += style.Render(gpuLine) + "\n"
	} else {
		lines += gpuLine + "\n"
	}

	return lines
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
