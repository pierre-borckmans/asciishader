package components

import (
	"strings"
	"testing"
)

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
