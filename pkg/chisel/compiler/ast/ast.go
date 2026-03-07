// Package ast defines the abstract syntax tree types for the Chisel language.
package ast

import "asciishader/pkg/chisel/compiler/token"

// ---------------------------------------------------------------------------
// Node interface
// ---------------------------------------------------------------------------

// Node is implemented by every AST node. It provides the source span.
type Node interface {
	NodeSpan() token.Span
}

// BaseNode is embedded by all concrete AST nodes to satisfy the Node interface.
type BaseNode struct {
	Span token.Span
}

// NodeSpan returns the source span of the node.
func (b BaseNode) NodeSpan() token.Span { return b.Span }

// ---------------------------------------------------------------------------
// Top-level program
// ---------------------------------------------------------------------------

// Program is the root AST node containing a list of statements.
type Program struct {
	BaseNode
	Statements []Statement
}

// ---------------------------------------------------------------------------
// Statement interface
// ---------------------------------------------------------------------------

// Statement is a marker interface for all statement nodes.
type Statement interface {
	Node
	stmtNode()
}

// ---------------------------------------------------------------------------
// Concrete statement types
// ---------------------------------------------------------------------------

// Param represents a function parameter with an optional default value.
type Param struct {
	Name    string
	Default Expr // nil when no default
}

// AssignStmt represents a variable or function assignment.
//
//	name = expr              (variable: Params is nil)
//	name(a, b = 1) = expr   (function: Params is non-nil)
type AssignStmt struct {
	BaseNode
	Name   string
	Params []Param // nil for variable assignment, non-nil for function def
	Value  Expr
}

func (*AssignStmt) stmtNode() {}

// ExprStmt wraps an expression that appears at statement level.
type ExprStmt struct {
	BaseNode
	Expression Expr
}

func (*ExprStmt) stmtNode() {}

// SettingStmt represents a settings block (light, camera, bg, etc.).
type SettingStmt struct {
	BaseNode
	Kind string      // "light", "camera", "bg", "raymarch", "post", "debug", "mat"
	Body interface{} // parsed into specific setting structs
}

func (*SettingStmt) stmtNode() {}

// ---------------------------------------------------------------------------
// Expr interface
// ---------------------------------------------------------------------------

// Expr is a marker interface for all expression nodes.
type Expr interface {
	Node
	exprNode()
}

// ---------------------------------------------------------------------------
// Concrete expression types
// ---------------------------------------------------------------------------

// NumberLit represents a numeric literal (int or float).
type NumberLit struct {
	BaseNode
	Value float64
}

func (*NumberLit) exprNode() {}

// BoolLit represents a boolean literal (true/false).
type BoolLit struct {
	BaseNode
	Value bool
}

func (*BoolLit) exprNode() {}

// StringLit represents a quoted string literal.
type StringLit struct {
	BaseNode
	Value string
}

func (*StringLit) exprNode() {}

// VecLit represents a vector literal: [x, y] or [x, y, z].
type VecLit struct {
	BaseNode
	Elems []Expr
}

func (*VecLit) exprNode() {}

// HexColorLit represents a hex color literal such as #ff0000.
// Components are normalised to [0, 1].
type HexColorLit struct {
	BaseNode
	R, G, B, A float64
}

func (*HexColorLit) exprNode() {}

// Ident represents an identifier reference.
type Ident struct {
	BaseNode
	Name string
}

func (*Ident) exprNode() {}

// BinaryExpr represents a binary operation (boolean SDF ops, arithmetic, comparison).
type BinaryExpr struct {
	BaseNode
	Left  Expr
	Op    BinaryOp
	Right Expr
	Blend *float64 // smooth/chamfer radius; nil for sharp ops
}

func (*BinaryExpr) exprNode() {}

// UnaryExpr represents a unary operation (negation, logical not).
type UnaryExpr struct {
	BaseNode
	Op      UnaryOp
	Operand Expr
}

func (*UnaryExpr) exprNode() {}

// Arg represents a function/method argument, optionally named.
type Arg struct {
	Name  string // "" for positional arguments
	Value Expr
}

// MethodCall represents a method invocation on a receiver.
type MethodCall struct {
	BaseNode
	Receiver Expr
	Name     string
	Args     []Arg
}

func (*MethodCall) exprNode() {}

// Swizzle represents a swizzle access on a receiver (e.g. p.xz).
type Swizzle struct {
	BaseNode
	Receiver   Expr
	Components string // e.g. "xz", "xy", "xxx"
}

func (*Swizzle) exprNode() {}

// FuncCall represents a top-level function call.
type FuncCall struct {
	BaseNode
	Name string
	Args []Arg
}

func (*FuncCall) exprNode() {}

// Block represents a brace-delimited block of statements with an optional result expression.
type Block struct {
	BaseNode
	Stmts  []Statement
	Result Expr // final expression (the block's value); may be nil
}

func (*Block) exprNode() {}

// Iterator represents a single loop iterator in a for expression.
type Iterator struct {
	Name  string
	Start Expr
	End   Expr
	Step  Expr // nil when using default step
}

// ForExpr represents a for loop expression.
type ForExpr struct {
	BaseNode
	Iterators []Iterator
	Body      *Block
}

func (*ForExpr) exprNode() {}

// IfExpr represents an if/else expression.
type IfExpr struct {
	BaseNode
	Cond Expr
	Then *Block
	Else Expr // nil, *Block, or *IfExpr (else if chain)
}

func (*IfExpr) exprNode() {}

// GlslEscape represents a raw GLSL escape block: glsl(param) { code }.
type GlslEscape struct {
	BaseNode
	Param string // e.g. "p"
	Code  string // raw GLSL source
}

func (*GlslEscape) exprNode() {}

// ---------------------------------------------------------------------------
// BinaryOp enum
// ---------------------------------------------------------------------------

// BinaryOp identifies the operator in a BinaryExpr.
type BinaryOp int

const (
	Union            BinaryOp = iota // |
	SmoothUnion                      // |~
	ChamferUnion                     // |/
	Subtract                         // - (SDF)
	SmoothSubtract                   // -~
	ChamferSubtract                  // -/
	Intersect                        // &
	SmoothIntersect                  // &~
	ChamferIntersect                 // &/
	Add                              // +
	Sub                              // - (arithmetic)
	Mul                              // *
	Div                              // /
	Mod                              // %
	Eq                               // ==
	Neq                              // !=
	Lt                               // <
	Gt                               // >
	Lte                              // <=
	Gte                              // >=
)

var binaryOpNames = [...]string{
	Union:            "Union",
	SmoothUnion:      "SmoothUnion",
	ChamferUnion:     "ChamferUnion",
	Subtract:         "Subtract",
	SmoothSubtract:   "SmoothSubtract",
	ChamferSubtract:  "ChamferSubtract",
	Intersect:        "Intersect",
	SmoothIntersect:  "SmoothIntersect",
	ChamferIntersect: "ChamferIntersect",
	Add:              "Add",
	Sub:              "Sub",
	Mul:              "Mul",
	Div:              "Div",
	Mod:              "Mod",
	Eq:               "Eq",
	Neq:              "Neq",
	Lt:               "Lt",
	Gt:               "Gt",
	Lte:              "Lte",
	Gte:              "Gte",
}

// String returns the human-readable name of the binary operator.
func (op BinaryOp) String() string {
	if int(op) < len(binaryOpNames) {
		return binaryOpNames[op]
	}
	return "Unknown"
}

// ---------------------------------------------------------------------------
// UnaryOp enum
// ---------------------------------------------------------------------------

// UnaryOp identifies the operator in a UnaryExpr.
type UnaryOp int

const (
	Neg UnaryOp = iota // -
	Not                // !
)

var unaryOpNames = [...]string{
	Neg: "Neg",
	Not: "Not",
}

// String returns the human-readable name of the unary operator.
func (op UnaryOp) String() string {
	if int(op) < len(unaryOpNames) {
		return unaryOpNames[op]
	}
	return "Unknown"
}

// ---------------------------------------------------------------------------
// Walk — depth-first AST traversal
// ---------------------------------------------------------------------------

// Walk traverses the AST depth-first, calling fn for each node.
// If fn returns false the children of that node are skipped.
func Walk(node Node, fn func(Node) bool) {
	if node == nil {
		return
	}
	if !fn(node) {
		return
	}

	switch n := node.(type) {
	case *Program:
		for _, s := range n.Statements {
			Walk(s, fn)
		}

	// -- Statements ---------------------------------------------------------

	case *AssignStmt:
		for _, p := range n.Params {
			if p.Default != nil {
				Walk(p.Default, fn)
			}
		}
		Walk(n.Value, fn)

	case *ExprStmt:
		Walk(n.Expression, fn)

	case *SettingStmt:
		// Body is interface{}; settings-specific walking is handled elsewhere.

	// -- Expressions --------------------------------------------------------

	case *NumberLit:
		// leaf

	case *BoolLit:
		// leaf

	case *StringLit:
		// leaf

	case *VecLit:
		for _, e := range n.Elems {
			Walk(e, fn)
		}

	case *HexColorLit:
		// leaf

	case *Ident:
		// leaf

	case *BinaryExpr:
		Walk(n.Left, fn)
		Walk(n.Right, fn)

	case *UnaryExpr:
		Walk(n.Operand, fn)

	case *MethodCall:
		Walk(n.Receiver, fn)
		for _, a := range n.Args {
			Walk(a.Value, fn)
		}

	case *Swizzle:
		Walk(n.Receiver, fn)

	case *FuncCall:
		for _, a := range n.Args {
			Walk(a.Value, fn)
		}

	case *Block:
		for _, s := range n.Stmts {
			Walk(s, fn)
		}
		if n.Result != nil {
			Walk(n.Result, fn)
		}

	case *ForExpr:
		for _, it := range n.Iterators {
			Walk(it.Start, fn)
			Walk(it.End, fn)
			if it.Step != nil {
				Walk(it.Step, fn)
			}
		}
		if n.Body != nil {
			Walk(n.Body, fn)
		}

	case *IfExpr:
		Walk(n.Cond, fn)
		if n.Then != nil {
			Walk(n.Then, fn)
		}
		if n.Else != nil {
			Walk(n.Else, fn)
		}

	case *GlslEscape:
		// leaf (code is raw string, not walked)
	}
}
