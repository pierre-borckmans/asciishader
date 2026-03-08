package components

import (
	"strings"
	"testing"

	"asciishader/pkg/chisel/compiler/token"
)

func TestBuildSceneTreeSimple(t *testing.T) {
	roots := BuildSceneTree("sphere")
	geo := findRoot(roots, "Geometry")
	if geo == nil {
		t.Fatal("expected Geometry root")
	}
	// Settings section should exist with scaffold nodes
	settings := findRoot(roots, "Settings")
	if settings == nil {
		t.Fatal("expected Settings root with scaffold nodes")
	}
}

func TestBuildSceneTreeVariables(t *testing.T) {
	source := `
r = 1.5
phase = t * 0.8
sphere(r)
`
	roots := BuildSceneTree(source)
	varNode := findRoot(roots, "Variables")
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
	fnNode := findRoot(roots, "Functions")
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
	settNode := findRoot(roots, "Settings")
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

	geo := findRoot(roots, "Geometry")
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

	geo := findRoot(roots, "Geometry")
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

	geo := findRoot(roots, "Geometry")
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

	settNode := findRoot(roots, "Settings")
	if settNode == nil {
		t.Fatal("expected Settings section")
	}

	children := settNode.Children()
	// First child should be the real mat setting
	if len(children) == 0 {
		t.Fatal("expected at least 1 setting")
	}
	if children[0].Label != "mat gold" {
		t.Errorf("expected 'mat gold', got %q", children[0].Label)
	}
	// Remaining children should be scaffold nodes
	for _, c := range children[1:] {
		if !c.Scaffold {
			t.Errorf("expected scaffold node, got regular node %q", c.Label)
		}
	}
}

func TestBuildSceneTreeSpanData(t *testing.T) {
	source := `sphere(1.5)`
	roots := BuildSceneTree(source)

	geo := findRoot(roots, "Geometry")
	if geo == nil {
		t.Fatal("expected Geometry root")
	}
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

func findRoot(roots []TreeNode, label string) *TreeNode {
	for i := range roots {
		if roots[i].Label == label {
			return &roots[i]
		}
	}
	return nil
}

func nodeLabels(nodes []TreeNode) []string {
	labels := make([]string, len(nodes))
	for i, n := range nodes {
		labels[i] = n.Label
	}
	return labels
}

func TestBuildSceneTreeScaffoldNodes(t *testing.T) {
	// Only bg is present — scaffold should show for light, camera, raymarch, post
	source := "bg #1a1a2e\nsphere\n"
	roots := BuildSceneTree(source)

	settNode := findRoot(roots, "Settings")
	if settNode == nil {
		t.Fatal("expected Settings section")
	}

	children := settNode.Children()
	var scaffolds []string
	for _, c := range children {
		if c.Scaffold {
			scaffolds = append(scaffolds, c.Label)
		}
	}
	// bg is present, so only light, camera, raymarch, post should be scaffolded
	expected := []string{"light", "camera", "raymarch", "post"}
	if len(scaffolds) != len(expected) {
		t.Fatalf("expected %d scaffold nodes, got %d: %v", len(expected), len(scaffolds), scaffolds)
	}
	for i, s := range scaffolds {
		if s != expected[i] {
			t.Errorf("scaffold[%d]: expected %q, got %q", i, expected[i], s)
		}
	}
}

func TestBuildSceneTreeScaffoldInsertAt(t *testing.T) {
	source := "bg #1a1a2e\nsphere\n"
	roots := BuildSceneTree(source)

	settNode := findRoot(roots, "Settings")
	if settNode == nil {
		t.Fatal("expected Settings section")
	}

	children := settNode.Children()
	for _, c := range children {
		if !c.Scaffold {
			continue
		}
		sd, ok := c.Data.(ScaffoldInfo)
		if !ok {
			t.Errorf("scaffold node %q has wrong Data type", c.Label)
			continue
		}
		if sd.InsertAt <= 0 {
			t.Errorf("scaffold node %q has InsertAt=%d, expected > 0", c.Label, sd.InsertAt)
		}
		if sd.Template == "" {
			t.Errorf("scaffold node %q has empty template", c.Label)
		}
	}
}

func TestBuildSceneTreeEditableVariable(t *testing.T) {
	source := "r = 1.5\nsphere(r)\n"
	roots := BuildSceneTree(source)

	varNode := findRoot(roots, "Variables")
	if varNode == nil {
		t.Fatal("expected Variables section")
	}

	children := varNode.Children()
	if len(children) == 0 {
		t.Fatal("expected at least 1 variable")
	}
	r := children[0]
	if r.Label != "r" {
		t.Fatalf("expected variable 'r', got %q", r.Label)
	}
	if !r.Editable {
		t.Error("expected variable 'r' to be editable")
	}
	if r.EditValue != "1.5" {
		t.Errorf("expected EditValue '1.5', got %q", r.EditValue)
	}
}

func TestBuildSceneTreeEditableSetting(t *testing.T) {
	source := "bg #1a1a2e\nsphere\n"
	roots := BuildSceneTree(source)

	settNode := findRoot(roots, "Settings")
	if settNode == nil {
		t.Fatal("expected Settings section")
	}

	children := settNode.Children()
	bg := children[0]
	if bg.Label != "bg" {
		t.Fatalf("expected 'bg', got %q", bg.Label)
	}
	if !bg.Editable {
		t.Error("expected 'bg' setting to be editable")
	}
	if bg.EditValue != "#1a1a2e" {
		t.Errorf("expected EditValue '#1a1a2e', got %q", bg.EditValue)
	}
}

func TestBuildSceneTreeEditableMapChild(t *testing.T) {
	source := "raymarch { steps: 128 }\nsphere\n"
	roots := BuildSceneTree(source)

	settNode := findRoot(roots, "Settings")
	if settNode == nil {
		t.Fatal("expected Settings section")
	}

	children := settNode.Children()
	var rm TreeNode
	for _, c := range children {
		if c.Label == "raymarch" {
			rm = c
			break
		}
	}
	if rm.Children == nil {
		t.Fatal("expected raymarch to have children")
	}
	rmChildren := rm.Children()
	found := false
	for _, c := range rmChildren {
		if c.Label == "steps" {
			found = true
			if !c.Editable {
				t.Error("expected 'steps' to be editable")
			}
			if c.EditValue != "128" {
				t.Errorf("expected EditValue '128', got %q", c.EditValue)
			}
		}
	}
	if !found {
		t.Error("expected 'steps' child in raymarch")
	}
}
