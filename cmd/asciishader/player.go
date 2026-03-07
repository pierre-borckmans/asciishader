package main

import (
	"fmt"
	"os"
	"time"

	"asciishader/pkg/clip"
	"asciishader/tui/app"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func runPlayer(path string) {
	c, err := clip.LoadClip(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading clip: %v\n", err)
		os.Exit(1)
	}

	player := clip.NewPlayer(c)
	player.SetLoop(true)

	pm := playerModel{
		player:    player,
		clipPath:  path,
		lastFrame: time.Now(),
	}

	p := tea.NewProgram(pm)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

type playerModel struct {
	player    *clip.Player
	clipPath  string
	width     int
	height    int
	lastFrame time.Time
}

func (pm playerModel) Init() tea.Cmd {
	return app.Tick()
}

func (pm playerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		pm.width = msg.Width
		pm.height = msg.Height
		pm.player.SetSize(pm.width, pm.height-1)
		return pm, nil

	case app.TickMsg:
		now := time.Now()
		dt := now.Sub(pm.lastFrame).Seconds()
		if dt < 1.0/63.0 {
			return pm, app.Tick()
		}
		pm.lastFrame = now
		pm.player.Tick(dt)
		return pm, app.Tick()

	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return pm, tea.Quit
		case "space":
			pm.player.Paused = !pm.player.Paused
		case "l":
			pm.player.SetLoop(!pm.player.Loop)
		}
		return pm, nil
	}
	return pm, nil
}

func (pm playerModel) View() tea.View {
	if pm.width == 0 || pm.height == 0 {
		v := tea.NewView("Loading...")
		v.AltScreen = true
		return v
	}

	frame := pm.player.Render()

	status := fmt.Sprintf(" Playing: %s | Frame %d/%d | q: quit | space: pause | l: loop",
		pm.clipPath, pm.player.CurrentFrame+1, len(pm.player.Clip().Frames))

	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#333333")).
		Foreground(lipgloss.Color("#CCCCCC")).
		Width(pm.width)

	v := tea.NewView(frame + "\n" + statusStyle.Render(status))
	v.AltScreen = true
	return v
}
