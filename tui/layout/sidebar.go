package layout

import (
	"strings"

	"asciishader/tui/components"
	"asciishader/tui/styles"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// SidebarItem describes a navigable item in the sidebar.
type SidebarItem struct {
	ID   string
	Icon string
	Name string
}

// Sidebar renders a vertical navigation panel on the left side.
type Sidebar struct {
	items    []SidebarItem
	activeID string
	expanded bool

	// Computed widths
	expandedWidth  int
	collapsedWidth int

	// Animation
	animator *components.PanelAnimator

	// Mouse interaction
	zoned *components.ZonedInteraction
}

// Sidebar styles
var (
	sidebarBg = styles.ChromeBgLight

	sidebarActiveIndicator = lipgloss.NewStyle().
				Foreground(lipgloss.Color("218")). // Soft vivid pink
				Background(sidebarBg)

	sidebarActiveText = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("218")).
				Background(sidebarBg)

	sidebarInactiveText = lipgloss.NewStyle().
				Foreground(styles.ChromeFgMuted).
				Background(sidebarBg)

	sidebarSepStyle = lipgloss.NewStyle().
			Foreground(sidebarBg).
			Background(sidebarBg)

	sidebarRowBg = lipgloss.NewStyle().
			Background(sidebarBg)
)

const sidebarEmojiWidth = 2

// NewSidebar creates a new sidebar.
func NewSidebar() *Sidebar {
	return &Sidebar{
		items:    []SidebarItem{},
		expanded: false,
		animator: components.NewPanelAnimator("sidebar", 6),
		zoned:    components.NewZonedInteraction("sidebar"),
	}
}

// SetItems replaces all sidebar items.
func (s *Sidebar) SetItems(items []SidebarItem) {
	s.items = make([]SidebarItem, len(items))
	copy(s.items, items)
	s.expandedWidth = 0
	s.collapsedWidth = 0
}

// SetActiveID sets the currently active item.
func (s *Sidebar) SetActiveID(id string) {
	s.activeID = id
}

// ActiveID returns the currently active item ID.
func (s *Sidebar) ActiveID() string {
	return s.activeID
}

// Items returns the sidebar items.
func (s *Sidebar) Items() []SidebarItem {
	return s.items
}

// ToggleExpanded toggles between expanded and collapsed states with animation.
func (s *Sidebar) ToggleExpanded() tea.Cmd {
	s.computeWidths()
	startWidth := s.Width()
	s.animator.Stop()
	s.expanded = !s.expanded
	targetWidth := s.fullWidth()
	return s.animator.Start(startWidth, targetWidth)
}

// AnimTick advances the sidebar animation by one frame.
func (s *Sidebar) AnimTick() tea.Cmd {
	return s.animator.Tick()
}

// IsExpanded returns whether the sidebar is expanded.
func (s *Sidebar) IsExpanded() bool {
	return s.expanded
}

// Animating returns whether the sidebar is currently animating.
func (s *Sidebar) Animating() bool {
	return s.animator.Animating()
}

// computeWidths calculates the expanded and collapsed inner widths.
func (s *Sidebar) computeWidths() {
	if s.expandedWidth > 0 {
		return
	}

	maxExpanded := 0
	maxCollapsed := 0
	for _, item := range s.items {
		expanded := 2 + sidebarEmojiWidth + 1 + lipgloss.Width(item.Name)
		if expanded > maxExpanded {
			maxExpanded = expanded
		}
		collapsed := 1 + sidebarEmojiWidth
		if collapsed > maxCollapsed {
			maxCollapsed = collapsed
		}
	}

	s.expandedWidth = maxExpanded + 3
	s.collapsedWidth = maxCollapsed + 1
}

// Width returns the total width of the sidebar including edge characters.
func (s *Sidebar) Width() int {
	if s.animator.Animating() {
		return s.animator.Value()
	}
	return s.fullWidth()
}

// fullWidth returns the non-animated width.
func (s *Sidebar) fullWidth() int {
	s.computeWidths()
	if s.expanded {
		return s.expandedWidth + 1 // +1 for right half block
	}
	return s.collapsedWidth + 2 // +2 for left and right half blocks
}

// innerWidth returns the content width without separator.
func (s *Sidebar) innerWidth() int {
	s.computeWidths()
	if s.expanded || s.animator.Animating() {
		return s.expandedWidth
	}
	return s.collapsedWidth
}

// renderExpanded returns true if we should render in expanded mode.
func (s *Sidebar) renderExpanded() bool {
	return s.expanded || s.animator.Animating()
}

// Render renders the sidebar with the given height.
func (s *Sidebar) Render(height int) string {
	if len(s.items) == 0 || height <= 0 {
		return ""
	}

	iw := s.innerWidth()
	expanded := s.renderExpanded()
	rightEdge := sidebarSepStyle.Render("█")
	leftEdge := ""
	if !expanded {
		leftEdge = sidebarSepStyle.Render("█")
	}

	rows := make([]string, 0, height)

	emptyRow := leftEdge + sidebarRowBg.Render(strings.Repeat(" ", iw)) + rightEdge

	// Toggle row at top
	toggleIcon := "«"
	if !expanded {
		toggleIcon = "»"
	}
	toggleStyle := lipgloss.NewStyle().Foreground(styles.ChromeFgMuted).Background(sidebarBg)
	if s.zoned.IsHovered("toggle") {
		toggleStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(sidebarBg)
	}

	{
		fillWidth := iw - 1
		margin := ""
		if expanded {
			margin = sidebarRowBg.Render(" ")
			fillWidth = iw - 2
		}
		if fillWidth < 0 {
			fillWidth = 0
		}
		toggleRow := leftEdge + margin + sidebarRowBg.Render(strings.Repeat(" ", fillWidth)) + toggleStyle.Render(toggleIcon) + rightEdge
		rows = append(rows, toggleRow)
	}

	// Item rows
	for _, item := range s.items {
		if len(rows) >= height {
			break
		}

		// Spacer
		rows = append(rows, emptyRow)
		if len(rows) >= height {
			break
		}

		// Item content row
		itemRow := leftEdge + s.renderItemRow(item, iw) + rightEdge
		rows = append(rows, itemRow)
	}

	// Trailing spacer
	if len(rows) < height {
		rows = append(rows, emptyRow)
	}

	// Fill remaining space
	for len(rows) < height {
		rows = append(rows, emptyRow)
	}

	// During animation, clip each row to the animated width
	if s.animator.Animating() {
		clipStyle := lipgloss.NewStyle().MaxWidth(s.animator.Value()).Background(sidebarBg)
		for i, row := range rows {
			rows[i] = clipStyle.Render(row)
		}
	}

	return strings.Join(rows, "\n")
}

// renderItemRow renders a single item row with indicator, icon, optional name.
func (s *Sidebar) renderItemRow(item SidebarItem, innerWidth int) string {
	expanded := s.renderExpanded()
	isActive := item.ID == s.activeID

	bg := sidebarRowBg

	// Active indicator
	activeIndicatorStyle := sidebarActiveIndicator
	var indicator string
	if isActive {
		if expanded {
			indicator = sidebarRowBg.Render(" ") + activeIndicatorStyle.Render("▎")
		} else {
			indicator = activeIndicatorStyle.Render("▎")
		}
	} else {
		if expanded {
			indicator = sidebarRowBg.Render(" ") + bg.Render(" ")
		} else {
			indicator = bg.Render(" ")
		}
	}

	// Text style
	var textStyle lipgloss.Style
	if isActive {
		textStyle = sidebarActiveText
	} else if s.zoned.IsHovered(item.ID) {
		textStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("252")).
			Background(sidebarBg)
	} else {
		textStyle = sidebarInactiveText
	}

	// Build content
	icon := item.Icon
	var content string
	if expanded {
		content = indicator + bg.Render(icon+" ") + textStyle.Render(item.Name)
	} else {
		content = indicator + bg.Render(icon)
	}

	// Pad to innerWidth
	contentWidth := lipgloss.Width(content)
	padding := innerWidth - contentWidth
	if padding < 0 {
		padding = 0
	}
	content += bg.Render(strings.Repeat(" ", padding))

	return content
}

// NextItem moves to the next item, returns the new active ID.
func (s *Sidebar) NextItem() string {
	if len(s.items) == 0 {
		return ""
	}
	for i, item := range s.items {
		if item.ID == s.activeID {
			next := (i + 1) % len(s.items)
			s.activeID = s.items[next].ID
			return s.activeID
		}
	}
	s.activeID = s.items[0].ID
	return s.activeID
}

// PrevItem moves to the previous item, returns the new active ID.
func (s *Sidebar) PrevItem() string {
	if len(s.items) == 0 {
		return ""
	}
	for i, item := range s.items {
		if item.ID == s.activeID {
			prev := (i - 1 + len(s.items)) % len(s.items)
			s.activeID = s.items[prev].ID
			return s.activeID
		}
	}
	s.activeID = s.items[0].ID
	return s.activeID
}

// SidebarMouseResult describes what happened from a sidebar mouse event.
type SidebarMouseResult struct {
	ToggleClicked bool   // The expand/collapse toggle was clicked
	ItemClicked   string // ID of the item that was clicked (empty if none)
	HoverChanged  bool   // Hover state changed (needs redraw)
}

// HandleMouse processes a mouse event using coordinate-based detection.
// offsetY is the screen Y coordinate where the sidebar starts (e.g. headerHeight).
func (s *Sidebar) HandleMouse(msg tea.MouseMsg, offsetY int) SidebarMouseResult {
	w := s.Width()
	hitTest := func(x, y int) string {
		if x < 0 || x >= w {
			return ""
		}
		relY := y - offsetY
		if relY < 0 {
			return ""
		}
		// Row 0 = toggle
		if relY == 0 {
			return "toggle"
		}
		// Items at rows 2, 4, 6, 8, ... (spacer + item pairs)
		for i, item := range s.items {
			if relY == 2+i*2 {
				return item.ID
			}
		}
		return ""
	}

	result := s.zoned.HandleMouseCoords(msg, hitTest)

	var out SidebarMouseResult
	out.HoverChanged = result.HoverChanged

	clicked := result.Clicked
	if clicked == "" {
		clicked = result.DoubleClicked
	}
	if clicked == "toggle" {
		out.ToggleClicked = true
	} else if clicked != "" {
		out.ItemClicked = clicked
	}
	return out
}

// IsHovered returns whether the given zone ID is hovered.
func (s *Sidebar) IsHovered(id string) bool {
	return s.zoned.IsHovered(id)
}

// ActiveTooltip returns a tooltip for the currently hovered sidebar item,
// or nil if nothing is hovered or the sidebar is expanded.
// offsetY is the screen Y where the sidebar content starts (header height).
func (s *Sidebar) ActiveTooltip(offsetY int) *components.Tooltip {
	if s.expanded || s.animator.Animating() {
		return nil
	}
	hovered := s.zoned.HoveredID()
	if hovered == "" || hovered == "toggle" {
		return nil
	}
	for i, item := range s.items {
		if item.ID == hovered {
			return &components.Tooltip{
				Text: item.Name,
				Row:  offsetY + 2 + i*2, // items are at rows 2, 4, 6, ...
				Col:  s.Width() + 1,
			}
		}
	}
	return nil
}

// ActiveIndex returns the index of the active item, or -1 if not found.
func (s *Sidebar) ActiveIndex() int {
	for i, item := range s.items {
		if item.ID == s.activeID {
			return i
		}
	}
	return -1
}
