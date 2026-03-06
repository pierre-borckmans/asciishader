package controls

import (
	"fmt"
	"strconv"

	"asciishader/pkg/core"
	"asciishader/tui/components"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

// ControlsTab manages the 7 parameter sliders + scene selector + render mode toggle.
type ControlsTab struct {
	sliders     []*components.Slider
	focus       int // which item is focused (0-8: 7 sliders + scene + blocks)
	numItems    int
	zoned       *components.ZonedInteraction
	renderWidth int // last render width
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
	ctrlBlocks      = 8
)

// NewControlsTab creates the controls tab with default slider values.
func NewControlsTab() *ControlsTab {
	ct := &ControlsTab{
		numItems: 9,
		zoned:    components.NewZonedInteraction("ctrl"),
		sliders: []*components.Slider{
			{Label: "Contrast", Min: 0.5, Max: 5.0, Step: 0.25, Format: "%.2f"},
			{Label: "Spread", Min: 0.25, Max: 3.0, Step: 0.25, Format: "%.2f"},
			{Label: "ExtDist", Min: 0.25, Max: 3.0, Step: 0.25, Format: "%.2f"},
			{Label: "Ambient", Min: 0.0, Max: 1.0, Step: 0.05, Format: "%.2f"},
			{Label: "SpecPower", Min: 4, Max: 128, Step: 0, Format: "%.0f"}, // uses *1.5 / /1.5
			{Label: "Shadows", Min: 0, Max: 48, Step: 4, Format: "%.0f"},
			{Label: "AO Steps", Min: 0, Max: 10, Step: 1, Format: "%.0f"},
		},
	}
	return ct
}

// SyncFromRenderConfig reads current values from the config into sliders.
func (ct *ControlsTab) SyncFromRenderConfig(rc *core.RenderConfig) {
	ct.sliders[ctrlContrast].Value = rc.Contrast
	ct.sliders[ctrlSpread].Value = rc.Spread
	ct.sliders[ctrlExtDist].Value = rc.ExtDist
	ct.sliders[ctrlAmbient].Value = rc.Ambient
	ct.sliders[ctrlSpecPower].Value = rc.SpecPower
	ct.sliders[ctrlShadowSteps].Value = float64(rc.ShadowSteps)
	ct.sliders[ctrlAOSteps].Value = float64(rc.AOSteps)
}

// SyncToRenderConfig writes slider values back to the config.
func (ct *ControlsTab) SyncToRenderConfig(rc *core.RenderConfig) {
	rc.Contrast = ct.sliders[ctrlContrast].Value
	rc.Spread = ct.sliders[ctrlSpread].Value
	rc.ExtDist = ct.sliders[ctrlExtDist].Value
	rc.Ambient = ct.sliders[ctrlAmbient].Value
	rc.SpecPower = ct.sliders[ctrlSpecPower].Value
	rc.ShadowSteps = int(ct.sliders[ctrlShadowSteps].Value)
	rc.AOSteps = int(ct.sliders[ctrlAOSteps].Value)
}

// HandleKey processes a key press. Returns true if consumed.
func (ct *ControlsTab) HandleKey(key string, m AppState) bool {
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

func (ct *ControlsTab) adjustValue(dir int, m AppState) bool {
	switch ct.focus {
	case ctrlScene:
		if dir > 0 {
			m.SetScene((m.GetScene() + 1) % m.NumScenes())
		} else {
			m.SetScene((m.GetScene() - 1 + m.NumScenes()) % m.NumScenes())
		}
		m.SetTime(0)
		return true
	case ctrlBlocks:
		if dir > 0 {
			m.GetRenderConfig().RenderMode = (m.GetRenderConfig().RenderMode + 1) % core.RenderModeCount
		} else {
			m.GetRenderConfig().RenderMode = (m.GetRenderConfig().RenderMode + core.RenderModeCount - 1) % core.RenderModeCount
		}
		return true
	case ctrlSpecPower:
		s := ct.sliders[ctrlSpecPower]
		if dir > 0 {
			s.Value = core.Clamp(s.Value*1.5, s.Min, s.Max)
		} else {
			s.Value = core.Clamp(s.Value/1.5, s.Min, s.Max)
		}
		ct.SyncToRenderConfig(m.GetRenderConfig())
		return true
	default:
		if ct.focus >= 0 && ct.focus < len(ct.sliders) {
			s := ct.sliders[ct.focus]
			if dir > 0 {
				s.Increase()
			} else {
				s.Decrease()
			}
			ct.SyncToRenderConfig(m.GetRenderConfig())
			return true
		}
	}
	return false
}

// zoneIDs returns all interactive zone IDs for mouse handling.
func (ct *ControlsTab) zoneIDs() []string {
	ids := make([]string, 0, ct.numItems)
	for i := range ct.sliders {
		ids = append(ids, "slider-"+strconv.Itoa(i))
	}
	ids = append(ids, "scene", "blocks")
	return ids
}

// HandleMouse processes a mouse event for the controls panel.
// Returns true if handled.
func (ct *ControlsTab) HandleMouse(msg tea.MouseMsg, m AppState) bool {
	// Delegate to sliders first (they own drag/hover state)
	for i, s := range ct.sliders {
		zi := zone.Get(ct.zoned.ZoneID("slider-" + strconv.Itoa(i)))
		if !zi.IsZero() {
			s.SetScreenX(zi.StartX)
		}
		if s.IsDragging() {
			if s.HandleMouse(msg) {
				ct.SyncToRenderConfig(m.GetRenderConfig())
				return true
			}
		}
		// Check zone bounds for non-drag events
		if !zi.IsZero() && zi.InBounds(msg) {
			if s.HandleMouse(msg) {
				if msg.Action == tea.MouseActionPress {
					ct.focus = i
				}
				ct.SyncToRenderConfig(m.GetRenderConfig())
				return true
			}
		} else if msg.Action == tea.MouseActionMotion {
			s.ClearHover()
		}
	}

	// Use zoned interaction for hover + release-based clicks (scene, blocks)
	result := ct.zoned.HandleMouse(msg, ct.zoneIDs())

	if result.HoverChanged {
		return true
	}

	clicked := result.Clicked
	if clicked == "" {
		clicked = result.DoubleClicked
	}
	if clicked == "" {
		return false
	}

	// Scene click — left half = prev, right half = next
	if clicked == "scene" {
		ct.focus = ctrlScene
		zi := zone.Get(ct.zoned.ZoneID("scene"))
		if !zi.IsZero() {
			mid := (zi.StartX + zi.EndX) / 2
			if msg.X < mid {
				m.SetScene((m.GetScene() - 1 + m.NumScenes()) % m.NumScenes())
			} else {
				m.SetScene((m.GetScene() + 1) % m.NumScenes())
			}
			m.SetTime(0)
			m.SyncSceneGLSL()
		}
		return true
	}

	// Render mode cycle
	if clicked == "blocks" {
		ct.focus = ctrlBlocks
		m.GetRenderConfig().RenderMode = (m.GetRenderConfig().RenderMode + 1) % core.RenderModeCount
		return true
	}

	return false
}

// IsDragging returns whether any slider drag is in progress.
func (ct *ControlsTab) IsDragging() bool {
	for _, s := range ct.sliders {
		if s.IsDragging() {
			return true
		}
	}
	return false
}

// Render returns the controls tab content as a string.
func (ct *ControlsTab) Render(width int, m AppState) string {
	ct.renderWidth = width
	var lines string

	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)

	// Section header
	lines += headerStyle.Render(pad(" Parameters", width)) + "\n"
	lines += dimStyle.Render(pad(" ───────────────────────────", width)) + "\n"

	// Sliders — each wrapped in a zone
	for i, s := range ct.sliders {
		rendered := s.Render(width, ct.focus == i)
		if ct.zoned.IsHovered("slider-"+strconv.Itoa(i)) && ct.focus != i {
			rendered = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252")).
				Render(rendered)
		}
		lines += ct.zoned.Mark("slider-"+strconv.Itoa(i), rendered) + "\n"
	}

	lines += "\n"
	lines += headerStyle.Render(pad(" Scene", width)) + "\n"
	lines += dimStyle.Render(pad(" ───────────────────────────", width)) + "\n"

	// Scene selector — wrapped in zone
	sceneStr := fmt.Sprintf(" < %s >", m.SceneName(m.GetScene()))
	sceneLine := pad(sceneStr, width)
	if ct.focus == ctrlScene {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("240"))
		sceneLine = style.Render(sceneLine)
	} else if ct.zoned.IsHovered("scene") {
		sceneLine = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Render(sceneLine)
	}
	lines += ct.zoned.Mark("scene", sceneLine) + "\n"

	lines += "\n"
	lines += headerStyle.Render(pad(" Renderer", width)) + "\n"
	lines += dimStyle.Render(pad(" ───────────────────────────", width)) + "\n"

	// Render mode — wrapped in zone
	blocksLabel := " Shapes"
	switch m.GetRenderConfig().RenderMode {
	case core.RenderDual:
		blocksLabel = " Dual"
	case core.RenderBlocks:
		blocksLabel = " Blocks"
	case core.RenderHalfBlock:
		blocksLabel = " Half"
	case core.RenderBraille:
		blocksLabel = " Braille"
	case core.RenderDensity:
		blocksLabel = " Density"
	}
	blocksLine := pad(blocksLabel, width)
	if ct.focus == ctrlBlocks {
		style := lipgloss.NewStyle().
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("240"))
		blocksLine = style.Render(blocksLine)
	} else if ct.zoned.IsHovered("blocks") {
		blocksLine = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Render(blocksLine)
	}
	lines += ct.zoned.Mark("blocks", blocksLine) + "\n"

	return lines
}

func pad(s string, width int) string {
	for len(s) < width {
		s += " "
	}
	return s
}
