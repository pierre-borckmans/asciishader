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
	screenX      int
	screenY      int
	scrollOffset int

	// OnSelect is called when the user activates a node (Enter key or click on leaf).
	OnSelect func(node TreeNode)
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

// SetPosition sets the screen coordinates of the top-left corner for mouse handling.
func (tv *TreeView) SetPosition(x, y int) {
	tv.screenX = x
	tv.screenY = y
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

// HandleKey processes a key press. Returns true if consumed.
func (tv *TreeView) HandleKey(key string) bool {
	if len(tv.flat) == 0 {
		return false
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
			node := tv.flat[tv.cursor].node
			e := tv.flat[tv.cursor]
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
func (tv *TreeView) HandleMouse(msg tea.MouseMsg) bool {
	if len(tv.flat) == 0 {
		return false
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

	// Click selects the row; toggles expand for branches, activates for leaves
	if _, ok := msg.(tea.MouseReleaseMsg); ok && mouse.Button == tea.MouseLeft {
		relY := mouse.Y - tv.screenY
		rowIdx := tv.scrollOffset + relY
		if relY >= 0 && rowIdx >= 0 && rowIdx < len(tv.flat) {
			tv.cursor = rowIdx
			e := tv.flat[tv.cursor]
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
	}

	return false
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

	visibleEnd := tv.scrollOffset + tv.height
	if visibleEnd > len(tv.flat) {
		visibleEnd = len(tv.flat)
	}

	hasScrollbar := len(tv.flat) > tv.height
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

		if i == tv.cursor {
			// Cursor row: uniform highlight, no inline ANSI
			plain := fmt.Sprintf(" %s%s %s", indent, arrow, e.node.Label)
			if e.node.Detail != "" {
				plain += " " + e.node.Detail
			}
			padded := TruncateOrPadLine(plain, contentWidth)
			fmt.Fprintf(&b, "\x1b[97;48;5;240m%s\x1b[0m", padded)
		} else {
			// Normal row with dimmed detail
			var line string
			if e.node.Detail != "" {
				line = fmt.Sprintf(" %s%s %s \x1b[38;5;243m%s\x1b[0m", indent, arrow, e.node.Label, e.node.Detail)
			} else {
				line = fmt.Sprintf(" %s%s %s", indent, arrow, e.node.Label)
			}
			b.WriteString(TruncateOrPadLine(line, contentWidth))
		}

		if hasScrollbar {
			b.WriteString(" ")
			b.WriteString(tv.scrollbarChar(i - tv.scrollOffset))
		}
	}

	// Pad remaining rows
	for i := visibleEnd - tv.scrollOffset; i < tv.height; i++ {
		b.WriteByte('\n')
		b.WriteString(strings.Repeat(" ", contentWidth))
		if hasScrollbar {
			b.WriteString("  ")
		}
	}

	return b.String()
}

func (tv *TreeView) scrollbarChar(row int) string {
	trackHeight := tv.height
	if trackHeight <= 0 || len(tv.flat) <= tv.height {
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
