package parser

import (
	"math"
	"testing"

	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/token"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// parseSource lexes and parses a source string, returning the program and diagnostics.
func parseSource(source string) (*ast.Program, []diagnostic.Diagnostic) {
	tokens, _ := lexer.Lex("test.chisel", source)
	return Parse(tokens)
}

// parseExpr is a convenience helper that parses a source string and returns
// the first expression from the program. It expects exactly one ExprStmt.
func parseExpr(t *testing.T, source string) ast.Expr {
	t.Helper()
	prog, diags := parseSource(source)
	for _, d := range diags {
		t.Logf("diagnostic: %s", d.Error())
	}
	if len(prog.Statements) == 0 {
		t.Fatalf("parseExpr(%q): no statements", source)
	}
	es, ok := prog.Statements[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("parseExpr(%q): expected ExprStmt, got %T", source, prog.Statements[0])
	}
	return es.Expression
}

// assertNoDiags fails if there are any diagnostics.
func assertNoDiags(t *testing.T, diags []diagnostic.Diagnostic) {
	t.Helper()
	if len(diags) > 0 {
		for _, d := range diags {
			t.Errorf("unexpected diagnostic: %s", d.Error())
		}
	}
}

// floatEq compares two floats with tolerance.
func floatEq(a, b float64) bool {
	return math.Abs(a-b) < 1e-6
}

// ---------------------------------------------------------------------------
// Atoms (Task 1.8)
// ---------------------------------------------------------------------------

func TestParseNumberLitInt(t *testing.T) {
	expr := parseExpr(t, "42")
	num, ok := expr.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit, got %T", expr)
	}
	if !floatEq(num.Value, 42) {
		t.Errorf("expected 42, got %f", num.Value)
	}
}

func TestParseNumberLitFloat(t *testing.T) {
	expr := parseExpr(t, "3.14")
	num, ok := expr.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit, got %T", expr)
	}
	if !floatEq(num.Value, 3.14) {
		t.Errorf("expected 3.14, got %f", num.Value)
	}
}

func TestParseBoolTrue(t *testing.T) {
	expr := parseExpr(t, "true")
	b, ok := expr.(*ast.BoolLit)
	if !ok {
		t.Fatalf("expected BoolLit, got %T", expr)
	}
	if !b.Value {
		t.Error("expected true")
	}
}

func TestParseBoolFalse(t *testing.T) {
	expr := parseExpr(t, "false")
	b, ok := expr.(*ast.BoolLit)
	if !ok {
		t.Fatalf("expected BoolLit, got %T", expr)
	}
	if b.Value {
		t.Error("expected false")
	}
}

func TestParseIdent(t *testing.T) {
	expr := parseExpr(t, "sphere")
	id, ok := expr.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident, got %T", expr)
	}
	if id.Name != "sphere" {
		t.Errorf("expected 'sphere', got %q", id.Name)
	}
}

func TestParseHexColor6(t *testing.T) {
	expr := parseExpr(t, "#ff0000")
	hc, ok := expr.(*ast.HexColorLit)
	if !ok {
		t.Fatalf("expected HexColorLit, got %T", expr)
	}
	if !floatEq(hc.R, 1) || !floatEq(hc.G, 0) || !floatEq(hc.B, 0) || !floatEq(hc.A, 1) {
		t.Errorf("expected {1,0,0,1}, got {%f,%f,%f,%f}", hc.R, hc.G, hc.B, hc.A)
	}
}

func TestParseHexColor3(t *testing.T) {
	expr := parseExpr(t, "#f00")
	hc, ok := expr.(*ast.HexColorLit)
	if !ok {
		t.Fatalf("expected HexColorLit, got %T", expr)
	}
	if !floatEq(hc.R, 1) || !floatEq(hc.G, 0) || !floatEq(hc.B, 0) || !floatEq(hc.A, 1) {
		t.Errorf("expected {1,0,0,1}, got {%f,%f,%f,%f}", hc.R, hc.G, hc.B, hc.A)
	}
}

func TestParseHexColor8(t *testing.T) {
	expr := parseExpr(t, "#ff000080")
	hc, ok := expr.(*ast.HexColorLit)
	if !ok {
		t.Fatalf("expected HexColorLit, got %T", expr)
	}
	if !floatEq(hc.R, 1) || !floatEq(hc.G, 0) || !floatEq(hc.B, 0) {
		t.Errorf("expected R=1,G=0,B=0 got {%f,%f,%f}", hc.R, hc.G, hc.B)
	}
	if !floatEq(hc.A, 128.0/255.0) {
		t.Errorf("expected A=%f, got %f", 128.0/255.0, hc.A)
	}
}

func TestParseHexColor1a1a2e(t *testing.T) {
	expr := parseExpr(t, "#1a1a2e")
	hc, ok := expr.(*ast.HexColorLit)
	if !ok {
		t.Fatalf("expected HexColorLit, got %T", expr)
	}
	if !floatEq(hc.R, 0x1a/255.0) || !floatEq(hc.G, 0x1a/255.0) || !floatEq(hc.B, 0x2e/255.0) {
		t.Errorf("unexpected color values: {%f,%f,%f}", hc.R, hc.G, hc.B)
	}
}

func TestParseStringLit(t *testing.T) {
	expr := parseExpr(t, `"hello"`)
	s, ok := expr.(*ast.StringLit)
	if !ok {
		t.Fatalf("expected StringLit, got %T", expr)
	}
	if s.Value != "hello" {
		t.Errorf("expected 'hello', got %q", s.Value)
	}
}

func TestParseVecLit(t *testing.T) {
	expr := parseExpr(t, "[1, 2, 3]")
	vec, ok := expr.(*ast.VecLit)
	if !ok {
		t.Fatalf("expected VecLit, got %T", expr)
	}
	if len(vec.Elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(vec.Elems))
	}
	for i, expected := range []float64{1, 2, 3} {
		num, ok := vec.Elems[i].(*ast.NumberLit)
		if !ok {
			t.Errorf("elem[%d]: expected NumberLit, got %T", i, vec.Elems[i])
			continue
		}
		if !floatEq(num.Value, expected) {
			t.Errorf("elem[%d]: expected %f, got %f", i, expected, num.Value)
		}
	}
}

func TestParseVecLit2(t *testing.T) {
	expr := parseExpr(t, "[1, 2]")
	vec, ok := expr.(*ast.VecLit)
	if !ok {
		t.Fatalf("expected VecLit, got %T", expr)
	}
	if len(vec.Elems) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(vec.Elems))
	}
}

func TestParseParenExpr(t *testing.T) {
	expr := parseExpr(t, "(sphere)")
	id, ok := expr.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident (parens stripped), got %T", expr)
	}
	if id.Name != "sphere" {
		t.Errorf("expected 'sphere', got %q", id.Name)
	}
}

func TestParseNestedParens(t *testing.T) {
	expr := parseExpr(t, "((42))")
	num, ok := expr.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit, got %T", expr)
	}
	if !floatEq(num.Value, 42) {
		t.Errorf("expected 42, got %f", num.Value)
	}
}

// ---------------------------------------------------------------------------
// Function Calls & Method Chains (Task 1.9)
// ---------------------------------------------------------------------------

func TestParseFuncCall(t *testing.T) {
	expr := parseExpr(t, "sphere(2)")
	fc, ok := expr.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall, got %T", expr)
	}
	if fc.Name != "sphere" {
		t.Errorf("expected 'sphere', got %q", fc.Name)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(fc.Args))
	}
	if fc.Args[0].Name != "" {
		t.Errorf("expected positional arg, got named %q", fc.Args[0].Name)
	}
	num, ok := fc.Args[0].Value.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit arg, got %T", fc.Args[0].Value)
	}
	if !floatEq(num.Value, 2) {
		t.Errorf("expected 2, got %f", num.Value)
	}
}

func TestParseFuncCallMultipleArgs(t *testing.T) {
	expr := parseExpr(t, "cylinder(1, 3)")
	fc, ok := expr.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall, got %T", expr)
	}
	if len(fc.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(fc.Args))
	}
}

func TestParseFuncCallNamedArg(t *testing.T) {
	expr := parseExpr(t, "sphere(radius: 2)")
	fc, ok := expr.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall, got %T", expr)
	}
	if len(fc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(fc.Args))
	}
	if fc.Args[0].Name != "radius" {
		t.Errorf("expected named arg 'radius', got %q", fc.Args[0].Name)
	}
}

func TestParseFuncCallMixedArgs(t *testing.T) {
	expr := parseExpr(t, "fbm(p, octaves: 6)")
	fc, ok := expr.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall, got %T", expr)
	}
	if len(fc.Args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(fc.Args))
	}
	if fc.Args[0].Name != "" {
		t.Error("expected first arg positional")
	}
	if fc.Args[1].Name != "octaves" {
		t.Errorf("expected second arg named 'octaves', got %q", fc.Args[1].Name)
	}
}

func TestParseMethodCall(t *testing.T) {
	expr := parseExpr(t, "sphere.at(1, 0, 0)")
	mc, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if mc.Name != "at" {
		t.Errorf("expected 'at', got %q", mc.Name)
	}
	if len(mc.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(mc.Args))
	}
	recv, ok := mc.Receiver.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident receiver, got %T", mc.Receiver)
	}
	if recv.Name != "sphere" {
		t.Errorf("expected receiver 'sphere', got %q", recv.Name)
	}
}

func TestParseBareMethod(t *testing.T) {
	expr := parseExpr(t, "sphere.red")
	mc, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if mc.Name != "red" {
		t.Errorf("expected 'red', got %q", mc.Name)
	}
	if len(mc.Args) != 0 {
		t.Errorf("expected 0 args, got %d", len(mc.Args))
	}
}

func TestParseSwizzle(t *testing.T) {
	expr := parseExpr(t, "p.xz")
	sw, ok := expr.(*ast.Swizzle)
	if !ok {
		t.Fatalf("expected Swizzle, got %T", expr)
	}
	if sw.Components != "xz" {
		t.Errorf("expected 'xz', got %q", sw.Components)
	}
	recv, ok := sw.Receiver.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident receiver, got %T", sw.Receiver)
	}
	if recv.Name != "p" {
		t.Errorf("expected receiver 'p', got %q", recv.Name)
	}
}

func TestParseSwizzleXYZ(t *testing.T) {
	expr := parseExpr(t, "v.xyz")
	sw, ok := expr.(*ast.Swizzle)
	if !ok {
		t.Fatalf("expected Swizzle, got %T", expr)
	}
	if sw.Components != "xyz" {
		t.Errorf("expected 'xyz', got %q", sw.Components)
	}
}

func TestParseSwizzleSingleComponent(t *testing.T) {
	expr := parseExpr(t, "p.x")
	sw, ok := expr.(*ast.Swizzle)
	if !ok {
		t.Fatalf("expected Swizzle, got %T", expr)
	}
	if sw.Components != "x" {
		t.Errorf("expected 'x', got %q", sw.Components)
	}
}

func TestParseMethodNotSwizzle(t *testing.T) {
	expr := parseExpr(t, "v.scale")
	mc, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if mc.Name != "scale" {
		t.Errorf("expected 'scale', got %q", mc.Name)
	}
}

func TestParseMethodChainComplex(t *testing.T) {
	expr := parseExpr(t, "sphere.at(1,0,0).scale(2).red")
	// sphere.at(1,0,0).scale(2).red
	// -> MethodCall{MethodCall{MethodCall{Ident{sphere}, at, [1,0,0]}, scale, [2]}, red, []}

	mc1, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall (red), got %T", expr)
	}
	if mc1.Name != "red" {
		t.Errorf("expected 'red', got %q", mc1.Name)
	}

	mc2, ok := mc1.Receiver.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall (scale), got %T", mc1.Receiver)
	}
	if mc2.Name != "scale" {
		t.Errorf("expected 'scale', got %q", mc2.Name)
	}

	mc3, ok := mc2.Receiver.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall (at), got %T", mc2.Receiver)
	}
	if mc3.Name != "at" {
		t.Errorf("expected 'at', got %q", mc3.Name)
	}
	if len(mc3.Args) != 3 {
		t.Errorf("expected 3 args for at(), got %d", len(mc3.Args))
	}

	recv, ok := mc3.Receiver.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident (sphere), got %T", mc3.Receiver)
	}
	if recv.Name != "sphere" {
		t.Errorf("expected 'sphere', got %q", recv.Name)
	}
}

func TestParseFuncCallNoArgs(t *testing.T) {
	expr := parseExpr(t, "sphere()")
	fc, ok := expr.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall, got %T", expr)
	}
	if len(fc.Args) != 0 {
		t.Errorf("expected 0 args, got %d", len(fc.Args))
	}
}

// ---------------------------------------------------------------------------
// Pratt Expression Parser — Operators (Task 1.10)
// ---------------------------------------------------------------------------

func TestParseUnionOp(t *testing.T) {
	expr := parseExpr(t, "a | b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union, got %s", be.Op)
	}
}

func TestParseSubtractOp(t *testing.T) {
	expr := parseExpr(t, "a - b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Subtract {
		t.Errorf("expected Subtract, got %s", be.Op)
	}
}

func TestParseIntersectOp(t *testing.T) {
	expr := parseExpr(t, "a & b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Intersect {
		t.Errorf("expected Intersect, got %s", be.Op)
	}
}

func TestParseSmoothUnionOp(t *testing.T) {
	expr := parseExpr(t, "a |~0.3 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.SmoothUnion {
		t.Errorf("expected SmoothUnion, got %s", be.Op)
	}
	if be.Blend == nil {
		t.Fatal("expected blend radius, got nil")
	}
	if !floatEq(*be.Blend, 0.3) {
		t.Errorf("expected blend 0.3, got %f", *be.Blend)
	}
}

func TestParseSmoothSubtractOp(t *testing.T) {
	expr := parseExpr(t, "a -~0.2 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.SmoothSubtract {
		t.Errorf("expected SmoothSubtract, got %s", be.Op)
	}
	if be.Blend == nil || !floatEq(*be.Blend, 0.2) {
		t.Errorf("expected blend 0.2")
	}
}

func TestParseSmoothIntersectOp(t *testing.T) {
	expr := parseExpr(t, "a &~0.1 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.SmoothIntersect {
		t.Errorf("expected SmoothIntersect, got %s", be.Op)
	}
	if be.Blend == nil || !floatEq(*be.Blend, 0.1) {
		t.Errorf("expected blend 0.1")
	}
}

func TestParseChamferUnionOp(t *testing.T) {
	expr := parseExpr(t, "a |/0.3 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.ChamferUnion {
		t.Errorf("expected ChamferUnion, got %s", be.Op)
	}
	if be.Blend == nil || !floatEq(*be.Blend, 0.3) {
		t.Errorf("expected blend 0.3")
	}
}

func TestParseChamferSubtractOp(t *testing.T) {
	expr := parseExpr(t, "a -/0.2 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.ChamferSubtract {
		t.Errorf("expected ChamferSubtract, got %s", be.Op)
	}
}

func TestParseChamferIntersectOp(t *testing.T) {
	expr := parseExpr(t, "a &/0.1 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.ChamferIntersect {
		t.Errorf("expected ChamferIntersect, got %s", be.Op)
	}
}

func TestParseSmoothWithoutRadius(t *testing.T) {
	expr := parseExpr(t, "a |~ b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.SmoothUnion {
		t.Errorf("expected SmoothUnion, got %s", be.Op)
	}
	if be.Blend != nil {
		t.Errorf("expected no blend radius, got %f", *be.Blend)
	}
}

func TestParseAddOp(t *testing.T) {
	expr := parseExpr(t, "a + b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Add {
		t.Errorf("expected Add, got %s", be.Op)
	}
}

func TestParseMulOp(t *testing.T) {
	expr := parseExpr(t, "a * b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Mul {
		t.Errorf("expected Mul, got %s", be.Op)
	}
}

func TestParseDivOp(t *testing.T) {
	expr := parseExpr(t, "a / b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Div {
		t.Errorf("expected Div, got %s", be.Op)
	}
}

func TestParseModOp(t *testing.T) {
	expr := parseExpr(t, "a % b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Mod {
		t.Errorf("expected Mod, got %s", be.Op)
	}
}

func TestParseEqOp(t *testing.T) {
	expr := parseExpr(t, "a == b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Eq {
		t.Errorf("expected Eq, got %s", be.Op)
	}
}

func TestParseNeqOp(t *testing.T) {
	expr := parseExpr(t, "a != b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Neq {
		t.Errorf("expected Neq, got %s", be.Op)
	}
}

func TestParseLtOp(t *testing.T) {
	expr := parseExpr(t, "a < b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Lt {
		t.Errorf("expected Lt, got %s", be.Op)
	}
}

func TestParseGtOp(t *testing.T) {
	expr := parseExpr(t, "a > b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Gt {
		t.Errorf("expected Gt, got %s", be.Op)
	}
}

func TestParseLteOp(t *testing.T) {
	expr := parseExpr(t, "a <= b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Lte {
		t.Errorf("expected Lte, got %s", be.Op)
	}
}

func TestParseGteOp(t *testing.T) {
	expr := parseExpr(t, "a >= b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Gte {
		t.Errorf("expected Gte, got %s", be.Op)
	}
}

func TestParseUnaryNeg(t *testing.T) {
	expr := parseExpr(t, "-a")
	ue, ok := expr.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	if ue.Op != ast.Neg {
		t.Errorf("expected Neg, got %s", ue.Op)
	}
	id, ok := ue.Operand.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident operand, got %T", ue.Operand)
	}
	if id.Name != "a" {
		t.Errorf("expected 'a', got %q", id.Name)
	}
}

func TestParseUnaryNot(t *testing.T) {
	expr := parseExpr(t, "!a")
	ue, ok := expr.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	if ue.Op != ast.Not {
		t.Errorf("expected Not, got %s", ue.Op)
	}
}

// Precedence tests

func TestPrecUnionSubtract(t *testing.T) {
	// "a | b - c" should parse as "a | (b - c)" since subtract > union
	expr := parseExpr(t, "a | b - c")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union at top, got %s", be.Op)
	}
	right, ok := be.Right.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on right, got %T", be.Right)
	}
	if right.Op != ast.Subtract {
		t.Errorf("expected Subtract on right, got %s", right.Op)
	}
}

func TestPrecParenOverride(t *testing.T) {
	// "(a | b) - c" should parse as "(a | b) - c"
	expr := parseExpr(t, "(a | b) - c")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Subtract {
		t.Errorf("expected Subtract at top, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on left, got %T", be.Left)
	}
	if left.Op != ast.Union {
		t.Errorf("expected Union on left, got %s", left.Op)
	}
}

func TestPrecMulAdd(t *testing.T) {
	// "a * 2 + b" should parse as "(a * 2) + b" since mul > add
	expr := parseExpr(t, "a * 2 + b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Add {
		t.Errorf("expected Add at top, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on left, got %T", be.Left)
	}
	if left.Op != ast.Mul {
		t.Errorf("expected Mul on left, got %s", left.Op)
	}
}

func TestPrecLeftAssociative(t *testing.T) {
	// "a | b | c" should parse as "(a | b) | c" (left associative)
	expr := parseExpr(t, "a | b | c")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union at top, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on left, got %T", be.Left)
	}
	if left.Op != ast.Union {
		t.Errorf("expected Union on left, got %s", left.Op)
	}
	// Right should be "c"
	right, ok := be.Right.(*ast.Ident)
	if !ok {
		t.Fatalf("expected Ident 'c' on right, got %T", be.Right)
	}
	if right.Name != "c" {
		t.Errorf("expected 'c', got %q", right.Name)
	}
}

func TestPrecIntersectSubtract(t *testing.T) {
	// "a & b - c" should parse as "(a & b) - c" since intersect > subtract
	expr := parseExpr(t, "a & b - c")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Subtract {
		t.Errorf("expected Subtract at top, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on left, got %T", be.Left)
	}
	if left.Op != ast.Intersect {
		t.Errorf("expected Intersect on left, got %s", left.Op)
	}
}

func TestPrecComparisonArith(t *testing.T) {
	// "a + 1 == b" should parse as "(a + 1) == b" since add > comparison
	// Wait - per our precedence: comparison (4) < add (5). Higher = tighter.
	// So add binds tighter. "(a + 1) == b"
	expr := parseExpr(t, "a + 1 == b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Eq {
		t.Errorf("expected Eq at top, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on left, got %T", be.Left)
	}
	if left.Op != ast.Add {
		t.Errorf("expected Add on left, got %s", left.Op)
	}
}

func TestParseNegNumber(t *testing.T) {
	expr := parseExpr(t, "-42")
	ue, ok := expr.(*ast.UnaryExpr)
	if !ok {
		t.Fatalf("expected UnaryExpr, got %T", expr)
	}
	if ue.Op != ast.Neg {
		t.Errorf("expected Neg, got %s", ue.Op)
	}
	num, ok := ue.Operand.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit, got %T", ue.Operand)
	}
	if !floatEq(num.Value, 42) {
		t.Errorf("expected 42, got %f", num.Value)
	}
}

// ---------------------------------------------------------------------------
// Statements & Assignments (Task 1.11)
// ---------------------------------------------------------------------------

func TestParseVariableAssignment(t *testing.T) {
	prog, diags := parseSource("r = 1.5")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	assign, ok := prog.Statements[0].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("expected AssignStmt, got %T", prog.Statements[0])
	}
	if assign.Name != "r" {
		t.Errorf("expected 'r', got %q", assign.Name)
	}
	if assign.Params != nil {
		t.Errorf("expected nil params for variable, got %v", assign.Params)
	}
	num, ok := assign.Value.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit, got %T", assign.Value)
	}
	if !floatEq(num.Value, 1.5) {
		t.Errorf("expected 1.5, got %f", num.Value)
	}
}

func TestParseFunctionAssignment(t *testing.T) {
	prog, diags := parseSource("f(x) = sphere(x)")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	assign, ok := prog.Statements[0].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("expected AssignStmt, got %T", prog.Statements[0])
	}
	if assign.Name != "f" {
		t.Errorf("expected 'f', got %q", assign.Name)
	}
	if assign.Params == nil {
		t.Fatal("expected non-nil params for function")
	}
	if len(assign.Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(assign.Params))
	}
	if assign.Params[0].Name != "x" {
		t.Errorf("expected param 'x', got %q", assign.Params[0].Name)
	}
}

func TestParseFunctionWithDefaults(t *testing.T) {
	prog, diags := parseSource("f(x, y = 1) = x")
	assertNoDiags(t, diags)
	assign := prog.Statements[0].(*ast.AssignStmt)
	if len(assign.Params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(assign.Params))
	}
	if assign.Params[0].Name != "x" {
		t.Errorf("expected 'x', got %q", assign.Params[0].Name)
	}
	if assign.Params[0].Default != nil {
		t.Error("expected no default for first param")
	}
	if assign.Params[1].Name != "y" {
		t.Errorf("expected 'y', got %q", assign.Params[1].Name)
	}
	if assign.Params[1].Default == nil {
		t.Fatal("expected default for second param")
	}
	num, ok := assign.Params[1].Default.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit default, got %T", assign.Params[1].Default)
	}
	if !floatEq(num.Value, 1) {
		t.Errorf("expected default 1, got %f", num.Value)
	}
}

func TestParseAssignmentThenRef(t *testing.T) {
	prog, diags := parseSource("a = sphere\na")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Statements))
	}
	_, ok1 := prog.Statements[0].(*ast.AssignStmt)
	_, ok2 := prog.Statements[1].(*ast.ExprStmt)
	if !ok1 || !ok2 {
		t.Errorf("expected AssignStmt then ExprStmt, got %T, %T",
			prog.Statements[0], prog.Statements[1])
	}
}

// Implicit union

func TestParseImplicitUnionProgram(t *testing.T) {
	prog, diags := parseSource("sphere\nbox")
	assertNoDiags(t, diags)
	// Two consecutive ExprStmts should be merged into one with Union.
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement (implicit union), got %d", len(prog.Statements))
	}
	es, ok := prog.Statements[0].(*ast.ExprStmt)
	if !ok {
		t.Fatalf("expected ExprStmt, got %T", prog.Statements[0])
	}
	be, ok := es.Expression.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr (Union), got %T", es.Expression)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union, got %s", be.Op)
	}
}

func TestParseImplicitUnionThree(t *testing.T) {
	prog, diags := parseSource("sphere\nbox\ncylinder")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	es := prog.Statements[0].(*ast.ExprStmt)
	// Should be Union(Union(sphere, box), cylinder)
	be, ok := es.Expression.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", es.Expression)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union at top, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr on left, got %T", be.Left)
	}
	if left.Op != ast.Union {
		t.Errorf("expected Union on left, got %s", left.Op)
	}
}

func TestParseBlockImplicitUnion(t *testing.T) {
	expr := parseExpr(t, "{ sphere\n  box }")
	block, ok := expr.(*ast.Block)
	if !ok {
		t.Fatalf("expected Block, got %T", expr)
	}
	// Result should be a Union of sphere and box
	if block.Result == nil {
		t.Fatal("expected block result")
	}
	be, ok := block.Result.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr (Union) as result, got %T", block.Result)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union, got %s", be.Op)
	}
}

func TestParseBlockWithAssignmentAndResult(t *testing.T) {
	expr := parseExpr(t, "{ r = 2\n  sphere(r) }")
	block, ok := expr.(*ast.Block)
	if !ok {
		t.Fatalf("expected Block, got %T", expr)
	}
	if len(block.Stmts) != 1 {
		t.Fatalf("expected 1 statement in block, got %d", len(block.Stmts))
	}
	_, ok = block.Stmts[0].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("expected AssignStmt, got %T", block.Stmts[0])
	}
	if block.Result == nil {
		t.Fatal("expected block result")
	}
	fc, ok := block.Result.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall as result, got %T", block.Result)
	}
	if fc.Name != "sphere" {
		t.Errorf("expected 'sphere', got %q", fc.Name)
	}
}

// ---------------------------------------------------------------------------
// Control Flow (Task 1.12)
// ---------------------------------------------------------------------------

func TestParseForExpr(t *testing.T) {
	expr := parseExpr(t, "for i in 0..8 { sphere }")
	fe, ok := expr.(*ast.ForExpr)
	if !ok {
		t.Fatalf("expected ForExpr, got %T", expr)
	}
	if len(fe.Iterators) != 1 {
		t.Fatalf("expected 1 iterator, got %d", len(fe.Iterators))
	}
	it := fe.Iterators[0]
	if it.Name != "i" {
		t.Errorf("expected 'i', got %q", it.Name)
	}
	startNum, ok := it.Start.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit start, got %T", it.Start)
	}
	if !floatEq(startNum.Value, 0) {
		t.Errorf("expected start 0, got %f", startNum.Value)
	}
	endNum, ok := it.End.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit end, got %T", it.End)
	}
	if !floatEq(endNum.Value, 8) {
		t.Errorf("expected end 8, got %f", endNum.Value)
	}
	if it.Step != nil {
		t.Errorf("expected no step, got %T", it.Step)
	}
	if fe.Body == nil {
		t.Fatal("expected body block")
	}
}

func TestParseForMultiIterator(t *testing.T) {
	expr := parseExpr(t, "for x in 0..3, y in 0..3 { sphere }")
	fe, ok := expr.(*ast.ForExpr)
	if !ok {
		t.Fatalf("expected ForExpr, got %T", expr)
	}
	if len(fe.Iterators) != 2 {
		t.Fatalf("expected 2 iterators, got %d", len(fe.Iterators))
	}
	if fe.Iterators[0].Name != "x" {
		t.Errorf("expected 'x', got %q", fe.Iterators[0].Name)
	}
	if fe.Iterators[1].Name != "y" {
		t.Errorf("expected 'y', got %q", fe.Iterators[1].Name)
	}
}

func TestParseForWithStep(t *testing.T) {
	expr := parseExpr(t, "for i in 0..1 step 0.1 { sphere }")
	fe, ok := expr.(*ast.ForExpr)
	if !ok {
		t.Fatalf("expected ForExpr, got %T", expr)
	}
	if fe.Iterators[0].Step == nil {
		t.Fatal("expected step")
	}
	stepNum, ok := fe.Iterators[0].Step.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit step, got %T", fe.Iterators[0].Step)
	}
	if !floatEq(stepNum.Value, 0.1) {
		t.Errorf("expected step 0.1, got %f", stepNum.Value)
	}
}

func TestParseIfExpr(t *testing.T) {
	expr := parseExpr(t, "if x > 1 { sphere } else { box }")
	ie, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", expr)
	}
	// Condition should be x > 1
	cond, ok := ie.Cond.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr cond, got %T", ie.Cond)
	}
	if cond.Op != ast.Gt {
		t.Errorf("expected Gt, got %s", cond.Op)
	}
	if ie.Then == nil {
		t.Fatal("expected then block")
	}
	if ie.Else == nil {
		t.Fatal("expected else block")
	}
}

func TestParseIfNoElse(t *testing.T) {
	expr := parseExpr(t, "if x > 1 { sphere }")
	ie, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", expr)
	}
	if ie.Else != nil {
		t.Error("expected no else block")
	}
}

func TestParseIfElseIf(t *testing.T) {
	expr := parseExpr(t, "if a { x } else if b { y } else { z }")
	ie, ok := expr.(*ast.IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr, got %T", expr)
	}
	elseIf, ok := ie.Else.(*ast.IfExpr)
	if !ok {
		t.Fatalf("expected IfExpr as else, got %T", ie.Else)
	}
	elseBlock, ok := elseIf.Else.(*ast.Block)
	if !ok {
		t.Fatalf("expected Block as final else, got %T", elseIf.Else)
	}
	if elseBlock.Result == nil {
		t.Fatal("expected result in final else block")
	}
}

// ---------------------------------------------------------------------------
// Settings (Task 1.13)
// ---------------------------------------------------------------------------

func TestParseSettingBgExpr(t *testing.T) {
	prog, diags := parseSource("bg #1a1a2e")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	ss, ok := prog.Statements[0].(*ast.SettingStmt)
	if !ok {
		t.Fatalf("expected SettingStmt, got %T", prog.Statements[0])
	}
	if ss.Kind != "bg" {
		t.Errorf("expected 'bg', got %q", ss.Kind)
	}
	_, ok = ss.Body.(*ast.HexColorLit)
	if !ok {
		t.Fatalf("expected HexColorLit body, got %T", ss.Body)
	}
}

func TestParseSettingRaymarch(t *testing.T) {
	prog, diags := parseSource("raymarch { steps: 128 }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "raymarch" {
		t.Errorf("expected 'raymarch', got %q", ss.Kind)
	}
	body, ok := ss.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body, got %T", ss.Body)
	}
	stepsExpr, ok := body["steps"]
	if !ok {
		t.Fatal("expected 'steps' key")
	}
	num, ok := stepsExpr.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit for steps, got %T", stepsExpr)
	}
	if !floatEq(num.Value, 128) {
		t.Errorf("expected 128, got %f", num.Value)
	}
}

func TestParseSettingDebug(t *testing.T) {
	prog, diags := parseSource("debug normals")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "debug" {
		t.Errorf("expected 'debug', got %q", ss.Kind)
	}
	mode, ok := ss.Body.(string)
	if !ok {
		t.Fatalf("expected string body, got %T", ss.Body)
	}
	if mode != "normals" {
		t.Errorf("expected 'normals', got %q", mode)
	}
}

func TestParseSettingLight(t *testing.T) {
	prog, diags := parseSource("light [-1, -1, -1]")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "light" {
		t.Errorf("expected 'light', got %q", ss.Kind)
	}
	// Body should be a VecLit wrapped in unary negation
	// Actually it's [-1, -1, -1] which is a VecLit with UnaryExpr elements
	vec, ok := ss.Body.(*ast.VecLit)
	if !ok {
		t.Fatalf("expected VecLit, got %T", ss.Body)
	}
	if len(vec.Elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(vec.Elems))
	}
}

func TestParseSettingLightBlock(t *testing.T) {
	prog, diags := parseSource("light { ambient: 0.1 }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	body, ok := ss.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body, got %T", ss.Body)
	}
	_, ok = body["ambient"]
	if !ok {
		t.Fatal("expected 'ambient' key")
	}
}

func TestParseSettingCamera(t *testing.T) {
	prog, diags := parseSource("camera { pos: [0, 2, 5], target: [0, 0, 0] }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "camera" {
		t.Errorf("expected 'camera', got %q", ss.Kind)
	}
	body, ok := ss.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body, got %T", ss.Body)
	}
	if _, ok := body["pos"]; !ok {
		t.Fatal("expected 'pos' key")
	}
	if _, ok := body["target"]; !ok {
		t.Fatal("expected 'target' key")
	}
}

func TestParseSettingCameraOneliner(t *testing.T) {
	prog, diags := parseSource("camera [0,2,5] -> [0,0,0]")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "camera" {
		t.Errorf("expected 'camera', got %q", ss.Kind)
	}
	body, ok := ss.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body for camera one-liner, got %T", ss.Body)
	}
	if _, ok := body["pos"]; !ok {
		t.Fatal("expected 'pos' key")
	}
	if _, ok := body["target"]; !ok {
		t.Fatal("expected 'target' key")
	}
}

func TestParseSettingMat(t *testing.T) {
	prog, diags := parseSource("mat gold = { color: [1, 0.843, 0], metallic: 1 }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "mat" {
		t.Errorf("expected 'mat', got %q", ss.Kind)
	}
	body, ok := ss.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body, got %T", ss.Body)
	}
	name, ok := body["name"].(string)
	if !ok || name != "gold" {
		t.Errorf("expected name 'gold', got %v", body["name"])
	}
}

func TestParseSettingPost(t *testing.T) {
	prog, diags := parseSource("post { gamma: 2.2 }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	if ss.Kind != "post" {
		t.Errorf("expected 'post', got %q", ss.Kind)
	}
	body, ok := ss.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map body, got %T", ss.Body)
	}
	gamma, ok := body["gamma"]
	if !ok {
		t.Fatal("expected 'gamma' key")
	}
	num, ok := gamma.(*ast.NumberLit)
	if !ok {
		t.Fatalf("expected NumberLit, got %T", gamma)
	}
	if !floatEq(num.Value, 2.2) {
		t.Errorf("expected 2.2, got %f", num.Value)
	}
}

func TestParseSettingRaymarchMultiKey(t *testing.T) {
	prog, diags := parseSource("raymarch { steps: 128, precision: 0.001 }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	body := ss.Body.(map[string]interface{})
	if _, ok := body["steps"]; !ok {
		t.Fatal("expected 'steps' key")
	}
	if _, ok := body["precision"]; !ok {
		t.Fatal("expected 'precision' key")
	}
}

func TestParseSettingNestedBlock(t *testing.T) {
	prog, diags := parseSource("light { sun { dir: [1, 0, 0] } }")
	assertNoDiags(t, diags)
	ss := prog.Statements[0].(*ast.SettingStmt)
	body := ss.Body.(map[string]interface{})
	sun, ok := body["sun"]
	if !ok {
		t.Fatal("expected 'sun' key")
	}
	sunMap, ok := sun.(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested map, got %T", sun)
	}
	if _, ok := sunMap["dir"]; !ok {
		t.Fatal("expected 'dir' key in sun block")
	}
}

// ---------------------------------------------------------------------------
// GLSL Escape (Task 1.14)
// ---------------------------------------------------------------------------

func TestParseGlslEscapeSimple(t *testing.T) {
	expr := parseExpr(t, "glsl(p) { return length(p) - 1.0; }")
	ge, ok := expr.(*ast.GlslEscape)
	if !ok {
		t.Fatalf("expected GlslEscape, got %T", expr)
	}
	if ge.Param != "p" {
		t.Errorf("expected param 'p', got %q", ge.Param)
	}
	// The code should contain "return" and "length"
	if ge.Code == "" {
		t.Fatal("expected non-empty code")
	}
	// Check that key parts of the GLSL are present
	if !containsAll(ge.Code, "return", "length", "1.0") {
		t.Errorf("expected GLSL code to contain return/length/1.0, got %q", ge.Code)
	}
}

func TestParseGlslEscapeNestedBraces(t *testing.T) {
	expr := parseExpr(t, "glsl(p) { if (p.x > 0.0) { return 1.0; } return 0.0; }")
	ge, ok := expr.(*ast.GlslEscape)
	if !ok {
		t.Fatalf("expected GlslEscape, got %T", expr)
	}
	if ge.Param != "p" {
		t.Errorf("expected param 'p', got %q", ge.Param)
	}
	// Should have captured the nested braces
	if !containsAll(ge.Code, "if", "return 1.0", "return 0.0") {
		t.Errorf("expected nested GLSL code, got %q", ge.Code)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, part := range parts {
		found := false
		for i := 0; i <= len(s)-len(part); i++ {
			if s[i:i+len(part)] == part {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Error recovery
// ---------------------------------------------------------------------------

func TestParseErrorRecoveryExtraRParen(t *testing.T) {
	prog, diags := parseSource("sphere )")
	// Should produce a partial AST and at least one diagnostic
	if len(prog.Statements) == 0 {
		t.Fatal("expected at least one statement (partial AST)")
	}
	if len(diags) == 0 {
		t.Error("expected at least one diagnostic for extra ')'")
	}
}

func TestParseErrorRecoveryUnexpected(t *testing.T) {
	prog, diags := parseSource(")")
	_ = prog
	if len(diags) == 0 {
		t.Error("expected diagnostics for standalone ')'")
	}
}

func TestParseEmptySource(t *testing.T) {
	prog, diags := parseSource("")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 0 {
		t.Errorf("expected 0 statements, got %d", len(prog.Statements))
	}
}

func TestParseNeverPanics(t *testing.T) {
	inputs := []string{
		"",
		")",
		"]",
		"}",
		".",
		"..",
		"...",
		"((",
		"[[[",
		"{{{",
		"for",
		"for in",
		"for x in",
		"for x in 0",
		"if",
		"if {",
		"glsl",
		"glsl(",
		"glsl(p)",
		"glsl(p) {",
		"= =",
		"+ + +",
		"sphere.at(",
		"sphere(",
		"42 42 42",
		"bg",
		"debug",
		"mat",
		"camera ->",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Parse panicked on %q: %v", input, r)
				}
			}()
			tokens, _ := lexer.Lex("test.chisel", input)
			Parse(tokens)
		})
	}
}

// ---------------------------------------------------------------------------
// Complex programs
// ---------------------------------------------------------------------------

func TestParseComplexProgram(t *testing.T) {
	source := `r = 1.5
base = sphere(r)
base.at(0, 1, 0).red`
	prog, diags := parseSource(source)
	assertNoDiags(t, diags)
	// Should have: assign, assign, expr
	if len(prog.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Statements))
	}
}

func TestParseSphereSubtractCylinder(t *testing.T) {
	expr := parseExpr(t, "sphere(2) - cylinder(0.5, 6)")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Subtract {
		t.Errorf("expected Subtract, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall on left, got %T", be.Left)
	}
	if left.Name != "sphere" {
		t.Errorf("expected 'sphere', got %q", left.Name)
	}
}

func TestParseProgramWithSettingsAndShapes(t *testing.T) {
	source := `bg #1a1a2e
sphere | box.at(2, 0, 0)`
	prog, diags := parseSource(source)
	assertNoDiags(t, diags)
	if len(prog.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Statements))
	}
	_, ok1 := prog.Statements[0].(*ast.SettingStmt)
	_, ok2 := prog.Statements[1].(*ast.ExprStmt)
	if !ok1 || !ok2 {
		t.Errorf("expected SettingStmt then ExprStmt, got %T, %T",
			prog.Statements[0], prog.Statements[1])
	}
}

func TestParseForWithMethodChain(t *testing.T) {
	expr := parseExpr(t, "for i in 0..8 { sphere.at(i, 0, 0) }")
	fe, ok := expr.(*ast.ForExpr)
	if !ok {
		t.Fatalf("expected ForExpr, got %T", expr)
	}
	if fe.Body == nil || fe.Body.Result == nil {
		t.Fatal("expected body with result")
	}
	mc, ok := fe.Body.Result.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall as body result, got %T", fe.Body.Result)
	}
	if mc.Name != "at" {
		t.Errorf("expected 'at', got %q", mc.Name)
	}
}

func TestParseFuncDefinitionWithBlock(t *testing.T) {
	source := `pillar(h) = {
  cylinder(0.3, h)
  sphere(0.4).at(0, h, 0)
}`
	prog, diags := parseSource(source)
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	assign, ok := prog.Statements[0].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("expected AssignStmt, got %T", prog.Statements[0])
	}
	if assign.Name != "pillar" {
		t.Errorf("expected 'pillar', got %q", assign.Name)
	}
	block, ok := assign.Value.(*ast.Block)
	if !ok {
		t.Fatalf("expected Block value, got %T", assign.Value)
	}
	// Block result should be a Union of the two shapes
	if block.Result == nil {
		t.Fatal("expected block result")
	}
	be, ok := block.Result.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr (Union) as result, got %T", block.Result)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union, got %s", be.Op)
	}
}

func TestParseVecInExpr(t *testing.T) {
	expr := parseExpr(t, "capsule([0,-1,0], [0,1,0], 0.5)")
	fc, ok := expr.(*ast.FuncCall)
	if !ok {
		t.Fatalf("expected FuncCall, got %T", expr)
	}
	if fc.Name != "capsule" {
		t.Errorf("expected 'capsule', got %q", fc.Name)
	}
	if len(fc.Args) != 3 {
		t.Fatalf("expected 3 args, got %d", len(fc.Args))
	}
	_, ok = fc.Args[0].Value.(*ast.VecLit)
	if !ok {
		t.Fatalf("expected VecLit as first arg, got %T", fc.Args[0].Value)
	}
}

// ---------------------------------------------------------------------------
// Hex color parsing unit tests
// ---------------------------------------------------------------------------

func TestParseHexColorFunc(t *testing.T) {
	cases := []struct {
		input      string
		r, g, b, a float64
	}{
		{"#ff0000", 1, 0, 0, 1},
		{"#00ff00", 0, 1, 0, 1},
		{"#0000ff", 0, 0, 1, 1},
		{"#ffffff", 1, 1, 1, 1},
		{"#000000", 0, 0, 0, 1},
		{"#f00", 1, 0, 0, 1},
		{"#0f0", 0, 1, 0, 1},
		{"#00f", 0, 0, 1, 1},
		{"#ff000080", 1, 0, 0, 128.0 / 255.0},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			r, g, b, a := parseHexColor(tc.input)
			if !floatEq(r, tc.r) || !floatEq(g, tc.g) || !floatEq(b, tc.b) || !floatEq(a, tc.a) {
				t.Errorf("parseHexColor(%q) = {%f,%f,%f,%f}, want {%f,%f,%f,%f}",
					tc.input, r, g, b, a, tc.r, tc.g, tc.b, tc.a)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Swizzle vs Method distinction
// ---------------------------------------------------------------------------

func TestSwizzleVsMethod(t *testing.T) {
	cases := []struct {
		input     string
		isSwizzle bool
		name      string
	}{
		{"v.x", true, "x"},
		{"v.xy", true, "xy"},
		{"v.xyz", true, "xyz"},
		{"v.xz", true, "xz"},
		{"v.rgb", true, "rgb"},
		{"v.r", true, "r"},
		{"v.scale", false, "scale"},
		{"v.at", false, "at"},
		{"v.red", false, "red"}, // "red" has 'e' and 'd' which are not swizzle chars
		{"v.color", false, "color"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			expr := parseExpr(t, tc.input)
			if tc.isSwizzle {
				sw, ok := expr.(*ast.Swizzle)
				if !ok {
					t.Fatalf("expected Swizzle for %q, got %T", tc.input, expr)
				}
				if sw.Components != tc.name {
					t.Errorf("expected components %q, got %q", tc.name, sw.Components)
				}
			} else {
				mc, ok := expr.(*ast.MethodCall)
				if !ok {
					t.Fatalf("expected MethodCall for %q, got %T", tc.input, expr)
				}
				if mc.Name != tc.name {
					t.Errorf("expected name %q, got %q", tc.name, mc.Name)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Walk integration
// ---------------------------------------------------------------------------

func TestWalkAfterParse(t *testing.T) {
	prog, _ := parseSource("sphere | box")
	count := 0
	ast.Walk(prog, func(n ast.Node) bool {
		count++
		return true
	})
	// Should visit: Program, ExprStmt, BinaryExpr, Ident(sphere), Ident(box) = 5
	if count < 4 {
		t.Errorf("expected at least 4 nodes visited, got %d", count)
	}
}

// ---------------------------------------------------------------------------
// Span tracking
// ---------------------------------------------------------------------------

func TestSpanTracking(t *testing.T) {
	prog, _ := parseSource("sphere")
	if len(prog.Statements) != 1 {
		t.Fatal("expected 1 statement")
	}
	es := prog.Statements[0].(*ast.ExprStmt)
	id := es.Expression.(*ast.Ident)
	span := id.NodeSpan()
	if span.Start.Line != 1 || span.Start.Col != 1 {
		t.Errorf("expected span start 1:1, got %d:%d", span.Start.Line, span.Start.Col)
	}
}

// ---------------------------------------------------------------------------
// Method call with named args
// ---------------------------------------------------------------------------

func TestParseMethodCallNamedArg(t *testing.T) {
	expr := parseExpr(t, "sphere.at(x: 2)")
	mc, ok := expr.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall, got %T", expr)
	}
	if len(mc.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d", len(mc.Args))
	}
	if mc.Args[0].Name != "x" {
		t.Errorf("expected named arg 'x', got %q", mc.Args[0].Name)
	}
}

// ---------------------------------------------------------------------------
// Expression with method chain and binary op
// ---------------------------------------------------------------------------

func TestParseMethodChainWithUnion(t *testing.T) {
	expr := parseExpr(t, "sphere.red | box.blue")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Op != ast.Union {
		t.Errorf("expected Union, got %s", be.Op)
	}
	left, ok := be.Left.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall on left, got %T", be.Left)
	}
	if left.Name != "red" {
		t.Errorf("expected 'red', got %q", left.Name)
	}
	right, ok := be.Right.(*ast.MethodCall)
	if !ok {
		t.Fatalf("expected MethodCall on right, got %T", be.Right)
	}
	if right.Name != "blue" {
		t.Errorf("expected 'blue', got %q", right.Name)
	}
}

// ---------------------------------------------------------------------------
// Parse assigns with complex values
// ---------------------------------------------------------------------------

func TestParseAssignWithExpr(t *testing.T) {
	prog, diags := parseSource("base = sphere(2) - cylinder(0.5, 6)")
	assertNoDiags(t, diags)
	assign := prog.Statements[0].(*ast.AssignStmt)
	if assign.Name != "base" {
		t.Errorf("expected 'base', got %q", assign.Name)
	}
	be, ok := assign.Value.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", assign.Value)
	}
	if be.Op != ast.Subtract {
		t.Errorf("expected Subtract, got %s", be.Op)
	}
}

// ---------------------------------------------------------------------------
// Multiple settings
// ---------------------------------------------------------------------------

func TestParseMultipleSettings(t *testing.T) {
	source := `raymarch { steps: 128 }
bg #1a1a2e
sphere`
	prog, diags := parseSource(source)
	assertNoDiags(t, diags)
	if len(prog.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Statements))
	}
	_, ok1 := prog.Statements[0].(*ast.SettingStmt)
	_, ok2 := prog.Statements[1].(*ast.SettingStmt)
	_, ok3 := prog.Statements[2].(*ast.ExprStmt)
	if !ok1 || !ok2 || !ok3 {
		t.Errorf("expected SettingStmt, SettingStmt, ExprStmt, got %T, %T, %T",
			prog.Statements[0], prog.Statements[1], prog.Statements[2])
	}
}

// ---------------------------------------------------------------------------
// Token coverage: ensure Parse handles all token types without panicking
// ---------------------------------------------------------------------------

func TestParseTokenCoverage(t *testing.T) {
	// Various token sequences to ensure no panics
	inputs := []string{
		"42",
		"3.14",
		"true",
		"false",
		`"string"`,
		"#ff0000",
		"sphere",
		"[1, 2]",
		"(42)",
		"{ sphere }",
		"a | b",
		"a |~0.3 b",
		"a |/0.3 b",
		"a - b",
		"a -~0.2 b",
		"a -/0.2 b",
		"a & b",
		"a &~0.1 b",
		"a &/0.1 b",
		"a + b",
		"a * b",
		"a / b",
		"a % b",
		"a == b",
		"a != b",
		"a < b",
		"a > b",
		"a <= b",
		"a >= b",
		"-a",
		"!a",
		"f(x)",
		"f(x: 1)",
		"a.b()",
		"a.xyz",
		"for i in 0..8 { a }",
		"if a { b }",
		"if a { b } else { c }",
		"bg #000",
		"debug normals",
		"glsl(p) { code }",
		"r = 1",
		"f(x) = x",
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			tokens, _ := lexer.Lex("test.chisel", input)
			prog, _ := Parse(tokens)
			if prog == nil {
				t.Fatal("expected non-nil program")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Implicit union not applied across assignments
// ---------------------------------------------------------------------------

func TestImplicitUnionNotAcrossAssignment(t *testing.T) {
	prog, diags := parseSource("sphere\nr = 1\nbox")
	assertNoDiags(t, diags)
	// Should be: ExprStmt(sphere), AssignStmt(r=1), ExprStmt(box)
	// No implicit union because assignment breaks the run.
	if len(prog.Statements) != 3 {
		t.Fatalf("expected 3 statements, got %d", len(prog.Statements))
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestParseSingleIdent(t *testing.T) {
	prog, diags := parseSource("sphere")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
}

func TestParseCommentOnly(t *testing.T) {
	prog, diags := parseSource("// just a comment")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 0 {
		t.Errorf("expected 0 statements, got %d", len(prog.Statements))
	}
}

func TestParseExprWithComment(t *testing.T) {
	prog, diags := parseSource("sphere // this is a sphere")
	assertNoDiags(t, diags)
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
}

func TestParseBlockEmpty(t *testing.T) {
	expr := parseExpr(t, "{ }")
	block, ok := expr.(*ast.Block)
	if !ok {
		t.Fatalf("expected Block, got %T", expr)
	}
	if len(block.Stmts) != 0 {
		t.Errorf("expected 0 stmts, got %d", len(block.Stmts))
	}
	if block.Result != nil {
		t.Error("expected nil result for empty block")
	}
}

func TestParseSmoothUnionIntRadius(t *testing.T) {
	// Blend radius as integer
	expr := parseExpr(t, "a |~1 b")
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		t.Fatalf("expected BinaryExpr, got %T", expr)
	}
	if be.Blend == nil {
		t.Fatal("expected blend radius")
	}
	if !floatEq(*be.Blend, 1.0) {
		t.Errorf("expected blend 1.0, got %f", *be.Blend)
	}
}

// ---------------------------------------------------------------------------
// Unused import prevention
// ---------------------------------------------------------------------------

var _ = token.TokEOF // use token package
