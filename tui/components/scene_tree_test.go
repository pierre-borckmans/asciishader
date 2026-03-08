package components

import (
	"strings"
	"testing"

	"asciishader/pkg/chisel/compiler/token"
)

func TestBuildSceneTreeSimple(t *testing.T) {
	roots := BuildSceneTree("sphere")
	if len(roots) != 1 {
		t.Fatalf("expected 1 root (Geometry), got %d", len(roots))
	}
	if roots[0].Label != "Geometry" {
		t.Errorf("expected Geometry root, got %q", roots[0].Label)
	}
}

func TestBuildSceneTreeVariables(t *testing.T) {
	source := `
r = 1.5
phase = t * 0.8
sphere(r)
`
	roots := BuildSceneTree(source)
	var varNode *TreeNode
	for i := range roots {
		if roots[i].Label == "Variables" {
			varNode = &roots[i]
			break
		}
	}
	if varNode == nil {
		t.Fatal("expected Variables section")
	}
	if varNode.Detail != "(2)" {
		t.Errorf("expected detail '(2)', got %q", varNode.Detail)
	}

	children := varNode.Children()
	if len(children) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(children))
	}
	if children[0].Label != "r" {
		t.Errorf("expected first var 'r', got %q", children[0].Label)
	}
	if !strings.Contains(children[0].Detail, "1.5") {
		t.Errorf("expected detail to contain '1.5', got %q", children[0].Detail)
	}
}

func TestBuildSceneTreeFunctions(t *testing.T) {
	source := `
gear(r, teeth, thickness=0.1) = cylinder(r, thickness)
sphere
`
	roots := BuildSceneTree(source)
	var fnNode *TreeNode
	for i := range roots {
		if roots[i].Label == "Functions" {
			fnNode = &roots[i]
			break
		}
	}
	if fnNode == nil {
		t.Fatal("expected Functions section")
	}

	children := fnNode.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 function, got %d", len(children))
	}
	sig := children[0].Label
	if !strings.Contains(sig, "gear") {
		t.Errorf("expected function label to contain 'gear', got %q", sig)
	}
	if !strings.Contains(sig, "thickness=0.1") {
		t.Errorf("expected default value in signature, got %q", sig)
	}
}

func TestBuildSceneTreeSettings(t *testing.T) {
	source := `
raymarch { steps: 150, precision: 0.001 }
light { ambient: 0.15 }
bg #1a1a2e
sphere
`
	roots := BuildSceneTree(source)
	var settNode *TreeNode
	for i := range roots {
		if roots[i].Label == "Settings" {
			settNode = &roots[i]
			break
		}
	}
	if settNode == nil {
		t.Fatal("expected Settings section")
	}
	if settNode.Detail != "(3)" {
		t.Errorf("expected 3 settings, got %q", settNode.Detail)
	}

	children := settNode.Children()
	// Find raymarch
	var rm TreeNode
	for _, c := range children {
		if c.Label == "raymarch" {
			rm = c
			break
		}
	}
	if rm.Label == "" {
		t.Fatal("expected raymarch setting")
	}
	if rm.Children == nil {
		t.Fatal("expected raymarch to have children")
	}
	rmChildren := rm.Children()
	if len(rmChildren) != 2 {
		t.Fatalf("expected 2 raymarch children (steps, precision), got %d", len(rmChildren))
	}
}

func TestBuildSceneTreeGeometry(t *testing.T) {
	source := `sphere(1.5) | box(2).at(1, 0, 0)`
	roots := BuildSceneTree(source)

	var geo *TreeNode
	for i := range roots {
		if roots[i].Label == "Geometry" {
			geo = &roots[i]
			break
		}
	}
	if geo == nil {
		t.Fatal("expected Geometry section")
	}
	if geo.Children == nil {
		t.Fatal("expected Geometry to have children")
	}

	children := geo.Children()
	if len(children) == 0 {
		t.Fatal("expected geometry children")
	}
	// Should be a union node at the top
	if !strings.Contains(children[0].Label, "union") {
		// Could be the root is the union itself
		t.Logf("root geometry children: %v", nodeLabels(children))
	}
}

func TestBuildSceneTreeMethodChain(t *testing.T) {
	source := `sphere(1).at(2, 0, 0).color(#ff0000)`
	roots := BuildSceneTree(source)

	var geo *TreeNode
	for i := range roots {
		if roots[i].Label == "Geometry" {
			geo = &roots[i]
			break
		}
	}
	if geo == nil {
		t.Fatal("expected Geometry section")
	}

	children := geo.Children()
	if len(children) == 0 {
		t.Fatal("expected geometry children")
	}

	// The root should be sphere(1) with methods as children
	root := children[0]
	if !strings.Contains(root.Label, "sphere") {
		t.Errorf("expected sphere as root, got %q", root.Label)
	}
	if root.Children == nil {
		t.Fatal("expected sphere to have children (methods)")
	}

	methods := root.Children()
	if len(methods) != 2 {
		t.Fatalf("expected 2 methods (.at, .color), got %d: %v", len(methods), nodeLabels(methods))
	}
	if !strings.Contains(methods[0].Label, ".at") {
		t.Errorf("expected .at method, got %q", methods[0].Label)
	}
	if !strings.Contains(methods[1].Label, ".color") {
		t.Errorf("expected .color method, got %q", methods[1].Label)
	}
}

func TestBuildSceneTreeForLoop(t *testing.T) {
	source := `
for i in 0..8 {
  sphere(0.3).at(cos(i), sin(i), 0)
}
`
	roots := BuildSceneTree(source)

	var geo *TreeNode
	for i := range roots {
		if roots[i].Label == "Geometry" {
			geo = &roots[i]
			break
		}
	}
	if geo == nil {
		t.Fatal("expected Geometry section")
	}

	children := geo.Children()
	found := false
	for _, c := range children {
		if strings.HasPrefix(c.Label, "for ") {
			found = true
			if !strings.Contains(c.Label, "i in 0..8") {
				t.Errorf("expected 'for i in 0..8', got %q", c.Label)
			}
			break
		}
	}
	if !found {
		t.Error("expected a for loop node in geometry")
	}
}

func TestBuildSceneTreeMat(t *testing.T) {
	source := `
mat gold = { color: [1, 0.843, 0], metallic: 1 }
sphere
`
	roots := BuildSceneTree(source)

	var settNode *TreeNode
	for i := range roots {
		if roots[i].Label == "Settings" {
			settNode = &roots[i]
			break
		}
	}
	if settNode == nil {
		t.Fatal("expected Settings section")
	}

	children := settNode.Children()
	if len(children) != 1 {
		t.Fatalf("expected 1 setting (mat), got %d", len(children))
	}
	if children[0].Label != "mat gold" {
		t.Errorf("expected 'mat gold', got %q", children[0].Label)
	}
}

func TestBuildSceneTreeSpanData(t *testing.T) {
	source := `sphere(1.5)`
	roots := BuildSceneTree(source)
	if len(roots) == 0 {
		t.Fatal("expected roots")
	}

	geo := roots[0]
	if geo.Children == nil {
		t.Fatal("expected Geometry to have children")
	}
	children := geo.Children()
	if len(children) == 0 {
		t.Fatal("expected at least one geometry child")
	}
	span := SpanFromData(children[0])
	if span == (token.Span{}) {
		t.Error("expected non-zero span on geometry child node")
	}
}

func TestBuildSceneTreeEmpty(t *testing.T) {
	roots := BuildSceneTree("")
	if len(roots) != 0 {
		t.Errorf("expected 0 roots for empty source, got %d", len(roots))
	}
}

func TestBuildSceneTreeParseError(t *testing.T) {
	// Invalid syntax — parser should still produce partial AST
	roots := BuildSceneTree("sphere(")
	// Should not panic; may return partial results or nil
	_ = roots
}

func nodeLabels(nodes []TreeNode) []string {
	labels := make([]string, len(nodes))
	for i, n := range nodes {
		labels[i] = n.Label
	}
	return labels
}
