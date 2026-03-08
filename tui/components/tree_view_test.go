package components

import (
	"os"
	"strings"
	"testing"

	zone "github.com/lrstanley/bubblezone/v2"
)

func TestMain(m *testing.M) {
	zone.NewGlobal()
	os.Exit(m.Run())
}

func makeTestTree() []TreeNode {
	return []TreeNode{
		{Label: "alpha"},
		{Label: "beta", Detail: "(2)", Children: func() []TreeNode {
			return []TreeNode{
				{Label: "child1"},
				{Label: "child2"},
			}
		}},
		{Label: "gamma"},
	}
}

func TestTreeViewSetRoots(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)
	tv.SetRoots(makeTestTree())

	if tv.Len() != 3 {
		t.Errorf("expected 3 visible nodes, got %d", tv.Len())
	}
	if tv.Cursor() != 0 {
		t.Errorf("expected cursor at 0, got %d", tv.Cursor())
	}
}

func TestTreeViewExpandCollapse(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)
	tv.SetRoots([]TreeNode{
		{Label: "root", Children: func() []TreeNode {
			return []TreeNode{
				{Label: "child1"},
				{Label: "child2", Children: func() []TreeNode {
					return []TreeNode{
						{Label: "grandchild"},
					}
				}},
			}
		}},
	})

	if tv.Len() != 1 {
		t.Fatalf("expected 1, got %d", tv.Len())
	}

	// Expand root
	tv.HandleKey("right")
	if tv.Len() != 3 {
		t.Fatalf("expected 3 after expand, got %d", tv.Len())
	}

	// Move to child2 and expand
	tv.HandleKey("down")
	tv.HandleKey("down")
	tv.HandleKey("right")
	if tv.Len() != 4 {
		t.Fatalf("expected 4 after nested expand, got %d", tv.Len())
	}

	// Collapse root — child2 expand state preserved in map
	tv.cursor = 0
	tv.HandleKey("left")
	if tv.Len() != 1 {
		t.Fatalf("expected 1 after collapse, got %d", tv.Len())
	}

	// Re-expand root: child2 should also be expanded (state preserved)
	tv.HandleKey("right")
	if tv.Len() != 4 {
		t.Fatalf("expected 4 after re-expand (preserved nested state), got %d", tv.Len())
	}
}

func TestTreeViewMoveToParent(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)
	tv.SetRoots([]TreeNode{
		{Label: "root", Children: func() []TreeNode {
			return []TreeNode{
				{Label: "child"},
			}
		}},
	})

	// Expand and move to child
	tv.HandleKey("right") // expand root
	tv.HandleKey("down")  // cursor on child
	if tv.cursor != 1 {
		t.Fatalf("expected cursor at 1, got %d", tv.cursor)
	}

	// Left on leaf → move to parent
	tv.HandleKey("left")
	if tv.cursor != 0 {
		t.Fatalf("expected cursor at 0 (parent), got %d", tv.cursor)
	}
}

func TestTreeViewRightOnExpandedMovesToChild(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)
	tv.SetRoots([]TreeNode{
		{Label: "root", Children: func() []TreeNode {
			return []TreeNode{
				{Label: "child"},
			}
		}},
	})

	tv.HandleKey("right") // expand
	if tv.cursor != 0 {
		t.Fatalf("cursor should stay on root after expand, got %d", tv.cursor)
	}

	tv.HandleKey("right") // already expanded → move to first child
	if tv.cursor != 1 {
		t.Fatalf("expected cursor at 1 (first child), got %d", tv.cursor)
	}
}

func TestTreeViewRender(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(30, 5)
	tv.SetRoots(makeTestTree())

	output := tv.Render()
	if !strings.Contains(output, "alpha") {
		t.Error("expected 'alpha' in output")
	}
	if !strings.Contains(output, "beta") {
		t.Error("expected 'beta' in output")
	}
	if !strings.Contains(output, "▸") {
		t.Error("expected collapsed arrow ▸")
	}
	if !strings.Contains(output, "(2)") {
		t.Error("expected detail '(2)'")
	}
}

func TestTreeViewRenderCursor(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(30, 5)
	tv.SetRoots(makeTestTree())

	// Cursor at row 0 — should have highlight ANSI (background 240)
	output := tv.Render()
	lines := strings.Split(output, "\n")
	if len(lines) < 1 {
		t.Fatal("expected at least 1 line")
	}
	if !strings.Contains(lines[0], "\x1b[97;48;5;240m") {
		t.Error("expected cursor highlight on first line")
	}
}

func TestTreeViewRenderEmpty(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(30, 5)

	output := tv.Render()
	if !strings.Contains(output, "(empty)") {
		t.Error("expected '(empty)' for tree with no roots")
	}
}

func TestTreeViewCursorPersistence(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)

	tv.SetRoots(makeTestTree())
	tv.HandleKey("down")
	tv.HandleKey("down")
	if tv.cursor != 2 {
		t.Fatalf("expected cursor at 2, got %d", tv.cursor)
	}

	// Rebuild with same structure — cursor should stay
	tv.SetRoots(makeTestTree())
	if tv.cursor != 2 {
		t.Fatalf("expected cursor at 2 after rebuild, got %d", tv.cursor)
	}
}

func TestTreeViewDuplicateLabels(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)
	tv.SetRoots([]TreeNode{
		{Label: "root", Children: func() []TreeNode {
			return []TreeNode{
				{Label: "sphere(1.0)"},
				{Label: "sphere(1.0)"},
			}
		}},
	})

	tv.HandleKey("right")
	if tv.Len() != 3 {
		t.Fatalf("expected 3, got %d", tv.Len())
	}

	// Sibling nodes with the same label must have unique paths
	if tv.flat[1].path == tv.flat[2].path {
		t.Error("duplicate paths for sibling nodes with same label")
	}
}

func TestTreeViewScrolling(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 3) // only 3 visible rows

	tv.SetRoots([]TreeNode{
		{Label: "a"},
		{Label: "b"},
		{Label: "c"},
		{Label: "d"},
		{Label: "e"},
	})

	if tv.scrollOffset != 0 {
		t.Fatalf("expected scroll at 0, got %d", tv.scrollOffset)
	}

	// Move cursor past visible area
	tv.HandleKey("down")
	tv.HandleKey("down")
	tv.HandleKey("down") // cursor=3, should scroll
	if tv.scrollOffset == 0 {
		t.Error("expected scroll to advance")
	}
	if tv.cursor != 3 {
		t.Fatalf("expected cursor at 3, got %d", tv.cursor)
	}
}

func TestTreeViewOnSelect(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)

	var selected string
	tv.OnSelect = func(node TreeNode) {
		selected = node.Label
	}

	tv.SetRoots([]TreeNode{
		{Label: "leaf1"},
		{Label: "leaf2"},
	})

	tv.HandleKey("down")
	tv.HandleKey("enter")
	if selected != "leaf2" {
		t.Fatalf("expected 'leaf2' selected, got %q", selected)
	}
}

func TestTreeViewEnterTogglesExpand(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 10)

	tv.SetRoots([]TreeNode{
		{Label: "root", Children: func() []TreeNode {
			return []TreeNode{
				{Label: "child"},
			}
		}},
	})

	tv.HandleKey("enter") // expand
	if tv.Len() != 2 {
		t.Fatalf("expected 2 after enter-expand, got %d", tv.Len())
	}

	tv.HandleKey("enter") // collapse
	if tv.Len() != 1 {
		t.Fatalf("expected 1 after enter-collapse, got %d", tv.Len())
	}
}

func TestTreeViewRenderScrollbar(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(30, 3)

	tv.SetRoots([]TreeNode{
		{Label: "a"},
		{Label: "b"},
		{Label: "c"},
		{Label: "d"},
		{Label: "e"},
	})

	output := tv.Render()
	// Should contain scrollbar character (●)
	if !strings.Contains(output, "●") {
		t.Error("expected scrollbar thumb in output when content exceeds height")
	}
}

func TestTreeViewScaffoldRender(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(30, 5)
	tv.SetRoots([]TreeNode{
		{Label: "bg", Detail: "#1a1a2e"},
		{Label: "light", Scaffold: true},
		{Label: "camera", Scaffold: true},
	})

	output := tv.Render()
	// Scaffold nodes should show with + prefix
	if !strings.Contains(output, "+ light") {
		t.Error("expected scaffold '+ light' in output")
	}
	if !strings.Contains(output, "+ camera") {
		t.Error("expected scaffold '+ camera' in output")
	}
	// Scaffold nodes should have dimmed styling (color 239)
	if !strings.Contains(output, "\x1b[38;5;239m") {
		t.Error("expected dimmed ANSI styling for scaffold nodes")
	}
}

func TestTreeViewScaffoldEnter(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(30, 5)
	tv.SetRoots([]TreeNode{
		{Label: "light", Scaffold: true, Data: "scaffold-data"},
	})

	tv.HandleKey("enter")
	if tv.ScaffoldResult == nil {
		t.Fatal("expected ScaffoldResult to be set after enter on scaffold node")
	}
	if tv.ScaffoldResult.Label != "light" {
		t.Errorf("expected scaffold label 'light', got %q", tv.ScaffoldResult.Label)
	}
}

func TestTreeViewEditMode(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 5)
	tv.SetRoots([]TreeNode{
		{Label: "bg", Detail: "#1a1a2e", Editable: true, EditValue: "#1a1a2e"},
		{Label: "gamma"},
	})

	// Enter starts edit mode
	tv.HandleKey("enter")
	if !tv.IsEditing() {
		t.Fatal("expected edit mode after enter on editable node")
	}

	// Type some characters
	tv.HandleKey("backspace") // delete last char
	tv.HandleKey("f")

	// Confirm edit
	tv.HandleKey("enter")
	if tv.IsEditing() {
		t.Error("expected edit mode to end after enter")
	}
	if tv.EditResult == nil {
		t.Fatal("expected EditResult after confirming edit")
	}
	if tv.EditResult.NewValue != "#1a1a2f" {
		t.Errorf("expected '#1a1a2f', got %q", tv.EditResult.NewValue)
	}
}

func TestTreeViewEditCancel(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 5)
	tv.SetRoots([]TreeNode{
		{Label: "r", Detail: "= 1.5", Editable: true, EditValue: "1.5"},
	})

	tv.HandleKey("enter")
	if !tv.IsEditing() {
		t.Fatal("expected edit mode")
	}

	tv.HandleKey("esc")
	if tv.IsEditing() {
		t.Error("expected edit mode to end on esc")
	}
	if tv.EditResult != nil {
		t.Error("expected no EditResult after cancel")
	}
}

func TestTreeViewEditRender(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 5)
	tv.SetRoots([]TreeNode{
		{Label: "bg", Detail: "#1a1a2e", Editable: true, EditValue: "#1a1a2e"},
	})

	tv.HandleKey("enter")
	output := tv.Render()
	// Should show yellow edit mode styling
	if !strings.Contains(output, "\x1b[93m") {
		t.Error("expected yellow ANSI styling in edit mode")
	}
	// Should show reverse video cursor
	if !strings.Contains(output, "\x1b[7m") {
		t.Error("expected reverse video cursor in edit mode")
	}
}

func TestTreeViewEditNoChangeNoResult(t *testing.T) {
	tv := NewTreeView()
	tv.SetSize(40, 5)
	tv.SetRoots([]TreeNode{
		{Label: "r", Detail: "= 1.5", Editable: true, EditValue: "1.5"},
	})

	tv.HandleKey("enter") // start edit
	tv.HandleKey("enter") // confirm without changes
	if tv.EditResult != nil {
		t.Error("expected no EditResult when value unchanged")
	}
}
