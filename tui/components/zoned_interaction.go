package components

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

// DoubleClickThreshold is the maximum time between clicks to count as double-click
const DoubleClickThreshold = 400 * time.Millisecond

// ZonedInteraction handles mouse interactions (hover, click, double-click) for zoned elements.
type ZonedInteraction struct {
	prefix        string    // Zone ID prefix to avoid collisions between components
	hoveredID     string    // Currently hovered item ID
	lastClickTime time.Time // Time of last click for double-click detection
	lastClickID   string    // ID of last clicked item for double-click detection
}

// NewZonedInteraction creates a new ZonedInteraction with the given prefix.
func NewZonedInteraction(prefix string) *ZonedInteraction {
	return &ZonedInteraction{
		prefix:        prefix,
		hoveredID:     "",
		lastClickTime: time.Time{},
		lastClickID:   "",
	}
}

// ZoneID returns the full zone ID for an item.
func (z *ZonedInteraction) ZoneID(id string) string {
	return fmt.Sprintf("%s-%s", z.prefix, id)
}

// MouseResult contains the result of processing a mouse event.
type MouseResult struct {
	Clicked       string // ID of item that was single-clicked (empty if none)
	DoubleClicked string // ID of item that was double-clicked (empty if none)
	HoverChanged  bool   // True if hover state changed
}

// HandleMouse processes a mouse event and returns what actions occurred.
// zoneIDs should be the list of item IDs that are currently visible/clickable.
func (z *ZonedInteraction) HandleMouse(msg tea.MouseMsg, zoneIDs []string) MouseResult {
	var result MouseResult
	mouse := msg.Mouse()

	// Handle hover (mouse motion)
	if _, ok := msg.(tea.MouseMotionMsg); ok {
		newHovered := ""
		for _, id := range zoneIDs {
			if zone.Get(z.ZoneID(id)).InBounds(msg) {
				newHovered = id
				break
			}
		}
		if newHovered != z.hoveredID {
			z.hoveredID = newHovered
			result.HoverChanged = true
		}
		return result
	}

	// Handle click
	if _, ok := msg.(tea.MouseReleaseMsg); ok && mouse.Button == tea.MouseLeft {
		for _, id := range zoneIDs {
			if zone.Get(z.ZoneID(id)).InBounds(msg) {
				now := time.Now()
				isDoubleClick := z.lastClickID == id && now.Sub(z.lastClickTime) < DoubleClickThreshold
				z.lastClickTime = now
				z.lastClickID = id

				if isDoubleClick {
					result.DoubleClicked = id
				} else {
					result.Clicked = id
				}
				return result
			}
		}
	}

	return result
}

// Mark wraps content in a zone marker for the given item ID.
func (z *ZonedInteraction) Mark(id, content string) string {
	return zone.Mark(z.ZoneID(id), content)
}

// IsHovered returns true if the given item ID is currently hovered.
func (z *ZonedInteraction) IsHovered(id string) bool {
	return z.hoveredID == id
}

// HoveredID returns the currently hovered item ID (empty string if none).
func (z *ZonedInteraction) HoveredID() string {
	return z.hoveredID
}

// ClearHover clears the current hover state.
func (z *ZonedInteraction) ClearHover() {
	z.hoveredID = ""
}

// HandleMouseCoords processes a mouse event using a caller-provided hit test function
// instead of bubblezone markers. This is useful when zone markers don't survive
// lipgloss composition functions (e.g. JoinHorizontal).
// hitTest should return the zone ID at the given screen coordinates, or "" if none.
func (z *ZonedInteraction) HandleMouseCoords(msg tea.MouseMsg, hitTest func(x, y int) string) MouseResult {
	var result MouseResult
	mouse := msg.Mouse()

	if _, ok := msg.(tea.MouseMotionMsg); ok {
		newHovered := hitTest(mouse.X, mouse.Y)
		if newHovered != z.hoveredID {
			z.hoveredID = newHovered
			result.HoverChanged = true
		}
		return result
	}

	if _, ok := msg.(tea.MouseReleaseMsg); ok && mouse.Button == tea.MouseLeft {
		id := hitTest(mouse.X, mouse.Y)
		if id != "" {
			now := time.Now()
			isDoubleClick := z.lastClickID == id && now.Sub(z.lastClickTime) < DoubleClickThreshold
			z.lastClickTime = now
			z.lastClickID = id

			if isDoubleClick {
				result.DoubleClicked = id
			} else {
				result.Clicked = id
			}
		}
	}

	return result
}
