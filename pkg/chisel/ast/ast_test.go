package ast

import (
	"fmt"
	"reflect"
	"testing"

	"asciishader/pkg/chisel/token"
)

// span returns a zero-value span (used for test AST construction).
func span() token.Span { return token.Span{} }

// typeName returns the short (unqualified) type name of a node.
func typeName(n Node) string {
	t := reflect.TypeOf(n)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t.Name()
}

// TestBuildAndWalk_UnionWithMethodCall constructs the AST for
//
//	sphere | box.at(2, 0, 0)
//
// and verifies the depth-first Walk visit order.
func TestBuildAndWalk_UnionWithMethodCall(t *testing.T) {
	// Build AST by hand:
	//
	//   BinaryExpr (Union)
	//     Left:  Ident "sphere"
	//     Right: MethodCall
	//              Receiver: Ident "box"
	//              Name:     "at"
	//              Args:     [NumberLit(2), NumberLit(0), NumberLit(0)]

	sphere := &Ident{BaseNode: BaseNode{Span: span()}, Name: "sphere"}

	box := &Ident{BaseNode: BaseNode{Span: span()}, Name: "box"}
	arg0 := &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 2}
	arg1 := &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 0}
	arg2 := &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 0}
	boxAt := &MethodCall{
		BaseNode: BaseNode{Span: span()},
		Receiver: box,
		Name:     "at",
		Args: []Arg{
			{Value: arg0},
			{Value: arg1},
			{Value: arg2},
		},
	}

	union := &BinaryExpr{
		BaseNode: BaseNode{Span: span()},
		Left:     sphere,
		Op:       Union,
		Right:    boxAt,
	}

	root := &Program{
		BaseNode:   BaseNode{Span: span()},
		Statements: []Statement{&ExprStmt{BaseNode: BaseNode{Span: span()}, Expression: union}},
	}

	// Walk and collect type names.
	var visited []string
	Walk(root, func(n Node) bool {
		visited = append(visited, typeName(n))
		return true
	})

	expected := []string{
		"Program",
		"ExprStmt",
		"BinaryExpr",
		"Ident",      // sphere
		"MethodCall", // box.at(...)
		"Ident",      // box
		"NumberLit",  // 2
		"NumberLit",  // 0
		"NumberLit",  // 0
	}

	if !reflect.DeepEqual(visited, expected) {
		t.Errorf("Walk order mismatch\n  got:  %v\n  want: %v", visited, expected)
	}
}

// TestWalkSkipsChildren verifies that returning false from the visitor
// prevents Walk from descending into children.
func TestWalkSkipsChildren(t *testing.T) {
	inner := &Ident{BaseNode: BaseNode{Span: span()}, Name: "a"}
	outer := &UnaryExpr{
		BaseNode: BaseNode{Span: span()},
		Op:       Neg,
		Operand:  inner,
	}

	var visited []string
	Walk(outer, func(n Node) bool {
		visited = append(visited, typeName(n))
		// Stop descending after visiting UnaryExpr.
		return false
	})

	expected := []string{"UnaryExpr"}
	if !reflect.DeepEqual(visited, expected) {
		t.Errorf("expected skip to prevent visiting children\n  got:  %v\n  want: %v", visited, expected)
	}
}

// TestBinaryOpString verifies the String() method on BinaryOp.
func TestBinaryOpString(t *testing.T) {
	tests := []struct {
		op   BinaryOp
		want string
	}{
		{Union, "Union"},
		{SmoothUnion, "SmoothUnion"},
		{ChamferUnion, "ChamferUnion"},
		{Subtract, "Subtract"},
		{SmoothSubtract, "SmoothSubtract"},
		{ChamferSubtract, "ChamferSubtract"},
		{Intersect, "Intersect"},
		{SmoothIntersect, "SmoothIntersect"},
		{ChamferIntersect, "ChamferIntersect"},
		{Add, "Add"},
		{Sub, "Sub"},
		{Mul, "Mul"},
		{Div, "Div"},
		{Mod, "Mod"},
		{Eq, "Eq"},
		{Neq, "Neq"},
		{Lt, "Lt"},
		{Gt, "Gt"},
		{Lte, "Lte"},
		{Gte, "Gte"},
	}
	for _, tc := range tests {
		if got := tc.op.String(); got != tc.want {
			t.Errorf("BinaryOp(%d).String() = %q, want %q", tc.op, got, tc.want)
		}
	}
	// Out-of-range value returns "Unknown".
	if got := BinaryOp(999).String(); got != "Unknown" {
		t.Errorf("BinaryOp(999).String() = %q, want %q", got, "Unknown")
	}
}

// TestUnaryOpString verifies the String() method on UnaryOp.
func TestUnaryOpString(t *testing.T) {
	if got := Neg.String(); got != "Neg" {
		t.Errorf("Neg.String() = %q, want %q", got, "Neg")
	}
	if got := Not.String(); got != "Not" {
		t.Errorf("Not.String() = %q, want %q", got, "Not")
	}
	if got := UnaryOp(99).String(); got != "Unknown" {
		t.Errorf("UnaryOp(99).String() = %q, want %q", got, "Unknown")
	}
}

// TestNodeSpan verifies that BaseNode.NodeSpan returns the embedded span.
func TestNodeSpan(t *testing.T) {
	s := token.Span{
		Start: token.Position{File: "test.chisel", Line: 1, Col: 1, Offset: 0},
		End:   token.Position{File: "test.chisel", Line: 1, Col: 7, Offset: 6},
	}
	n := &Ident{BaseNode: BaseNode{Span: s}, Name: "sphere"}
	got := n.NodeSpan()
	if got != s {
		t.Errorf("NodeSpan() = %+v, want %+v", got, s)
	}
}

// TestWalkForExpr verifies Walk traversal of ForExpr with iterators and body.
func TestWalkForExpr(t *testing.T) {
	start := &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 0}
	end := &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 8}
	body := &Block{
		BaseNode: BaseNode{Span: span()},
		Result:   &Ident{BaseNode: BaseNode{Span: span()}, Name: "sphere"},
	}
	forExpr := &ForExpr{
		BaseNode: BaseNode{Span: span()},
		Iterators: []Iterator{
			{Name: "i", Start: start, End: end},
		},
		Body: body,
	}

	var visited []string
	Walk(forExpr, func(n Node) bool {
		visited = append(visited, typeName(n))
		return true
	})

	expected := []string{
		"ForExpr",
		"NumberLit", // start
		"NumberLit", // end
		"Block",
		"Ident", // sphere (block result)
	}
	if !reflect.DeepEqual(visited, expected) {
		t.Errorf("Walk ForExpr order mismatch\n  got:  %v\n  want: %v", visited, expected)
	}
}

// TestWalkIfExpr verifies Walk traversal of IfExpr with else-if chain.
func TestWalkIfExpr(t *testing.T) {
	cond := &BoolLit{BaseNode: BaseNode{Span: span()}, Value: true}
	then := &Block{
		BaseNode: BaseNode{Span: span()},
		Result:   &Ident{BaseNode: BaseNode{Span: span()}, Name: "a"},
	}
	elseBlock := &Block{
		BaseNode: BaseNode{Span: span()},
		Result:   &Ident{BaseNode: BaseNode{Span: span()}, Name: "b"},
	}
	ifExpr := &IfExpr{
		BaseNode: BaseNode{Span: span()},
		Cond:     cond,
		Then:     then,
		Else:     elseBlock,
	}

	var visited []string
	Walk(ifExpr, func(n Node) bool {
		visited = append(visited, typeName(n))
		return true
	})

	expected := []string{
		"IfExpr",
		"BoolLit", // cond
		"Block",   // then
		"Ident",   // a
		"Block",   // else
		"Ident",   // b
	}
	if !reflect.DeepEqual(visited, expected) {
		t.Errorf("Walk IfExpr order mismatch\n  got:  %v\n  want: %v", visited, expected)
	}
}

// TestWalkNilNode verifies that Walk handles nil nodes gracefully.
func TestWalkNilNode(t *testing.T) {
	// Should not panic.
	Walk(nil, func(n Node) bool {
		t.Error("visitor should not be called for nil node")
		return true
	})
}

// TestAllExprTypes verifies that all Expr types satisfy the Node interface
// and that Walk visits them.
func TestAllExprTypes(t *testing.T) {
	blend := 0.5
	exprs := []Expr{
		&NumberLit{BaseNode: BaseNode{Span: span()}, Value: 42},
		&BoolLit{BaseNode: BaseNode{Span: span()}, Value: true},
		&StringLit{BaseNode: BaseNode{Span: span()}, Value: "hello"},
		&VecLit{BaseNode: BaseNode{Span: span()}, Elems: []Expr{
			&NumberLit{BaseNode: BaseNode{Span: span()}, Value: 1},
		}},
		&HexColorLit{BaseNode: BaseNode{Span: span()}, R: 1, G: 0, B: 0, A: 1},
		&Ident{BaseNode: BaseNode{Span: span()}, Name: "x"},
		&BinaryExpr{BaseNode: BaseNode{Span: span()},
			Left:  &Ident{BaseNode: BaseNode{Span: span()}, Name: "a"},
			Op:    SmoothUnion,
			Right: &Ident{BaseNode: BaseNode{Span: span()}, Name: "b"},
			Blend: &blend,
		},
		&UnaryExpr{BaseNode: BaseNode{Span: span()}, Op: Neg,
			Operand: &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 1}},
		&MethodCall{BaseNode: BaseNode{Span: span()},
			Receiver: &Ident{BaseNode: BaseNode{Span: span()}, Name: "s"},
			Name:     "at",
			Args:     []Arg{{Value: &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 0}}},
		},
		&Swizzle{BaseNode: BaseNode{Span: span()},
			Receiver:   &Ident{BaseNode: BaseNode{Span: span()}, Name: "p"},
			Components: "xz",
		},
		&FuncCall{BaseNode: BaseNode{Span: span()}, Name: "sphere",
			Args: []Arg{{Value: &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 1}}},
		},
		&Block{BaseNode: BaseNode{Span: span()}},
		&ForExpr{BaseNode: BaseNode{Span: span()},
			Iterators: []Iterator{{
				Name:  "i",
				Start: &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 0},
				End:   &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 1},
			}},
			Body: &Block{BaseNode: BaseNode{Span: span()}},
		},
		&IfExpr{BaseNode: BaseNode{Span: span()},
			Cond: &BoolLit{BaseNode: BaseNode{Span: span()}, Value: true},
			Then: &Block{BaseNode: BaseNode{Span: span()}},
		},
		&GlslEscape{BaseNode: BaseNode{Span: span()}, Param: "p", Code: "return 1.0;"},
	}

	for _, e := range exprs {
		name := typeName(e)
		t.Run(name, func(t *testing.T) {
			// Verify Node interface is satisfied.
			var _ Node = e
			_ = e.NodeSpan()

			// Verify Walk visits the root.
			visited := false
			Walk(e, func(n Node) bool {
				if n == e {
					visited = true
				}
				return true
			})
			if !visited {
				t.Errorf("Walk did not visit %s", name)
			}
		})
	}
}

// TestWalkAssignStmt verifies that Walk handles AssignStmt including params with defaults.
func TestWalkAssignStmt(t *testing.T) {
	defVal := &NumberLit{BaseNode: BaseNode{Span: span()}, Value: 1}
	body := &Ident{BaseNode: BaseNode{Span: span()}, Name: "x"}
	assign := &AssignStmt{
		BaseNode: BaseNode{Span: span()},
		Name:     "f",
		Params: []Param{
			{Name: "x"},
			{Name: "y", Default: defVal},
		},
		Value: body,
	}

	var visited []string
	Walk(assign, func(n Node) bool {
		visited = append(visited, fmt.Sprintf("%s", typeName(n)))
		return true
	})

	expected := []string{
		"AssignStmt",
		"NumberLit", // default value for param "y"
		"Ident",     // body
	}
	if !reflect.DeepEqual(visited, expected) {
		t.Errorf("Walk AssignStmt order mismatch\n  got:  %v\n  want: %v", visited, expected)
	}
}
