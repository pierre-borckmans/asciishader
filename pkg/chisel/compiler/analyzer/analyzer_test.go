package analyzer

import (
	"strings"
	"testing"

	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/diagnostic"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
)

// helper: parse and analyze source, return diagnostics.
func analyze(t *testing.T, source string) []diagnostic.Diagnostic {
	t.Helper()
	tokens, lexDiags := lexer.Lex("test.chisel", source)
	for _, d := range lexDiags {
		if d.Severity == diagnostic.Error {
			t.Fatalf("lex error: %s", d.Message)
		}
	}
	prog, parseDiags := parser.Parse(tokens)
	for _, d := range parseDiags {
		if d.Severity == diagnostic.Error {
			t.Fatalf("parse error: %s", d.Message)
		}
	}
	return Analyze(prog)
}

// helper: check that diagnostics contain an error with the given substring.
func expectError(t *testing.T, diags []diagnostic.Diagnostic, substr string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == diagnostic.Error && strings.Contains(d.Message, substr) {
			return
		}
		// Also check help text.
		if d.Severity == diagnostic.Error && strings.Contains(d.Help, substr) {
			return
		}
	}
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Error())
	}
	t.Errorf("expected error containing %q, got: %v", substr, msgs)
}

// helper: check that there are no errors.
func expectNoErrors(t *testing.T, diags []diagnostic.Diagnostic) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			t.Errorf("unexpected error: %s", d.Error())
		}
	}
}

// ---------------------------------------------------------------------------
// Undefined variable
// ---------------------------------------------------------------------------

func TestUndefinedVariable(t *testing.T) {
	diags := analyze(t, "sphere(r)")
	expectError(t, diags, "undefined variable 'r'")
}

// ---------------------------------------------------------------------------
// Fuzzy suggestion
// ---------------------------------------------------------------------------

func TestFuzzySuggestion(t *testing.T) {
	diags := analyze(t, "sphree")
	expectError(t, diags, "did you mean 'sphere'?")
}

// ---------------------------------------------------------------------------
// Unknown method with suggestion
// ---------------------------------------------------------------------------

func TestUnknownMethodMove(t *testing.T) {
	diags := analyze(t, "sphere.move(1, 0, 0)")
	expectError(t, diags, "unknown method 'move'")
}

func TestUnknownMethodTranslate(t *testing.T) {
	diags := analyze(t, "sphere.translate(1, 0, 0)")
	expectError(t, diags, "did you mean 'at'?")
}

// ---------------------------------------------------------------------------
// Valid programs — no errors
// ---------------------------------------------------------------------------

func TestValidSphere(t *testing.T) {
	diags := analyze(t, "sphere")
	expectNoErrors(t, diags)
}

func TestValidUnion(t *testing.T) {
	diags := analyze(t, "sphere | box.at(2, 0, 0)")
	expectNoErrors(t, diags)
}

func TestValidVariableUsage(t *testing.T) {
	diags := analyze(t, "r = 1.5\nsphere(r)")
	expectNoErrors(t, diags)
}

func TestValidFunctionDef(t *testing.T) {
	diags := analyze(t, "f(x) = sphere(x)\nf(2)")
	expectNoErrors(t, diags)
}

func TestValidMethodChain(t *testing.T) {
	diags := analyze(t, "sphere.at(1, 0, 0).scale(2).red")
	expectNoErrors(t, diags)
}

func TestValidNamedColors(t *testing.T) {
	diags := analyze(t, "red")
	expectNoErrors(t, diags)
}

func TestValidConstants(t *testing.T) {
	diags := analyze(t, "sphere(PI)")
	expectNoErrors(t, diags)
}

// ---------------------------------------------------------------------------
// Shape arity
// ---------------------------------------------------------------------------

func TestShapeArityTooMany(t *testing.T) {
	diags := analyze(t, "sphere(1, 2, 3)")
	expectError(t, diags, "takes at most 1 argument")
}

func TestShapeArityOk(t *testing.T) {
	diags := analyze(t, "box(1, 2, 3)")
	expectNoErrors(t, diags)
}

// ---------------------------------------------------------------------------
// Levenshtein distance
// ---------------------------------------------------------------------------

func TestLevenshteinIdentical(t *testing.T) {
	if d := levenshtein("sphere", "sphere"); d != 0 {
		t.Errorf("expected 0, got %d", d)
	}
}

func TestLevenshteinOneOff(t *testing.T) {
	if d := levenshtein("sphere", "sphree"); d != 2 {
		t.Errorf("expected 2, got %d", d)
	}
}

func TestLevenshteinEmpty(t *testing.T) {
	if d := levenshtein("", "abc"); d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
	if d := levenshtein("abc", ""); d != 3 {
		t.Errorf("expected 3, got %d", d)
	}
}

// ---------------------------------------------------------------------------
// Scope
// ---------------------------------------------------------------------------

func TestScopeDefineAndLookup(t *testing.T) {
	s := NewScope(nil)
	sym := &Symbol{Name: "x", Type: TypeFloat}
	if err := s.Define("x", sym); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := s.Lookup("x")
	if found != sym {
		t.Errorf("expected to find symbol x")
	}
}

func TestScopeParentLookup(t *testing.T) {
	parent := NewScope(nil)
	parent.Symbols["x"] = &Symbol{Name: "x", Type: TypeFloat}
	child := NewScope(parent)
	found := child.Lookup("x")
	if found == nil {
		t.Errorf("expected to find 'x' in parent scope")
	}
}

func TestScopeShadowing(t *testing.T) {
	parent := NewScope(nil)
	parent.Symbols["x"] = &Symbol{Name: "x", Type: TypeFloat}
	child := NewScope(parent)
	childSym := &Symbol{Name: "x", Type: TypeVec3}
	if err := child.Define("x", childSym); err != nil {
		t.Fatalf("shadowing should be allowed: %v", err)
	}
	found := child.Lookup("x")
	if found.Type != TypeVec3 {
		t.Errorf("expected shadowed symbol to have TypeVec3")
	}
}

func TestScopeRedefineSameScope(t *testing.T) {
	s := NewScope(nil)
	_ = s.Define("x", &Symbol{Name: "x", Type: TypeFloat})
	err := s.Define("x", &Symbol{Name: "x", Type: TypeVec3})
	if err == nil {
		t.Errorf("expected error for redefinition in same scope")
	}
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

func TestTypeString(t *testing.T) {
	tests := []struct {
		typ  Type
		want string
	}{
		{TypeUnknown, "unknown"},
		{TypeFloat, "float"},
		{TypeVec2, "vec2"},
		{TypeVec3, "vec3"},
		{TypeBool, "bool"},
		{TypeSDF2D, "sdf2d"},
		{TypeSDF3D, "sdf3d"},
		{TypeColor, "color"},
		{TypeVoid, "void"},
	}
	for _, tt := range tests {
		if got := tt.typ.String(); got != tt.want {
			t.Errorf("Type(%d).String() = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Builtins scope
// ---------------------------------------------------------------------------

func TestBuiltinScope(t *testing.T) {
	s := NewBuiltinScope()

	// Verify some built-ins exist.
	for _, name := range []string{
		"sphere", "box", "circle", "sin", "cos", "PI", "t", "p",
		"x", "y", "z", "red", "green", "blue", "rgb", "hsl",
	} {
		if sym := s.Lookup(name); sym == nil {
			t.Errorf("expected built-in '%s' to be defined", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Suggest
// ---------------------------------------------------------------------------

func TestSuggest(t *testing.T) {
	candidates := []string{"sphere", "box", "cylinder", "torus"}

	if got := suggest("sphree", candidates, 2); got != "sphere" {
		t.Errorf("suggest(sphree) = %q, want 'sphere'", got)
	}

	if got := suggest("bx", candidates, 2); got != "box" {
		t.Errorf("suggest(bx) = %q, want 'box'", got)
	}

	// Too far away.
	if got := suggest("abcdef", candidates, 2); got != "" {
		t.Errorf("suggest(abcdef) = %q, want empty", got)
	}
}

// Ensure ast import is used for the linter.
var _ ast.Node
