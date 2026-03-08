package components

import (
	"fmt"
	"sort"
	"strings"

	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
	"asciishader/pkg/chisel/compiler/token"
)

// BuildSceneTree parses Chisel source and returns TreeNode roots for the scene
// tree panel. Returns nil if parsing fails.
func BuildSceneTree(source string) []TreeNode {
	tokens, _ := lexer.Lex("input.chisel", source)
	prog, _ := parser.Parse(tokens)
	if prog == nil {
		return nil
	}
	return BuildSceneTreeFromAST(prog)
}

// BuildSceneTreeFromAST builds TreeNode roots from a parsed AST program.
// The tree has up to four sections: Variables, Functions, Settings, Geometry.
func BuildSceneTreeFromAST(prog *ast.Program) []TreeNode {
	var variables []*ast.AssignStmt
	var functions []*ast.AssignStmt
	var settings []*ast.SettingStmt
	var geometry []ast.Expr

	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if s.Params != nil {
				functions = append(functions, s)
			} else {
				variables = append(variables, s)
			}
		case *ast.SettingStmt:
			settings = append(settings, s)
		case *ast.ExprStmt:
			geometry = append(geometry, s.Expression)
		}
	}

	var roots []TreeNode

	if len(variables) > 0 {
		roots = append(roots, buildVariablesNode(variables))
	}
	if len(functions) > 0 {
		roots = append(roots, buildFunctionsNode(functions))
	}
	if len(settings) > 0 {
		roots = append(roots, buildSettingsNode(settings))
	}
	if len(geometry) > 0 {
		roots = append(roots, buildGeometryNode(geometry))
	}

	return roots
}

// ---------------------------------------------------------------------------
// Variables section
// ---------------------------------------------------------------------------

func buildVariablesNode(vars []*ast.AssignStmt) TreeNode {
	return TreeNode{
		Label:  "Variables",
		Detail: fmt.Sprintf("(%d)", len(vars)),
		Data:   nil,
		Children: func() []TreeNode {
			nodes := make([]TreeNode, len(vars))
			for i, v := range vars {
				summary := summarizeExpr(v.Value)
				nodes[i] = TreeNode{
					Label:  v.Name,
					Detail: "= " + summary,
					Data:   v.Span,
					Children: func() []TreeNode {
						return exprChildren(v.Value)
					},
				}
			}
			return nodes
		},
	}
}

// ---------------------------------------------------------------------------
// Functions section
// ---------------------------------------------------------------------------

func buildFunctionsNode(fns []*ast.AssignStmt) TreeNode {
	return TreeNode{
		Label:  "Functions",
		Detail: fmt.Sprintf("(%d)", len(fns)),
		Data:   nil,
		Children: func() []TreeNode {
			nodes := make([]TreeNode, len(fns))
			for i, f := range fns {
				sig := buildSignature(f)
				// Capture for closure
				fn := f
				nodes[i] = TreeNode{
					Label: sig,
					Data:  f.Span,
					Children: func() []TreeNode {
						return exprChildren(fn.Value)
					},
				}
			}
			return nodes
		},
	}
}

func buildSignature(f *ast.AssignStmt) string {
	var b strings.Builder
	b.WriteString(f.Name)
	b.WriteByte('(')
	for i, p := range f.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(p.Name)
		if p.Default != nil {
			b.WriteString("=")
			b.WriteString(summarizeExpr(p.Default))
		}
	}
	b.WriteByte(')')
	return b.String()
}

// ---------------------------------------------------------------------------
// Settings section
// ---------------------------------------------------------------------------

func buildSettingsNode(settings []*ast.SettingStmt) TreeNode {
	return TreeNode{
		Label:  "Settings",
		Detail: fmt.Sprintf("(%d)", len(settings)),
		Data:   nil,
		Children: func() []TreeNode {
			nodes := make([]TreeNode, len(settings))
			for i, s := range settings {
				nodes[i] = buildSettingNode(s)
			}
			return nodes
		},
	}
}

func buildSettingNode(s *ast.SettingStmt) TreeNode {
	label := s.Kind

	// mat has a name: "mat gold"
	if s.Kind == "mat" {
		if m, ok := s.Body.(map[string]interface{}); ok {
			if name, ok := m["name"].(string); ok {
				label = "mat " + name
			}
		}
	}

	// debug has a mode string
	if s.Kind == "debug" {
		if mode, ok := s.Body.(string); ok {
			return TreeNode{
				Label:  label,
				Detail: mode,
				Data:   s.Span,
			}
		}
	}

	// Single expression body (e.g. "bg #1a1a2e", "light [-1,-1,-1]")
	// Show inline as a leaf if the expression is simple
	if expr, ok := s.Body.(ast.Expr); ok {
		summary := summarizeExpr(expr)
		if exprChildren(expr) == nil {
			// Simple leaf: show value as detail
			return TreeNode{
				Label:  label,
				Detail: summary,
				Data:   s.Span,
			}
		}
	}

	return TreeNode{
		Label: label,
		Data:  s.Span,
		Children: func() []TreeNode {
			return settingBodyChildren(s.Kind, s.Body)
		},
	}
}

func settingBodyChildren(kind string, body interface{}) []TreeNode {
	switch v := body.(type) {
	case map[string]interface{}:
		// mat stores {"name": string, "body": map} — skip name, show body contents
		if kind == "mat" {
			if bodyMap, ok := v["body"].(map[string]interface{}); ok {
				return mapChildren(bodyMap)
			}
		}
		return mapChildren(v)
	case ast.Expr:
		return []TreeNode{{
			Label: summarizeExpr(v),
			Data:  v.NodeSpan(),
			Children: func() []TreeNode {
				return exprChildren(v)
			},
		}}
	}
	return nil
}

func mapChildren(m map[string]interface{}) []TreeNode {
	// Sort keys for stable output
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var nodes []TreeNode
	for _, k := range keys {
		v := m[k]
		switch val := v.(type) {
		case map[string]interface{}:
			count := len(val)
			nodes = append(nodes, TreeNode{
				Label:  k,
				Detail: fmt.Sprintf("(%d)", count),
				Data:   nil,
				Children: func() []TreeNode {
					return mapChildren(val)
				},
			})
		case *ast.Block:
			// Block in settings context (e.g. bloom: { intensity: 0.3 })
			nodes = append(nodes, TreeNode{
				Label: k,
				Data:  val.NodeSpan(),
				Children: func() []TreeNode {
					return blockChildren(val)
				},
			})
		case ast.Expr:
			nodes = append(nodes, TreeNode{
				Label:  k,
				Detail: summarizeExpr(val),
				Data:   val.NodeSpan(),
			})
		case string:
			nodes = append(nodes, TreeNode{
				Label:  k,
				Detail: val,
			})
		default:
			nodes = append(nodes, TreeNode{
				Label:  k,
				Detail: fmt.Sprintf("%v", val),
			})
		}
	}
	return nodes
}

// ---------------------------------------------------------------------------
// Geometry section
// ---------------------------------------------------------------------------

func buildGeometryNode(exprs []ast.Expr) TreeNode {
	return TreeNode{
		Label: "Geometry",
		Children: func() []TreeNode {
			nodes := make([]TreeNode, len(exprs))
			for i, e := range exprs {
				nodes[i] = exprToNode(e)
			}
			return nodes
		},
	}
}

// ---------------------------------------------------------------------------
// Expression → TreeNode conversion
// ---------------------------------------------------------------------------

// exprToNode converts an AST expression into a TreeNode for the tree view.
func exprToNode(expr ast.Expr) TreeNode {
	if expr == nil {
		return TreeNode{Label: "(nil)"}
	}

	switch e := expr.(type) {
	case *ast.BinaryExpr:
		return binaryNode(e)
	case *ast.MethodCall:
		return methodCallNode(e)
	case *ast.FuncCall:
		return funcCallNode(e)
	case *ast.ForExpr:
		return forNode(e)
	case *ast.IfExpr:
		return ifNode(e)
	case *ast.Block:
		return blockNode(e)
	case *ast.Ident:
		return TreeNode{Label: e.Name, Data: e.Span}
	case *ast.UnaryExpr:
		return TreeNode{
			Label: "(-)",
			Data:  e.Span,
			Children: func() []TreeNode {
				return []TreeNode{exprToNode(e.Operand)}
			},
		}
	case *ast.GlslEscape:
		return TreeNode{Label: "glsl { ... }", Data: e.Span}
	default:
		return TreeNode{Label: summarizeExpr(expr), Data: expr.NodeSpan()}
	}
}

func binaryNode(e *ast.BinaryExpr) TreeNode {
	label := binaryOpLabel(e.Op)
	if e.Blend != nil {
		label += fmt.Sprintf(" (r=%.2g)", *e.Blend)
	}

	return TreeNode{
		Label: label,
		Data:  e.Span,
		Children: func() []TreeNode {
			return []TreeNode{exprToNode(e.Left), exprToNode(e.Right)}
		},
	}
}

func binaryOpLabel(op ast.BinaryOp) string {
	switch op {
	case ast.Union:
		return "union"
	case ast.SmoothUnion:
		return "smooth union"
	case ast.ChamferUnion:
		return "chamfer union"
	case ast.Subtract:
		return "subtract"
	case ast.SmoothSubtract:
		return "smooth subtract"
	case ast.ChamferSubtract:
		return "chamfer subtract"
	case ast.Intersect:
		return "intersect"
	case ast.SmoothIntersect:
		return "smooth intersect"
	case ast.ChamferIntersect:
		return "chamfer intersect"
	default:
		return op.String()
	}
}

// methodCallNode flattens a method chain: sphere(1).at(2,0,0).color(#f00)
// becomes a node for the root receiver with method calls as children.
func methodCallNode(e *ast.MethodCall) TreeNode {
	// Collect the chain of method calls
	var methods []*ast.MethodCall
	var root ast.Expr = e
	for {
		mc, ok := root.(*ast.MethodCall)
		if !ok {
			break
		}
		methods = append(methods, mc)
		root = mc.Receiver
	}

	// methods is in reverse order (outermost first), reverse it
	for i, j := 0, len(methods)-1; i < j; i, j = i+1, j-1 {
		methods[i], methods[j] = methods[j], methods[i]
	}

	// Build the root node
	rootNode := exprToNode(root)

	// If root is a leaf (no children func), attach methods as children
	if rootNode.Children == nil {
		rootNode.Children = func() []TreeNode {
			return methodNodes(methods)
		}
	} else {
		// Root has its own children (e.g., a BinaryExpr) — append methods
		origChildren := rootNode.Children
		rootNode.Children = func() []TreeNode {
			children := origChildren()
			children = append(children, methodNodes(methods)...)
			return children
		}
	}

	// Use the outermost span so clicking jumps to the full expression
	rootNode.Data = e.Span

	return rootNode
}

func methodNodes(methods []*ast.MethodCall) []TreeNode {
	nodes := make([]TreeNode, len(methods))
	for i, mc := range methods {
		label := "." + mc.Name + "(" + summarizeArgs(mc.Args) + ")"
		nodes[i] = TreeNode{
			Label: label,
			Data:  mc.Span,
		}
	}
	return nodes
}

func funcCallNode(e *ast.FuncCall) TreeNode {
	label := e.Name + "(" + summarizeArgs(e.Args) + ")"
	return TreeNode{
		Label: label,
		Data:  e.Span,
	}
}

func forNode(e *ast.ForExpr) TreeNode {
	// Build "for i in 0..8" label from iterators
	var parts []string
	for _, it := range e.Iterators {
		s := summarizeExpr(it.Start)
		end := summarizeExpr(it.End)
		part := fmt.Sprintf("%s in %s..%s", it.Name, s, end)
		if it.Step != nil {
			part += " step " + summarizeExpr(it.Step)
		}
		parts = append(parts, part)
	}
	label := "for " + strings.Join(parts, ", ")

	return TreeNode{
		Label: label,
		Data:  e.Span,
		Children: func() []TreeNode {
			if e.Body == nil {
				return nil
			}
			return blockChildren(e.Body)
		},
	}
}

func ifNode(e *ast.IfExpr) TreeNode {
	label := "if " + summarizeExpr(e.Cond)
	return TreeNode{
		Label: label,
		Data:  e.Span,
		Children: func() []TreeNode {
			var children []TreeNode
			if e.Then != nil {
				children = append(children, TreeNode{
					Label: "then",
					Data:  e.Then.Span,
					Children: func() []TreeNode {
						return blockChildren(e.Then)
					},
				})
			}
			if e.Else != nil {
				children = append(children, TreeNode{
					Label: "else",
					Data:  e.Else.NodeSpan(),
					Children: func() []TreeNode {
						return exprChildren(e.Else)
					},
				})
			}
			return children
		},
	}
}

func blockNode(e *ast.Block) TreeNode {
	return TreeNode{
		Label: "{ ... }",
		Data:  e.Span,
		Children: func() []TreeNode {
			return blockChildren(e)
		},
	}
}

func blockChildren(b *ast.Block) []TreeNode {
	var nodes []TreeNode
	for _, s := range b.Stmts {
		switch stmt := s.(type) {
		case *ast.AssignStmt:
			summary := summarizeExpr(stmt.Value)
			nodes = append(nodes, TreeNode{
				Label:  stmt.Name,
				Detail: "= " + summary,
				Data:   stmt.Span,
			})
		case *ast.ExprStmt:
			nodes = append(nodes, exprToNode(stmt.Expression))
		}
	}
	if b.Result != nil {
		nodes = append(nodes, exprToNode(b.Result))
	}
	return nodes
}

// exprChildren returns child TreeNodes for an expression's sub-expressions.
// Used when a variable or setting value needs to be expandable.
func exprChildren(expr ast.Expr) []TreeNode {
	if expr == nil {
		return nil
	}
	node := exprToNode(expr)
	if node.Children != nil {
		return node.Children()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Expression summarization
// ---------------------------------------------------------------------------

// summarizeExpr returns a compact string representation of an expression,
// suitable for display in tree node labels and details.
func summarizeExpr(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.NumberLit:
		return formatNumber(e.Value)
	case *ast.BoolLit:
		if e.Value {
			return "true"
		}
		return "false"
	case *ast.StringLit:
		return fmt.Sprintf("%q", e.Value)
	case *ast.HexColorLit:
		return fmt.Sprintf("#%02x%02x%02x", int(e.R*255), int(e.G*255), int(e.B*255))
	case *ast.Ident:
		return e.Name
	case *ast.VecLit:
		parts := make([]string, len(e.Elems))
		for i, elem := range e.Elems {
			parts[i] = summarizeExpr(elem)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *ast.FuncCall:
		return e.Name + "(" + summarizeArgs(e.Args) + ")"
	case *ast.MethodCall:
		return summarizeExpr(e.Receiver) + "." + e.Name + "(...)"
	case *ast.BinaryExpr:
		left := summarizeExpr(e.Left)
		right := summarizeExpr(e.Right)
		op := binaryOpSymbol(e.Op)
		s := left + " " + op + " " + right
		if len(s) > 40 {
			return left + " " + op + " ..."
		}
		return s
	case *ast.UnaryExpr:
		return "-" + summarizeExpr(e.Operand)
	case *ast.ForExpr:
		return "for ..."
	case *ast.IfExpr:
		return "if ..."
	case *ast.Block:
		return "{ ... }"
	case *ast.GlslEscape:
		return "glsl { ... }"
	case *ast.Swizzle:
		return summarizeExpr(e.Receiver) + "." + e.Components
	default:
		return "..."
	}
}

func summarizeArgs(args []ast.Arg) string {
	parts := make([]string, len(args))
	for i, a := range args {
		s := summarizeExpr(a.Value)
		if a.Name != "" {
			s = a.Name + ": " + s
		}
		parts[i] = s
	}
	result := strings.Join(parts, ", ")
	if len(result) > 50 {
		// Truncate long argument lists
		for i := range parts {
			if len(strings.Join(parts[:i+1], ", ")) > 45 {
				return strings.Join(parts[:i], ", ") + ", ..."
				break //nolint:all
			}
		}
	}
	return result
}

func binaryOpSymbol(op ast.BinaryOp) string {
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

func formatNumber(v float64) string {
	if v == float64(int(v)) && v >= -9999 && v <= 9999 {
		return fmt.Sprintf("%d", int(v))
	}
	s := fmt.Sprintf("%g", v)
	return s
}

// SpanFromData extracts a token.Span from a TreeNode's Data field.
// Returns the zero Span if Data is not a Span.
func SpanFromData(n TreeNode) token.Span {
	if s, ok := n.Data.(token.Span); ok {
		return s
	}
	return token.Span{}
}
