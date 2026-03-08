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

// NodeData carries source span and editing metadata for tree nodes.
type NodeData struct {
	Span     token.Span // span of the whole construct
	EditSpan token.Span // span of the editable value (zero if not editable)
}

// ScaffoldInfo describes a template to insert when a scaffold node is activated.
type ScaffoldInfo struct {
	Kind     string // setting kind (e.g. "light", "camera")
	Template string // Chisel code to insert
	InsertAt int    // byte offset where to insert
}

// scaffoldKinds lists the settings that can be scaffolded, in display order.
var scaffoldKinds = []struct {
	kind     string
	template string
}{
	{"bg", "bg #1a1a2e\n"},
	{"light", "light [-1, -1, -1]\n"},
	{"camera", "camera { pos: [0, 2, 5], target: [0, 0, 0] }\n"},
	{"raymarch", "raymarch { steps: 128 }\n"},
	{"post", "post { gamma: 2.2 }\n"},
}

// BuildSceneTree parses Chisel source and returns TreeNode roots for the scene
// tree panel. Returns nil if parsing fails.
func BuildSceneTree(source string) []TreeNode {
	tokens, _ := lexer.Lex("input.chisel", source)
	prog, _ := parser.Parse(tokens)
	if prog == nil {
		return nil
	}
	return BuildSceneTreeFromAST(prog, source)
}

// BuildSceneTreeFromAST builds TreeNode roots from a parsed AST program.
// The tree has up to four sections: Variables, Functions, Settings, Geometry.
// The source parameter is used for editable values and scaffold insertion points.
func BuildSceneTreeFromAST(prog *ast.Program, source string) []TreeNode {
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
		roots = append(roots, buildVariablesNode(variables, source))
	}
	if len(functions) > 0 {
		roots = append(roots, buildFunctionsNode(functions))
	}

	// Build settings with scaffold nodes for missing kinds
	scaffolds := buildScaffoldNodes(settings, geometry, source)
	if len(settings) > 0 || len(scaffolds) > 0 {
		roots = append(roots, buildSettingsNodeWithScaffolds(settings, scaffolds, source))
	}

	if len(geometry) > 0 {
		roots = append(roots, buildGeometryNode(geometry))
	}

	return roots
}

// ---------------------------------------------------------------------------
// Variables section
// ---------------------------------------------------------------------------

func buildVariablesNode(vars []*ast.AssignStmt, source string) TreeNode {
	return TreeNode{
		Label:  "Variables",
		Detail: fmt.Sprintf("(%d)", len(vars)),
		Data:   nil,
		Children: func() []TreeNode {
			nodes := make([]TreeNode, len(vars))
			for i, v := range vars {
				summary := summarizeExpr(v.Value)
				node := TreeNode{
					Label:  v.Name,
					Detail: "= " + summary,
					Data:   NodeData{Span: v.Span},
					Color:  colorFromExpr(v.Value),
					Children: func() []TreeNode {
						return exprChildren(v.Value)
					},
				}
				// Simple values (no sub-expressions) are editable
				if exprChildren(v.Value) == nil && isEditableExpr(v.Value) {
					node.Children = nil
					node.Editable = true
					node.Data = NodeData{Span: v.Span, EditSpan: v.Value.NodeSpan()}
					node.EditValue = spanText(source, v.Value.NodeSpan())
				}
				nodes[i] = node
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
					Data:  NodeData{Span: f.Span},
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

func buildSettingsNodeWithScaffolds(settings []*ast.SettingStmt, scaffolds []TreeNode, source string) TreeNode {
	return TreeNode{
		Label:  "Settings",
		Detail: fmt.Sprintf("(%d)", len(settings)),
		Data:   nil,
		Children: func() []TreeNode {
			nodes := make([]TreeNode, 0, len(settings)+len(scaffolds))
			for _, s := range settings {
				nodes = append(nodes, buildSettingNode(s, source))
			}
			nodes = append(nodes, scaffolds...)
			return nodes
		},
	}
}

func buildSettingNode(s *ast.SettingStmt, source string) TreeNode {
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
				Data:   NodeData{Span: s.Span},
			}
		}
	}

	// Single expression body (e.g. "bg #1a1a2e", "light [-1,-1,-1]")
	// Show inline as a leaf if the expression is simple
	if expr, ok := s.Body.(ast.Expr); ok {
		summary := summarizeExpr(expr)
		if exprChildren(expr) == nil {
			node := TreeNode{
				Label:  label,
				Detail: summary,
				Data:   NodeData{Span: s.Span},
				Color:  colorFromExpr(expr),
			}
			if isEditableExpr(expr) {
				node.Editable = true
				node.Data = NodeData{Span: s.Span, EditSpan: expr.NodeSpan()}
				node.EditValue = spanText(source, expr.NodeSpan())
			}
			return node
		}
	}

	return TreeNode{
		Label: label,
		Data:  NodeData{Span: s.Span},
		Children: func() []TreeNode {
			return settingBodyChildren(s.Kind, s.Body, source)
		},
	}
}

func settingBodyChildren(kind string, body interface{}, source string) []TreeNode {
	switch v := body.(type) {
	case map[string]interface{}:
		// mat stores {"name": string, "body": map} — skip name, show body contents
		if kind == "mat" {
			if bodyMap, ok := v["body"].(map[string]interface{}); ok {
				return mapChildren(bodyMap, source)
			}
		}
		return mapChildren(v, source)
	case ast.Expr:
		return []TreeNode{{
			Label: summarizeExpr(v),
			Data:  NodeData{Span: v.NodeSpan()},
			Children: func() []TreeNode {
				return exprChildren(v)
			},
		}}
	}
	return nil
}

func mapChildren(m map[string]interface{}, source string) []TreeNode {
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
					return mapChildren(val, source)
				},
			})
		case *ast.Block:
			// Block in settings context (e.g. bloom: { intensity: 0.3 })
			nodes = append(nodes, TreeNode{
				Label: k,
				Data:  NodeData{Span: val.NodeSpan()},
				Children: func() []TreeNode {
					return blockChildren(val)
				},
			})
		case ast.Expr:
			node := TreeNode{
				Label:  k,
				Detail: summarizeExpr(val),
				Data:   NodeData{Span: val.NodeSpan()},
				Color:  colorFromExpr(val),
			}
			if isEditableExpr(val) {
				node.Editable = true
				node.Data = NodeData{Span: val.NodeSpan(), EditSpan: val.NodeSpan()}
				node.EditValue = spanText(source, val.NodeSpan())
			}
			nodes = append(nodes, node)
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
		return TreeNode{Label: e.Name, Data: NodeData{Span: e.Span}}
	case *ast.UnaryExpr:
		return TreeNode{
			Label: "(-)",
			Data:  NodeData{Span: e.Span},
			Children: func() []TreeNode {
				return []TreeNode{exprToNode(e.Operand)}
			},
		}
	case *ast.GlslEscape:
		return TreeNode{Label: "glsl { ... }", Data: NodeData{Span: e.Span}}
	default:
		return TreeNode{Label: summarizeExpr(expr), Data: NodeData{Span: expr.NodeSpan()}}
	}
}

func binaryNode(e *ast.BinaryExpr) TreeNode {
	label := binaryOpLabel(e.Op)
	if e.Blend != nil {
		label += fmt.Sprintf(" (r=%.2g)", *e.Blend)
	}

	return TreeNode{
		Label: label,
		Data:  NodeData{Span: e.Span},
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
	rootNode.Data = NodeData{Span: e.Span}

	return rootNode
}

func methodNodes(methods []*ast.MethodCall) []TreeNode {
	nodes := make([]TreeNode, len(methods))
	for i, mc := range methods {
		label := "." + mc.Name + "(" + summarizeArgs(mc.Args) + ")"
		nodes[i] = TreeNode{
			Label: label,
			Data:  NodeData{Span: mc.Span},
		}
	}
	return nodes
}

func funcCallNode(e *ast.FuncCall) TreeNode {
	label := e.Name + "(" + summarizeArgs(e.Args) + ")"
	return TreeNode{
		Label: label,
		Data:  NodeData{Span: e.Span},
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
		Data:  NodeData{Span: e.Span},
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
		Data:  NodeData{Span: e.Span},
		Children: func() []TreeNode {
			var children []TreeNode
			if e.Then != nil {
				children = append(children, TreeNode{
					Label: "then",
					Data:  NodeData{Span: e.Then.Span},
					Children: func() []TreeNode {
						return blockChildren(e.Then)
					},
				})
			}
			if e.Else != nil {
				children = append(children, TreeNode{
					Label: "else",
					Data:  NodeData{Span: e.Else.NodeSpan()},
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
		Data:  NodeData{Span: e.Span},
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
				Data:   NodeData{Span: stmt.Span},
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
// Returns the zero Span if Data is not a Span or NodeData.
func SpanFromData(n TreeNode) token.Span {
	switch d := n.Data.(type) {
	case token.Span:
		return d
	case NodeData:
		return d.Span
	}
	return token.Span{}
}

// ---------------------------------------------------------------------------
// Scaffold node generation
// ---------------------------------------------------------------------------

func buildScaffoldNodes(settings []*ast.SettingStmt, geometry []ast.Expr, source string) []TreeNode {
	if source == "" {
		return nil
	}

	// Collect present setting kinds
	present := make(map[string]bool)
	for _, s := range settings {
		present[s.Kind] = true
	}

	// Compute insertion point: after last setting, or before first geometry
	insertAt := 0
	if len(settings) > 0 {
		last := settings[len(settings)-1]
		insertAt = last.Span.End.Offset
		// Skip to end of line
		for insertAt < len(source) && source[insertAt] != '\n' {
			insertAt++
		}
		if insertAt < len(source) {
			insertAt++ // past the newline
		}
	} else if len(geometry) > 0 {
		// Insert before first geometry expression
		insertAt = geometry[0].NodeSpan().Start.Offset
		// Make sure we're at a line start
		for insertAt > 0 && source[insertAt-1] != '\n' {
			insertAt--
		}
	} else {
		insertAt = len(source)
	}

	var scaffolds []TreeNode
	for _, sk := range scaffoldKinds {
		if present[sk.kind] {
			continue
		}
		scaffolds = append(scaffolds, TreeNode{
			Label:    sk.kind,
			Scaffold: true,
			Data: ScaffoldInfo{
				Kind:     sk.kind,
				Template: sk.template,
				InsertAt: insertAt,
			},
		})
	}
	return scaffolds
}

// ---------------------------------------------------------------------------
// Editing helpers
// ---------------------------------------------------------------------------

// colorFromExpr extracts RGB values from a HexColorLit expression.
// Returns nil if the expression is not a hex color.
func colorFromExpr(expr ast.Expr) *[3]uint8 {
	if c, ok := expr.(*ast.HexColorLit); ok {
		return &[3]uint8{
			uint8(c.R * 255),
			uint8(c.G * 255),
			uint8(c.B * 255),
		}
	}
	return nil
}

// isEditableExpr returns true if the expression is a simple value that can be
// edited inline (number, color, boolean, vector of simple values).
func isEditableExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.NumberLit, *ast.HexColorLit, *ast.BoolLit:
		return true
	case *ast.VecLit:
		for _, elem := range e.Elems {
			if !isEditableExpr(elem) {
				return false
			}
		}
		return true
	case *ast.UnaryExpr:
		return isEditableExpr(e.Operand)
	}
	return false
}

// spanText extracts source text for the given span. Returns "" if source is
// empty or span offsets are out of range.
func spanText(source string, span token.Span) string {
	if source == "" {
		return ""
	}
	start := span.Start.Offset
	end := span.End.Offset
	if start < 0 || end > len(source) || start > end {
		return ""
	}
	return source[start:end]
}
