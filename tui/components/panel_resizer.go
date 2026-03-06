package components

import (
	tea "charm.land/bubbletea/v2"
)

// PanelResizeAxis indicates which axis a panel resizes along.
type PanelResizeAxis int

const (
	// ResizeHorizontal resizes by changing width (right panel: drag left edge).
	ResizeHorizontal PanelResizeAxis = iota
	// ResizeVertical resizes by changing height (bottom panel: drag top edge).
	ResizeVertical
)

// PanelResizer handles drag-to-resize for a panel edge.
type PanelResizer struct {
	axis PanelResizeAxis

	// Edge position in screen coordinates (X for horizontal)
	edgePos int

	// Size limits
	minSize int

	// Drag state
	dragging    bool
	dragStartAt int
	sizeAtStart int

	// Hover state
	hovering bool
}

// NewPanelResizer creates a new resizer for the given axis.
func NewPanelResizer(axis PanelResizeAxis, minSize int) *PanelResizer {
	return &PanelResizer{
		axis:    axis,
		minSize: minSize,
	}
}

// SetEdgePos sets the screen coordinate of the draggable edge.
func (pr *PanelResizer) SetEdgePos(pos int) {
	pr.edgePos = pos
}

// IsDragging returns whether a resize drag is in progress.
func (pr *PanelResizer) IsDragging() bool {
	return pr.dragging
}

// IsHovering returns whether the mouse is hovering over the edge.
func (pr *PanelResizer) IsHovering() bool {
	return pr.hovering
}

// HandleMouse processes a mouse event for resize interaction.
// Returns (newSize, handled).
func (pr *PanelResizer) HandleMouse(msg tea.MouseMsg, currentSize int, screenSize int) (int, bool) {
	maxSize := screenSize / 2

	mouse := msg.Mouse()
	// Use X for horizontal, Y for vertical
	mousePos := mouse.X
	if pr.axis == ResizeVertical {
		mousePos = mouse.Y
	}

	onEdge := mousePos == pr.edgePos

	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft && onEdge {
			pr.dragging = true
			pr.dragStartAt = mousePos
			pr.sizeAtStart = currentSize
			return currentSize, true
		}

	case tea.MouseMotionMsg:
		oldHover := pr.hovering
		pr.hovering = onEdge || pr.dragging

		if pr.dragging {
			delta := mousePos - pr.dragStartAt
			// For horizontal: drag left = increase width (negative delta)
			// For vertical: drag up = increase height (negative delta)
			newSize := pr.sizeAtStart - delta
			newSize = clampInt(newSize, pr.minSize, maxSize)
			return newSize, true
		}

		if pr.hovering != oldHover {
			return currentSize, true
		}

	case tea.MouseReleaseMsg:
		if pr.dragging {
			delta := mousePos - pr.dragStartAt
			newSize := pr.sizeAtStart - delta
			newSize = clampInt(newSize, pr.minSize, maxSize)
			pr.dragging = false
			return newSize, true
		}
	}

	if !onEdge && !pr.dragging && pr.hovering {
		pr.hovering = false
		return currentSize, true
	}

	return currentSize, false
}

func clampInt(size, min, max int) int {
	if size < min {
		return min
	}
	if size > max {
		return max
	}
	return size
}
