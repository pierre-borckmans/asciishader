package components

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// PanelAnimTickMsg signals that a panel animation should advance one frame.
type PanelAnimTickMsg struct {
	ID string
}

// PanelAnimator provides reusable expand/collapse animation for any panel.
type PanelAnimator struct {
	id    string
	steps int

	animating bool
	current   int
	start     int
	target    int
	step      int
}

// NewPanelAnimator creates a new animator with the given ID and step count.
func NewPanelAnimator(id string, steps int) *PanelAnimator {
	if steps < 1 {
		steps = 6
	}
	return &PanelAnimator{
		id:    id,
		steps: steps,
	}
}

// Start begins an animation from startVal to targetVal.
func (pa *PanelAnimator) Start(startVal, targetVal int) tea.Cmd {
	pa.animating = true
	pa.start = startVal
	pa.target = targetVal
	pa.current = startVal
	pa.step = 0
	return pa.scheduleNextTick()
}

// Tick advances the animation by one frame.
func (pa *PanelAnimator) Tick() tea.Cmd {
	if !pa.animating {
		return nil
	}

	pa.step++
	if pa.step >= pa.steps {
		pa.animating = false
		pa.current = pa.target
		return nil
	}

	// Quadratic ease-out
	t := float64(pa.step) / float64(pa.steps)
	t = 1 - (1-t)*(1-t)
	pa.current = pa.start + int(t*float64(pa.target-pa.start))

	return pa.scheduleNextTick()
}

// Animating returns whether an animation is currently in progress.
func (pa *PanelAnimator) Animating() bool {
	return pa.animating
}

// Value returns the current animated value.
func (pa *PanelAnimator) Value() int {
	return pa.current
}

// Stop cancels any in-progress animation and snaps to the target value.
func (pa *PanelAnimator) Stop() {
	if pa.animating {
		pa.animating = false
		pa.current = pa.target
	}
}

func (pa *PanelAnimator) scheduleNextTick() tea.Cmd {
	id := pa.id
	return tea.Tick(16*time.Millisecond, func(_ time.Time) tea.Msg {
		return PanelAnimTickMsg{ID: id}
	})
}
