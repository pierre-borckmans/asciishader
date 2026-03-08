package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
)

const treeIndent = 2

// TreeNode represents a node in a tree structure with lazy-loaded children.
type TreeNode struct {
	Label    string
	Detail   string            // optional text after label (rendered dimmed)
	Children func() []TreeNode // nil = leaf; called lazily on first expand
	Data     interface{}       // opaque caller data (e.g., AST source spans)

	Scaffold  bool      // render as scaffold (+) node with dimmed style
	Editable  bool      // allow inline editing on Enter
	EditValue string    // initial value for inline editing
	Color     *[3]uint8 // non-nil: render color swatch ██; use color picker on edit
}

// TreeView is a collapsible tree viewer component with keyboard and mouse support.
//
// Nodes start collapsed and expand on demand. The expand/collapse state is
// preserved across calls to SetRoots, allowing the tree to be rebuilt (e.g.
// after recompilation) without losing the user's navigation context.
type TreeView struct {
	roots        []TreeNode
	flat         []flatEntry
	cursor       int
	expanded     map[string]bool
	width        int
	height       int
	scrollOffset int

	// Zone-based mouse interaction (replaces manual screenX/screenY math)
	zoned *ZonedInteraction

	// OnSelect is called when the user activates a node (Enter key or click on leaf).
	OnSelect func(node TreeNode)

	// Inline editing state
	editing bool
	editBuf []rune
	editPos int

	// Color picker (non-nil when active; component handles its own keys/render)
	colorPicker *ColorPicker

	// EditResult is set when the user confirms an inline edit. The caller
	// should consume it (set to nil) and apply the change.
	EditResult *EditResult

	// ScaffoldResult is set when the user activates a scaffold node. The
	// caller should consume it (set to nil) and insert the template.
	ScaffoldResult *TreeNode
}

// EditResult holds the result of a confirmed inline edit.
type EditResult struct {
	Node     TreeNode
	NewValue string
}

type flatEntry struct {
	node        TreeNode
	depth       int
	hasChildren bool
	childCache  []TreeNode // populated on expand
	path        string     // unique path for state persistence
}

// NewTreeView creates a new empty tree view.
func NewTreeView() *TreeView {
	return &TreeView{
		expanded: make(map[string]bool),
		width:    40,
		height:   20,
		zoned:    NewZonedInteraction("tree"),
	}
}

// SetRoots replaces the tree content, preserving expand/collapse state.
func (tv *TreeView) SetRoots(roots []TreeNode) {
	tv.roots = roots
	tv.rebuild()
}

// SetSize sets the visible area dimensions.
func (tv *TreeView) SetSize(width, height int) {
	tv.width = width
	tv.height = height
	tv.clampScroll()
}

// SelectedNode returns the currently selected node, or nil if the tree is empty.
func (tv *TreeView) SelectedNode() *TreeNode {
	if tv.cursor < 0 || tv.cursor >= len(tv.flat) {
		return nil
	}
	n := tv.flat[tv.cursor].node
	return &n
}

// Cursor returns the cursor index in the flattened visible list.
func (tv *TreeView) Cursor() int {
	return tv.cursor
}

// Len returns the number of visible (flattened) entries.
func (tv *TreeView) Len() int {
	return len(tv.flat)
}

// rebuild regenerates the flat list from roots, preserving expand state and cursor.
func (tv *TreeView) rebuild() {
	var cursorPath string
	if tv.cursor >= 0 && tv.cursor < len(tv.flat) {
		cursorPath = tv.flat[tv.cursor].path
	}

	tv.flat = tv.flat[:0]
	for i, root := range tv.roots {
		tv.buildFlat(root, 0, "", i)
	}

	// Restore cursor position by path
	tv.cursor = 0
	if cursorPath != "" {
		for i, e := range tv.flat {
			if e.path == cursorPath {
				tv.cursor = i
				break
			}
		}
	}
	if tv.cursor >= len(tv.flat) {
		tv.cursor = len(tv.flat) - 1
	}
	if tv.cursor < 0 {
		tv.cursor = 0
	}
	tv.clampScroll()
}

func (tv *TreeView) buildFlat(node TreeNode, depth int, parentPath string, sibIdx int) {
	path := fmt.Sprintf("%s/%d:%s", parentPath, sibIdx, node.Label)
	hasChildren := node.Children != nil

	entry := flatEntry{
		node:        node,
		depth:       depth,
		hasChildren: hasChildren,
		path:        path,
	}

	// Load children for previously-expanded nodes
	if hasChildren && tv.expanded[path] {
		entry.childCache = node.Children()
	}

	tv.flat = append(tv.flat, entry)

	if entry.childCache != nil {
		for i, child := range entry.childCache {
			tv.buildFlat(child, depth+1, path, i)
		}
	}
}

// IsEditing returns true if the tree is in inline edit or color picker mode.
func (tv *TreeView) IsEditing() bool {
	return tv.editing || tv.colorPicker != nil
}

// CancelEdit exits edit mode without applying changes.
func (tv *TreeView) CancelEdit() {
	tv.editing = false
	tv.editBuf = nil
	tv.colorPicker = nil
}

func (tv *TreeView) startEdit() {
	if tv.cursor < 0 || tv.cursor >= len(tv.flat) {
		return
	}
	node := tv.flat[tv.cursor].node
	tv.editing = true
	tv.editBuf = []rune(node.EditValue)
	tv.editPos = len(tv.editBuf)
}

func (tv *TreeView) confirmEdit() {
	if tv.cursor < 0 || tv.cursor >= len(tv.flat) {
		tv.editing = false
		return
	}
	node := tv.flat[tv.cursor].node
	newVal := string(tv.editBuf)
	if newVal != node.EditValue {
		tv.EditResult = &EditResult{Node: node, NewValue: newVal}
	}
	tv.editing = false
	tv.editBuf = nil
}

func (tv *TreeView) startColorPicker() {
	if tv.cursor < 0 || tv.cursor >= len(tv.flat) {
		return
	}
	node := tv.flat[tv.cursor].node
	if node.Color == nil {
		return
	}
	tv.colorPicker = NewColorPicker(node.Color[0], node.Color[1], node.Color[2])
}

func (tv *TreeView) handleColorPickerKey(key string) bool {
	if tv.colorPicker == nil {
		return false
	}
	action := tv.colorPicker.HandleKey(key)
	switch action {
	case ColorPickerConfirm:
		if tv.cursor >= 0 && tv.cursor < len(tv.flat) {
			node := tv.flat[tv.cursor].node
			newVal := tv.colorPicker.HexString()
			if newVal != node.EditValue {
				tv.EditResult = &EditResult{Node: node, NewValue: newVal}
			}
		}
		tv.colorPicker = nil
	case ColorPickerCancel:
		tv.colorPicker = nil
	}
	return true
}

// HandleColorPickerMouse routes mouse events to the active color picker.
// screenWidth and screenHeight are used to compute the centered overlay position.
// Returns true if the event was consumed (picker handled it or it was dismissed).
func (tv *TreeView) HandleColorPickerMouse(msg tea.MouseMsg, screenWidth, screenHeight int) bool {
	if tv.colorPicker == nil {
		return false
	}

	pickerX := (screenWidth - tv.colorPicker.Width()) / 2
	pickerY := (screenHeight - tv.colorPicker.Height()) / 2

	action := tv.colorPicker.HandleMouse(msg, pickerX, pickerY)
	switch action {
	case ColorPickerMouseMiss:
		// Click outside the picker dismisses it
		if _, ok := msg.(tea.MouseClickMsg); ok {
			tv.colorPicker = nil
			return true
		}
		return false
	default:
		return true
	}
}

func (tv *TreeView) handleEditKey(key string) bool {
	switch key {
	case "enter":
		tv.confirmEdit()
	case "esc":
		tv.CancelEdit()
	case "left":
		if tv.editPos > 0 {
			tv.editPos--
		}
	case "right":
		if tv.editPos < len(tv.editBuf) {
			tv.editPos++
		}
	case "home", "ctrl+a":
		tv.editPos = 0
	case "end", "ctrl+e":
		tv.editPos = len(tv.editBuf)
	case "backspace":
		if tv.editPos > 0 {
			tv.editBuf = append(tv.editBuf[:tv.editPos-1], tv.editBuf[tv.editPos:]...)
			tv.editPos--
		}
	case "delete":
		if tv.editPos < len(tv.editBuf) {
			tv.editBuf = append(tv.editBuf[:tv.editPos], tv.editBuf[tv.editPos+1:]...)
		}
	default:
		// Insert printable characters
		if len(key) == 1 && key[0] >= ' ' && key[0] <= '~' {
			tv.editBuf = append(tv.editBuf[:tv.editPos], append([]rune{rune(key[0])}, tv.editBuf[tv.editPos:]...)...)
			tv.editPos++
		}
	}
	return true
}

// HandleKey processes a key press. Returns true if consumed.
func (tv *TreeView) HandleKey(key string) bool {
	if len(tv.flat) == 0 {
		return false
	}

	if tv.colorPicker != nil {
		return tv.handleColorPickerKey(key)
	}
	if tv.editing {
		return tv.handleEditKey(key)
	}

	switch key {
	case "up", "k":
		if tv.cursor > 0 {
			tv.cursor--
			tv.ensureCursorVisible()
		}
		return true

	case "down", "j":
		if tv.cursor < len(tv.flat)-1 {
			tv.cursor++
			tv.ensureCursorVisible()
		}
		return true

	case "right", "l":
		e := tv.flat[tv.cursor]
		if e.hasChildren && !tv.expanded[e.path] {
			tv.expandNode(tv.cursor)
		} else if e.hasChildren && tv.cursor+1 < len(tv.flat) {
			// Already expanded — move to first child
			tv.cursor++
			tv.ensureCursorVisible()
		}
		return true

	case "left", "h":
		e := tv.flat[tv.cursor]
		if e.hasChildren && tv.expanded[e.path] {
			tv.collapseNode(tv.cursor)
		} else {
			tv.moveToParent()
		}
		return true

	case "enter":
		if tv.cursor >= 0 && tv.cursor < len(tv.flat) {
			e := tv.flat[tv.cursor]
			node := e.node
			// Editable leaf: enter edit mode (color picker for color nodes)
			if node.Editable && !e.hasChildren {
				if node.Color != nil {
					tv.startColorPicker()
				} else {
					tv.startEdit()
				}
				return true
			}
			// Scaffold node: signal insertion
			if node.Scaffold {
				nodeCopy := node
				tv.ScaffoldResult = &nodeCopy
				return true
			}
			if e.hasChildren {
				if tv.expanded[e.path] {
					tv.collapseNode(tv.cursor)
				} else {
					tv.expandNode(tv.cursor)
				}
			}
			if tv.OnSelect != nil {
				tv.OnSelect(node)
			}
		}
		return true
	}

	return false
}

// HandleMouse processes a mouse event. Returns true if consumed.
// Row detection uses bubblezone markers (set during Render) instead of manual
// screen-coordinate math, so the caller no longer needs to call SetPosition.
func (tv *TreeView) HandleMouse(msg tea.MouseMsg) bool {
	if len(tv.flat) == 0 {
		return false
	}

	// Only handle events within the tree area
	if !tv.zoned.InBounds("body", msg) {
		return false
	}

	// Cancel inline editing on any click (color picker is handled separately
	// as a modal overlay in HandleColorPickerMouse)
	if tv.editing {
		if _, ok := msg.(tea.MouseReleaseMsg); ok {
			tv.CancelEdit()
			return true
		}
	}

	mouse := msg.Mouse()

	// Scroll wheel
	if _, ok := msg.(tea.MouseWheelMsg); ok {
		switch mouse.Button {
		case tea.MouseWheelUp:
			if tv.scrollOffset > 0 {
				tv.scrollOffset--
				return true
			}
		case tea.MouseWheelDown:
			if tv.scrollOffset < tv.maxScroll() {
				tv.scrollOffset++
				return true
			}
		}
		return false
	}

	// Click detection via zone markers on each row
	visibleEnd := tv.scrollOffset + tv.height
	if visibleEnd > len(tv.flat) {
		visibleEnd = len(tv.flat)
	}
	zoneIDs := make([]string, 0, visibleEnd-tv.scrollOffset)
	for i := tv.scrollOffset; i < visibleEnd; i++ {
		zoneIDs = append(zoneIDs, fmt.Sprintf("row-%d", i))
	}

	result := tv.zoned.HandleMouse(msg, zoneIDs)

	clicked := result.Clicked
	if clicked == "" {
		clicked = result.DoubleClicked
	}
	if clicked == "" {
		return result.HoverChanged
	}

	// Parse row index from zone ID
	var rowIdx int
	if _, err := fmt.Sscanf(clicked, "row-%d", &rowIdx); err != nil {
		return false
	}
	if rowIdx < 0 || rowIdx >= len(tv.flat) {
		return false
	}

	tv.cursor = rowIdx
	e := tv.flat[tv.cursor]

	if e.node.Scaffold {
		nodeCopy := e.node
		tv.ScaffoldResult = &nodeCopy
		return true
	}
	if e.hasChildren {
		if tv.expanded[e.path] {
			tv.collapseNode(tv.cursor)
		} else {
			tv.expandNode(tv.cursor)
		}
	} else if tv.OnSelect != nil {
		tv.OnSelect(e.node)
	}
	return true
}

func (tv *TreeView) expandNode(idx int) {
	e := tv.flat[idx]
	tv.expanded[e.path] = true
	tv.rebuild()
}

func (tv *TreeView) collapseNode(idx int) {
	e := tv.flat[idx]
	delete(tv.expanded, e.path)
	tv.rebuild()
}

func (tv *TreeView) moveToParent() {
	if tv.cursor <= 0 {
		return
	}
	myDepth := tv.flat[tv.cursor].depth
	for i := tv.cursor - 1; i >= 0; i-- {
		if tv.flat[i].depth < myDepth {
			tv.cursor = i
			tv.ensureCursorVisible()
			return
		}
	}
}

func (tv *TreeView) ensureCursorVisible() {
	if tv.cursor < tv.scrollOffset {
		tv.scrollOffset = tv.cursor
	} else if tv.cursor >= tv.scrollOffset+tv.height {
		tv.scrollOffset = tv.cursor - tv.height + 1
	}
	tv.clampScroll()
}

func (tv *TreeView) maxScroll() int {
	m := len(tv.flat) - tv.height
	if m < 0 {
		return 0
	}
	return m
}

func (tv *TreeView) clampScroll() {
	if tv.scrollOffset < 0 {
		tv.scrollOffset = 0
	}
	m := tv.maxScroll()
	if tv.scrollOffset > m {
		tv.scrollOffset = m
	}
}

// Render returns the tree view as a styled string.
func (tv *TreeView) Render() string {
	if len(tv.flat) == 0 {
		return "\x1b[38;5;243m (empty)\x1b[0m"
	}

	treeHeight := tv.height

	visibleEnd := tv.scrollOffset + treeHeight
	if visibleEnd > len(tv.flat) {
		visibleEnd = len(tv.flat)
	}

	hasScrollbar := len(tv.flat) > treeHeight
	contentWidth := tv.width
	if hasScrollbar {
		contentWidth -= 2 // gap + scrollbar character
	}
	if contentWidth < 1 {
		contentWidth = 1
	}

	var b strings.Builder
	for i := tv.scrollOffset; i < visibleEnd; i++ {
		if i > tv.scrollOffset {
			b.WriteByte('\n')
		}
		e := tv.flat[i]

		indent := strings.Repeat(" ", e.depth*treeIndent)
		arrow := " "
		if e.hasChildren {
			if tv.expanded[e.path] {
				arrow = "▾"
			} else {
				arrow = "▸"
			}
		}

		// Build row content into a separate buffer for zone wrapping
		var row strings.Builder

		if tv.editing && i == tv.cursor {
			// Edit mode: show label + editable value with cursor
			prefix := fmt.Sprintf(" %s  %s ", indent, e.node.Label)
			row.WriteString(tv.renderEditRow(prefix, contentWidth))
		} else if e.node.Scaffold {
			// Scaffold node: dimmed with + prefix
			plain := fmt.Sprintf(" %s+ %s", indent, e.node.Label)
			if i == tv.cursor {
				padded := TruncateOrPadLine(plain, contentWidth)
				fmt.Fprintf(&row, "\x1b[97;48;5;240m%s\x1b[0m", padded)
			} else {
				padded := TruncateOrPadLine(plain, contentWidth)
				fmt.Fprintf(&row, "\x1b[38;5;239m%s\x1b[0m", padded)
			}
		} else if i == tv.cursor {
			// Cursor row with highlight
			if e.node.Color != nil {
				c := e.node.Color
				// Swatch uses true color fg; rest uses cursor highlight
				line := fmt.Sprintf(" %s%s %s \x1b[38;2;%d;%d;%dm██\x1b[97m %s",
					indent, arrow, e.node.Label, c[0], c[1], c[2], e.node.Detail)
				padded := TruncateOrPadLine(line, contentWidth)
				fmt.Fprintf(&row, "\x1b[97;48;5;240m%s\x1b[0m", padded)
			} else {
				plain := fmt.Sprintf(" %s%s %s", indent, arrow, e.node.Label)
				if e.node.Detail != "" {
					plain += " " + e.node.Detail
				}
				padded := TruncateOrPadLine(plain, contentWidth)
				fmt.Fprintf(&row, "\x1b[97;48;5;240m%s\x1b[0m", padded)
			}
		} else {
			// Normal row
			if e.node.Color != nil {
				c := e.node.Color
				line := fmt.Sprintf(" %s%s %s \x1b[38;2;%d;%d;%dm██\x1b[0m \x1b[38;5;243m%s\x1b[0m",
					indent, arrow, e.node.Label, c[0], c[1], c[2], e.node.Detail)
				row.WriteString(TruncateOrPadLine(line, contentWidth))
			} else if e.node.Detail != "" {
				line := fmt.Sprintf(" %s%s %s \x1b[38;5;243m%s\x1b[0m", indent, arrow, e.node.Label, e.node.Detail)
				row.WriteString(TruncateOrPadLine(line, contentWidth))
			} else {
				line := fmt.Sprintf(" %s%s %s", indent, arrow, e.node.Label)
				row.WriteString(TruncateOrPadLine(line, contentWidth))
			}
		}

		if hasScrollbar {
			row.WriteString(" ")
			row.WriteString(tv.scrollbarChar(i - tv.scrollOffset))
		}

		// Wrap row in a zone marker for click detection
		rowID := fmt.Sprintf("row-%d", i)
		b.WriteString(tv.zoned.Mark(rowID, row.String()))
	}

	// Pad remaining tree rows
	for i := visibleEnd - tv.scrollOffset; i < treeHeight; i++ {
		b.WriteByte('\n')
		b.WriteString(strings.Repeat(" ", contentWidth))
		if hasScrollbar {
			b.WriteString("  ")
		}
	}

	// Wrap entire tree in a body zone for scroll detection
	return tv.zoned.Mark("body", b.String())
}

func (tv *TreeView) renderEditRow(prefix string, width int) string {
	var buf strings.Builder
	buf.WriteString("\x1b[93m") // yellow for edit mode

	buf.WriteString(prefix)

	// Render edit buffer with block cursor
	pos := tv.editPos
	if pos > len(tv.editBuf) {
		pos = len(tv.editBuf)
	}
	buf.WriteString(string(tv.editBuf[:pos]))
	buf.WriteString("\x1b[7m") // reverse video for cursor
	if pos < len(tv.editBuf) {
		buf.WriteRune(tv.editBuf[pos])
	} else {
		buf.WriteByte(' ') // cursor at end
	}
	buf.WriteString("\x1b[27m") // end reverse
	if pos < len(tv.editBuf) {
		buf.WriteString(string(tv.editBuf[pos+1:]))
	}

	buf.WriteString("\x1b[0m")
	return TruncateOrPadLine(buf.String(), width)
}

// ActiveColorPicker returns the currently active color picker, or nil.
// The caller can use this to render the picker as a floating overlay.
func (tv *TreeView) ActiveColorPicker() *ColorPicker {
	return tv.colorPicker
}

func (tv *TreeView) scrollbarChar(row int) string {
	trackHeight := tv.height
	if trackHeight <= 0 || len(tv.flat) <= trackHeight {
		return " "
	}
	maxScr := tv.maxScroll()
	thumbPos := 0
	if maxScr > 0 && trackHeight > 1 {
		thumbPos = (tv.scrollOffset * (trackHeight - 1)) / maxScr
	}

	if row < thumbPos {
		return "\x1b[38;5;39m┃\x1b[0m"
	} else if row == thumbPos {
		return "\x1b[38;5;39m●\x1b[0m"
	}
	return "\x1b[38;5;39m│\x1b[0m"
}
