// Package format implements a canonical formatter for Chisel source code.
// It parses the source into an AST and reprints it with consistent style.
//
// Note: comments are not preserved in this initial implementation (the AST
// does not carry comment nodes). Comment preservation is a future improvement.
package format

import (
	"fmt"
	"math"
	"strings"

	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
)

// Format parses the source and returns the canonically formatted version.
// Returns an error if the source cannot be parsed.
func Format(source string) (string, error) {
	tokens, lexDiags := lexer.Lex("format.chisel", source)
	for _, d := range lexDiags {
		if d.Severity == diagnostic.Error {
			return "", fmt.Errorf("lex error: %s", d.Message)
		}
	}

	prog, parseDiags := parser.Parse(tokens)
	for _, d := range parseDiags {
		if d.Severity == diagnostic.Error {
			return "", fmt.Errorf("parse error: %s", d.Message)
		}
	}

	f := &formatter{}
	f.formatProgram(prog)

	result := f.buf.String()
	// Ensure trailing newline.
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

type formatter struct {
	buf    strings.Builder
	indent int
}

func (f *formatter) write(s string) {
	f.buf.WriteString(s)
}

func (f *formatter) writeIndent() {
	for i := 0; i < f.indent; i++ {
		f.buf.WriteString("  ")
	}
}

// ---------------------------------------------------------------------------
// Program
// ---------------------------------------------------------------------------

func (f *formatter) formatProgram(prog *ast.Program) {
	for i, stmt := range prog.Statements {
		if i > 0 {
			f.write("\n")
		}
		f.formatStmt(stmt)
	}
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

func (f *formatter) formatStmt(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		f.writeIndent()
		f.formatAssign(s)
	case *ast.ExprStmt:
		f.writeIndent()
		f.formatExpr(s.Expression)
	case *ast.SettingStmt:
		f.writeIndent()
		f.write(s.Kind)
		// Settings body formatting is simplified for MVP.
		if s.Body != nil {
			f.write(" ")
			switch body := s.Body.(type) {
			case ast.Expr:
				f.formatExpr(body)
			case string:
				f.write(body)
			default:
				f.write(fmt.Sprintf("%v", body))
			}
		}
	}
}

func (f *formatter) formatAssign(s *ast.AssignStmt) {
	f.write(s.Name)
	if s.Params != nil {
		f.write("(")
		for i, p := range s.Params {
			if i > 0 {
				f.write(", ")
			}
			f.write(p.Name)
			if p.Default != nil {
				f.write(" = ")
				f.formatExpr(p.Default)
			}
		}
		f.write(")")
	}
	f.write(" = ")
	f.formatExpr(s.Value)
}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

func (f *formatter) formatExpr(expr ast.Expr) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *ast.NumberLit:
		f.formatNumber(e.Value)
	case *ast.BoolLit:
		if e.Value {
			f.write("true")
		} else {
			f.write("false")
		}
	case *ast.StringLit:
		f.write(fmt.Sprintf("%q", e.Value))
	case *ast.HexColorLit:
		f.formatHexColor(e)
	case *ast.Ident:
		f.write(e.Name)
	case *ast.VecLit:
		f.formatVecLit(e)
	case *ast.BinaryExpr:
		f.formatBinaryExpr(e)
	case *ast.UnaryExpr:
		f.formatUnaryExpr(e)
	case *ast.FuncCall:
		f.formatFuncCall(e)
	case *ast.MethodCall:
		f.formatMethodCall(e)
	case *ast.Swizzle:
		f.formatExpr(e.Receiver)
		f.write(".")
		f.write(e.Components)
	case *ast.Block:
		f.formatBlock(e)
	case *ast.ForExpr:
		f.formatFor(e)
	case *ast.IfExpr:
		f.formatIf(e)
	case *ast.GlslEscape:
		f.write("glsl(")
		f.write(e.Param)
		f.write(") { ")
		f.write(e.Code)
		f.write(" }")
	}
}

func (f *formatter) formatNumber(v float64) {
	// Format integers without decimal point.
	if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
		if v >= 0 {
			f.write(fmt.Sprintf("%g", v))
		} else {
			f.write(fmt.Sprintf("%g", v))
		}
		return
	}
	f.write(fmt.Sprintf("%g", v))
}

func (f *formatter) formatHexColor(e *ast.HexColorLit) {
	r := int(math.Round(e.R * 255))
	g := int(math.Round(e.G * 255))
	b := int(math.Round(e.B * 255))
	if e.A < 1.0 {
		a := int(math.Round(e.A * 255))
		f.write(fmt.Sprintf("#%02x%02x%02x%02x", r, g, b, a))
	} else {
		f.write(fmt.Sprintf("#%02x%02x%02x", r, g, b))
	}
}

func (f *formatter) formatVecLit(e *ast.VecLit) {
	f.write("[")
	for i, elem := range e.Elems {
		if i > 0 {
			f.write(", ")
		}
		f.formatExpr(elem)
	}
	f.write("]")
}

func (f *formatter) formatBinaryExpr(e *ast.BinaryExpr) {
	f.formatExpr(e.Left)
	f.write(" ")
	f.write(binaryOpString(e.Op))
	if e.Blend != nil && isSmoothOrChamferOp(e.Op) {
		f.formatNumber(*e.Blend)
	}
	f.write(" ")
	f.formatExpr(e.Right)
}

func (f *formatter) formatUnaryExpr(e *ast.UnaryExpr) {
	switch e.Op {
	case ast.Neg:
		f.write("-")
	case ast.Not:
		f.write("!")
	}
	f.formatExpr(e.Operand)
}

func (f *formatter) formatFuncCall(e *ast.FuncCall) {
	f.write(e.Name)
	f.write("(")
	f.formatArgs(e.Args)
	f.write(")")
}

func (f *formatter) formatMethodCall(e *ast.MethodCall) {
	f.formatExpr(e.Receiver)
	f.write(".")
	f.write(e.Name)
	if len(e.Args) > 0 {
		f.write("(")
		f.formatArgs(e.Args)
		f.write(")")
	}
}

func (f *formatter) formatArgs(args []ast.Arg) {
	for i, arg := range args {
		if i > 0 {
			f.write(", ")
		}
		if arg.Name != "" {
			f.write(arg.Name)
			f.write(": ")
		}
		f.formatExpr(arg.Value)
	}
}

func (f *formatter) formatBlock(e *ast.Block) {
	f.write("{\n")
	f.indent++
	for _, s := range e.Stmts {
		f.formatStmt(s)
		f.write("\n")
	}
	if e.Result != nil {
		f.writeIndent()
		f.formatExpr(e.Result)
		f.write("\n")
	}
	f.indent--
	f.writeIndent()
	f.write("}")
}

func (f *formatter) formatFor(e *ast.ForExpr) {
	f.write("for ")
	for i, it := range e.Iterators {
		if i > 0 {
			f.write(", ")
		}
		f.write(it.Name)
		f.write(" in ")
		f.formatExpr(it.Start)
		f.write("..")
		f.formatExpr(it.End)
		if it.Step != nil {
			f.write(" step ")
			f.formatExpr(it.Step)
		}
	}
	f.write(" ")
	if e.Body != nil {
		f.formatBlock(e.Body)
	}
}

func (f *formatter) formatIf(e *ast.IfExpr) {
	f.write("if ")
	f.formatExpr(e.Cond)
	f.write(" ")
	if e.Then != nil {
		f.formatBlock(e.Then)
	}
	if e.Else != nil {
		f.write(" else ")
		switch el := e.Else.(type) {
		case *ast.IfExpr:
			f.formatIf(el)
		case *ast.Block:
			f.formatBlock(el)
		default:
			f.formatExpr(e.Else)
		}
	}
}

// ---------------------------------------------------------------------------
// Binary op formatting helpers
// ---------------------------------------------------------------------------

func binaryOpString(op ast.BinaryOp) string {
	switch op {
	case ast.Union:
		return "|"
	case ast.SmoothUnion:
		return "|~"
	case ast.ChamferUnion:
		return "|/"
	case ast.Subtract:
		return "-"
	case ast.SmoothSubtract:
		return "-~"
	case ast.ChamferSubtract:
		return "-/"
	case ast.Intersect:
		return "&"
	case ast.SmoothIntersect:
		return "&~"
	case ast.ChamferIntersect:
		return "&/"
	case ast.Add:
		return "+"
	case ast.Sub:
		return "-"
	case ast.Mul:
		return "*"
	case ast.Div:
		return "/"
	case ast.Mod:
		return "%"
	case ast.Eq:
		return "=="
	case ast.Neq:
		return "!="
	case ast.Lt:
		return "<"
	case ast.Gt:
		return ">"
	case ast.Lte:
		return "<="
	case ast.Gte:
		return ">="
	default:
		return "?"
	}
}

func isSmoothOrChamferOp(op ast.BinaryOp) bool {
	switch op {
	case ast.SmoothUnion, ast.ChamferUnion,
		ast.SmoothSubtract, ast.ChamferSubtract,
		ast.SmoothIntersect, ast.ChamferIntersect:
		return true
	}
	return false
}
