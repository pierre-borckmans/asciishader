package ast

import (
	"fmt"
	"strings"
)

// Print writes a human-readable representation of the AST to a string builder.
func Print(w *strings.Builder, node Node, depth int) {
	indent := strings.Repeat("  ", depth)

	switch n := node.(type) {
	case *Program:
		w.WriteString(indent)
		w.WriteString("Program\n")
		for _, s := range n.Statements {
			Print(w, s, depth+1)
		}

	case *AssignStmt:
		w.WriteString(indent)
		w.WriteString("Assign ")
		w.WriteString(n.Name)
		if n.Params != nil {
			w.WriteString("(")
			for i, p := range n.Params {
				if i > 0 {
					w.WriteString(", ")
				}
				w.WriteString(p.Name)
				if p.Default != nil {
					w.WriteString("=...")
				}
			}
			w.WriteString(")")
		}
		w.WriteString("\n")
		PrintExpr(w, n.Value, depth+1)

	case *ExprStmt:
		w.WriteString(indent)
		w.WriteString("ExprStmt\n")
		PrintExpr(w, n.Expression, depth+1)

	case *SettingStmt:
		w.WriteString(indent)
		fmt.Fprintf(w, "Setting %s\n", n.Kind)
	}
}

// PrintExpr writes a human-readable representation of an expression.
func PrintExpr(w *strings.Builder, expr Expr, depth int) {
	if expr == nil {
		return
	}
	indent := strings.Repeat("  ", depth)

	switch e := expr.(type) {
	case *NumberLit:
		fmt.Fprintf(w, "%sNumber(%g)\n", indent, e.Value)

	case *BoolLit:
		fmt.Fprintf(w, "%sBool(%t)\n", indent, e.Value)

	case *StringLit:
		fmt.Fprintf(w, "%sString(%q)\n", indent, e.Value)

	case *HexColorLit:
		fmt.Fprintf(w, "%sHexColor(%.2f, %.2f, %.2f, %.2f)\n", indent, e.R, e.G, e.B, e.A)

	case *Ident:
		fmt.Fprintf(w, "%sIdent(%s)\n", indent, e.Name)

	case *VecLit:
		fmt.Fprintf(w, "%sVec[\n", indent)
		for _, elem := range e.Elems {
			PrintExpr(w, elem, depth+1)
		}
		fmt.Fprintf(w, "%s]\n", indent)

	case *BinaryExpr:
		fmt.Fprintf(w, "%sBinary(%s)\n", indent, e.Op)
		PrintExpr(w, e.Left, depth+1)
		PrintExpr(w, e.Right, depth+1)

	case *UnaryExpr:
		fmt.Fprintf(w, "%sUnary(%s)\n", indent, e.Op)
		PrintExpr(w, e.Operand, depth+1)

	case *FuncCall:
		fmt.Fprintf(w, "%sCall(%s)\n", indent, e.Name)
		for _, a := range e.Args {
			if a.Name != "" {
				fmt.Fprintf(w, "%s  %s:\n", indent, a.Name)
				PrintExpr(w, a.Value, depth+2)
			} else {
				PrintExpr(w, a.Value, depth+1)
			}
		}

	case *MethodCall:
		fmt.Fprintf(w, "%sMethod(.%s)\n", indent, e.Name)
		PrintExpr(w, e.Receiver, depth+1)
		for _, a := range e.Args {
			if a.Name != "" {
				fmt.Fprintf(w, "%s  %s:\n", indent, a.Name)
				PrintExpr(w, a.Value, depth+2)
			} else {
				PrintExpr(w, a.Value, depth+1)
			}
		}

	case *Swizzle:
		fmt.Fprintf(w, "%sSwizzle(.%s)\n", indent, e.Components)
		PrintExpr(w, e.Receiver, depth+1)

	case *Block:
		fmt.Fprintf(w, "%sBlock\n", indent)
		for _, s := range e.Stmts {
			Print(w, s, depth+1)
		}
		if e.Result != nil {
			fmt.Fprintf(w, "%s  result:\n", indent)
			PrintExpr(w, e.Result, depth+2)
		}

	case *ForExpr:
		fmt.Fprintf(w, "%sFor\n", indent)
		for _, it := range e.Iterators {
			fmt.Fprintf(w, "%s  %s in\n", indent, it.Name)
			PrintExpr(w, it.Start, depth+2)
			fmt.Fprintf(w, "%s  ..\n", indent)
			PrintExpr(w, it.End, depth+2)
			if it.Step != nil {
				fmt.Fprintf(w, "%s  step\n", indent)
				PrintExpr(w, it.Step, depth+2)
			}
		}
		if e.Body != nil {
			PrintExpr(w, e.Body, depth+1)
		}

	case *IfExpr:
		fmt.Fprintf(w, "%sIf\n", indent)
		fmt.Fprintf(w, "%s  cond:\n", indent)
		PrintExpr(w, e.Cond, depth+2)
		if e.Then != nil {
			fmt.Fprintf(w, "%s  then:\n", indent)
			PrintExpr(w, e.Then, depth+2)
		}
		if e.Else != nil {
			fmt.Fprintf(w, "%s  else:\n", indent)
			PrintExpr(w, e.Else, depth+2)
		}

	case *GlslEscape:
		fmt.Fprintf(w, "%sGlslEscape(%s) { %s }\n", indent, e.Param, e.Code)
	}
}
