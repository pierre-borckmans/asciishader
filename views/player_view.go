package views

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"asciishader/clip"
	"asciishader/components"

	"github.com/charmbracelet/lipgloss"
)

// PlayerView is an embedded clip player with file picker.
type PlayerView struct {
	Player      *clip.Player
	files       []string // discovered .asciirec files
	selectedIdx int
	scrollView  *components.ScrollableView
	Loaded      bool
	width       int
	height      int
}

// NewPlayerView creates a new player view.
func NewPlayerView() *PlayerView {
	return &PlayerView{
		scrollView: components.NewScrollableView(),
	}
}

// ScrollView returns the underlying scrollable view for mouse handling.
func (pv *PlayerView) ScrollView() *components.ScrollableView {
	return pv.scrollView
}

// SetSize updates the available display area.
func (pv *PlayerView) SetSize(width, height int) {
	pv.width = width
	pv.height = height
	pv.scrollView.SetSize(width, height)
	if pv.Player != nil {
		pv.Player.SetSize(width, height-1) // -1 for status line
	}
}

// ScanFiles discovers .asciirec files in the current working directory.
func (pv *PlayerView) ScanFiles() {
	pv.files = nil
	matches, err := filepath.Glob("*.asciirec")
	if err != nil {
		return
	}
	pv.files = matches
}

// HandleKey processes a key press. Returns true if consumed.
func (pv *PlayerView) HandleKey(key string) bool {
	if pv.Loaded && pv.Player != nil {
		switch key {
		case " ":
			pv.Player.Paused = !pv.Player.Paused
			return true
		case "l":
			pv.Player.SetLoop(!pv.Player.Loop)
			return true
		case "esc":
			pv.Loaded = false
			pv.Player = nil
			return true
		}
		return false
	}

	// File list mode
	switch key {
	case "up", "k":
		if len(pv.files) > 0 {
			pv.selectedIdx--
			if pv.selectedIdx < 0 {
				pv.selectedIdx = len(pv.files) - 1
			}
			pv.scrollView.EnsureLineVisible(pv.selectedIdx)
		}
		return true
	case "down", "j":
		if len(pv.files) > 0 {
			pv.selectedIdx++
			if pv.selectedIdx >= len(pv.files) {
				pv.selectedIdx = 0
			}
			pv.scrollView.EnsureLineVisible(pv.selectedIdx)
		}
		return true
	case "enter":
		if len(pv.files) > 0 && pv.selectedIdx < len(pv.files) {
			pv.loadFile(pv.files[pv.selectedIdx])
		}
		return true
	}
	return false
}

// Tick advances playback by dt seconds.
func (pv *PlayerView) Tick(dt float64) {
	if pv.Loaded && pv.Player != nil {
		pv.Player.Tick(dt)
	}
}

// loadFile loads a clip file and enters playback mode.
func (pv *PlayerView) loadFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	c, err := clip.LoadClipFromBytes(data)
	if err != nil {
		return
	}
	pv.Player = clip.NewPlayer(c)
	pv.Player.SetLoop(true)
	pv.Player.SetSize(pv.width, pv.height-1)
	pv.Loaded = true
}

// Render returns the player view content as a string.
func (pv *PlayerView) Render(width, height int) string {
	pv.width = width
	pv.height = height

	if pv.Loaded && pv.Player != nil {
		return pv.renderPlayback(width, height)
	}
	return pv.renderFileList(width, height)
}

func (pv *PlayerView) renderPlayback(width, height int) string {
	pv.Player.SetSize(width, height-1)
	frame := pv.Player.Render()

	// Pad frame to fill viewport
	lines := strings.Split(frame, "\n")
	for len(lines) < height-1 {
		lines = append(lines, strings.Repeat(" ", width))
	}
	if len(lines) > height-1 {
		lines = lines[:height-1]
	}

	// Status line
	pauseStr := ""
	if pv.Player.Paused {
		pauseStr = " PAUSED"
	}
	loopStr := ""
	if pv.Player.Loop {
		loopStr = " LOOP"
	}
	track := pv.Player.Clip().Tracks[pv.Player.ScaleIdx]
	status := fmt.Sprintf(" Frame %d/%d%s%s  |  space: pause  l: loop  esc: back",
		pv.Player.CurrentFrame+1, len(track.Frames), pauseStr, loopStr)

	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("245")).
		Width(width)

	lines = append(lines, statusStyle.Render(status))
	return strings.Join(lines, "\n")
}

func (pv *PlayerView) renderFileList(width, height int) string {
	headerStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Bold(true)
	dimStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("243"))
	activeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("218")).
		Bold(true)
	normalStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252"))

	var lines []string
	lines = append(lines, headerStyle.Render(pad(" Recordings", width)))
	lines = append(lines, dimStyle.Render(pad(" ───────────────────────────", width)))
	lines = append(lines, "")

	if len(pv.files) == 0 {
		lines = append(lines, dimStyle.Render(pad("  No .asciirec files found", width)))
		lines = append(lines, dimStyle.Render(pad("  Record a clip with 'o' in Shader view", width)))
	} else {
		for i, f := range pv.files {
			label := fmt.Sprintf("  %s", f)
			label = pad(label, width)
			if i == pv.selectedIdx {
				label = activeStyle.Render(label)
			} else {
				label = normalStyle.Render(label)
			}
			lines = append(lines, label)
		}
	}

	content := strings.Join(lines, "\n")
	pv.scrollView.SetSize(width, height)
	return pv.scrollView.RenderContent(content)
}
