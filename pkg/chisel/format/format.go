// Package format implements a canonical formatter for Chisel source code.
// It re-emits source with consistent style while preserving comments and
// user intent for line breaks. Settings blocks, method chains, and argument
// lists are broken across lines when they exceed the target width.
package format

import (
	"fmt"
	"math"
	"strings"

	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/diagnostic"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
	"asciishader/pkg/chisel/compiler/token"
)

const maxWidth = 100

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

	// Extract comments from token stream.
	var comments []commentInfo
	for _, tok := range tokens {
		if tok.Kind == token.TokComment {
			comments = append(comments, commentInfo{
				line: tok.Pos.Line,
				text: tok.Value,
			})
		}
	}

	// Compute blank line positions from the original source.
	blankLines := findBlankLines(source)

	f := &formatter{
		comments:   comments,
		blankLines: blankLines,
		lastLine:   0,
	}
	f.formatProgram(prog)

	result := f.buf.String()
	if result != "" && !strings.HasSuffix(result, "\n") {
		result += "\n"
	}
	return result, nil
}

type commentInfo struct {
	line int    // 1-based
	text string // raw comment text including // or /* */
}

type formatter struct {
	buf        strings.Builder
	indent     int
	comments   []commentInfo
	commentIdx int
	blankLines map[int]bool // 1-based line numbers that are blank
	lastLine   int          // last source line we've emitted content for
}

// findBlankLines returns a set of 1-based line numbers that are blank.
func findBlankLines(source string) map[int]bool {
	result := make(map[int]bool)
	for i, line := range strings.Split(source, "\n") {
		if strings.TrimSpace(line) == "" {
			result[i+1] = true // 1-based
		}
	}
	return result
}

func (f *formatter) write(s string) {
	f.buf.WriteString(s)
}

func (f *formatter) writeln() {
	f.buf.WriteByte('\n')
}

func (f *formatter) writeIndent() {
	for i := 0; i < f.indent; i++ {
		f.buf.WriteString("  ")
	}
}

// emitCommentsBefore emits all comments on lines before the given line.
func (f *formatter) emitCommentsBefore(line int) {
	for f.commentIdx < len(f.comments) {
		c := f.comments[f.commentIdx]
		if c.line >= line {
			break
		}
		// Blank line before comment block if there was one in source.
		if f.lastLine > 0 && c.line > f.lastLine+1 {
			f.writeln()
		}
		f.writeIndent()
		f.write(c.text)
		f.writeln()
		f.lastLine = c.line
		f.commentIdx++
	}
}

// emitTrailingComment emits a comment on the same line if present.
func (f *formatter) emitTrailingComment(line int) {
	if f.commentIdx < len(f.comments) {
		c := f.comments[f.commentIdx]
		if c.line == line {
			f.write(" ")
			f.write(c.text)
			f.commentIdx++
		}
	}
}

// emitBlankLineBefore emits a blank line if the original source had one.
func (f *formatter) emitBlankLineBefore(line int) {
	if f.lastLine > 0 && line > f.lastLine+1 {
		// Check if there was actually a blank line in the source.
		for l := f.lastLine + 1; l < line; l++ {
			if f.blankLines[l] {
				f.writeln()
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Program
// ---------------------------------------------------------------------------

func (f *formatter) formatProgram(prog *ast.Program) {
	for _, stmt := range prog.Statements {
		stmtLine := stmt.NodeSpan().Start.Line
		f.emitCommentsBefore(stmtLine)
		f.emitBlankLineBefore(stmtLine)
		f.writeIndent()
		f.formatStmt(stmt)
		f.emitTrailingComment(stmt.NodeSpan().End.Line)
		f.writeln()
		f.lastLine = stmt.NodeSpan().End.Line
	}
	// Emit any trailing comments after the last statement.
	f.emitCommentsBefore(math.MaxInt32)
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

func (f *formatter) formatStmt(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		f.formatAssign(s)
	case *ast.ExprStmt:
		if isMultiLineExpr(s.Expression) {
			f.formatExpr(s.Expression, true)
		} else {
			singleLine := exprToString(s.Expression)
			if f.currentLineLen()+len(singleLine) > maxWidth {
				f.formatExpr(s.Expression, true)
			} else {
				f.write(singleLine)
			}
		}
	case *ast.SettingStmt:
		f.formatSetting(s)
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
				f.formatExpr(p.Default, false)
			}
		}
		f.write(")")
	}
	f.write(" = ")

	// Multi-line expressions (blocks, for, if, glsl) always use full formatting.
	if isMultiLineExpr(s.Value) {
		f.formatExpr(s.Value, true)
		return
	}
	// Check if the value fits on one line with the prefix.
	prefix := f.currentLineLen()
	valStr := exprToString(s.Value)
	if prefix+len(valStr) <= maxWidth {
		f.write(valStr)
	} else {
		f.formatExpr(s.Value, true)
	}
}

func (f *formatter) formatSetting(s *ast.SettingStmt) {
	f.write(s.Kind)
	if s.Body == nil {
		return
	}
	f.write(" ")
	switch body := s.Body.(type) {
	case ast.Expr:
		f.formatExpr(body, false)
	case string:
		f.write(body)
	case map[string]interface{}:
		f.formatSettingsMap(s.Kind, body)
	}
}

func (f *formatter) formatSettingsMap(kind string, m map[string]interface{}) {
	// Special case: mat has name + body.
	if kind == "mat" {
		if name, ok := m["name"].(string); ok {
			f.write(name)
			f.write(" = ")
			if body, ok := m["body"].(map[string]interface{}); ok {
				f.formatSettingsBlock(body)
			}
		}
		return
	}

	// Special case: camera with pos -> target.
	if kind == "camera" {
		if pos, ok := m["pos"]; ok {
			if posExpr, ok := pos.(ast.Expr); ok {
				f.formatExpr(posExpr, false)
				if target, ok := m["target"]; ok {
					if targetExpr, ok := target.(ast.Expr); ok {
						f.write(" -> ")
						f.formatExpr(targetExpr, false)
					}
				}
				return
			}
		}
	}

	f.formatSettingsBlock(m)
}

func (f *formatter) formatSettingsBlock(m map[string]interface{}) {
	// Collect key-value pairs.
	type entry struct {
		key string
		val interface{}
	}
	var entries []entry
	for k, v := range m {
		entries = append(entries, entry{k, v})
	}

	// Try single line: { key: val, key: val }
	var parts []string
	for _, e := range entries {
		switch v := e.val.(type) {
		case ast.Expr:
			parts = append(parts, e.key+": "+exprToString(v))
		case map[string]interface{}:
			// Nested block — force multi-line.
			parts = nil
		default:
			parts = append(parts, fmt.Sprintf("%s: %v", e.key, v))
		}
		if parts == nil {
			break
		}
	}

	if parts != nil {
		oneLine := "{ " + strings.Join(parts, ", ") + " }"
		if f.currentLineLen()+len(oneLine) <= maxWidth {
			f.write(oneLine)
			return
		}
	}

	// Multi-line.
	f.write("{\n")
	f.indent++
	for _, e := range entries {
		f.writeIndent()
		f.write(e.key)
		switch v := e.val.(type) {
		case ast.Expr:
			f.write(": ")
			f.formatExpr(v, false)
		case map[string]interface{}:
			f.write(" ")
			f.formatSettingsBlock(v)
		default:
			f.write(fmt.Sprintf(": %v", v))
		}
		f.writeln()
	}
	f.indent--
	f.writeIndent()
	f.write("}")
}

// ---------------------------------------------------------------------------
// Expressions
// ---------------------------------------------------------------------------

// formatExpr formats an expression. If multiLine is true, the formatter
// may break method chains and long argument lists across multiple lines.
func (f *formatter) formatExpr(expr ast.Expr, multiLine bool) {
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
		f.write("\"" + e.Value + "\"")
	case *ast.HexColorLit:
		f.formatHexColor(e)
	case *ast.Ident:
		f.write(e.Name)
	case *ast.VecLit:
		f.formatVecLit(e)
	case *ast.BinaryExpr:
		f.formatBinaryExpr(e, multiLine)
	case *ast.UnaryExpr:
		f.formatUnaryExpr(e)
	case *ast.FuncCall:
		f.formatFuncCall(e, multiLine)
	case *ast.MethodCall:
		f.formatMethodChain(e, multiLine)
	case *ast.Swizzle:
		f.formatExpr(e.Receiver, multiLine)
		f.write(".")
		f.write(e.Components)
	case *ast.Block:
		f.formatBlock(e)
	case *ast.ForExpr:
		f.formatFor(e)
	case *ast.IfExpr:
		f.formatIf(e)
	case *ast.GlslEscape:
		f.formatGlsl(e)
	}
}

func (f *formatter) formatNumber(v float64) {
	if v == math.Trunc(v) && !math.IsInf(v, 0) && !math.IsNaN(v) {
		f.write(fmt.Sprintf("%g", v))
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
		f.formatExpr(elem, false)
	}
	f.write("]")
}

func (f *formatter) formatBinaryExpr(e *ast.BinaryExpr, multiLine bool) {
	op := binaryOpString(e.Op)
	blendStr := ""
	if e.Blend != nil && isSmoothOrChamferOp(e.Op) {
		blendStr = formatNumberStr(*e.Blend)
	}
	opStr := op + blendStr

	if !multiLine || !isCSGOp(e.Op) {
		// Single line with precedence-aware parentheses.
		f.formatExprWithPrec(e.Left, e.Op, true)
		f.write(" " + opStr + " ")
		f.formatExprWithPrec(e.Right, e.Op, false)
		return
	}

	// Multi-line CSG: break at operators.
	parts := flattenCSGChain(e)
	for i, part := range parts {
		if i == 0 {
			f.formatExpr(part.expr, true)
		} else {
			f.writeln()
			f.writeIndent()
			f.write(part.op + " ")
			f.formatExpr(part.expr, true)
		}
	}
}

// formatExprWithPrec wraps a child expression in parens if its precedence
// is lower than the parent operator (i.e., it would bind wrong without parens).
func (f *formatter) formatExprWithPrec(child ast.Expr, parentOp ast.BinaryOp, isLeft bool) {
	if childBin, ok := child.(*ast.BinaryExpr); ok {
		childPrec := opPrecedence(childBin.Op)
		parentPrec := opPrecedence(parentOp)
		// Need parens if child binds looser than parent,
		// or if same precedence on the right side (right-associativity guard).
		if childPrec < parentPrec || (childPrec == parentPrec && !isLeft) {
			f.write("(")
			f.formatExpr(child, false)
			f.write(")")
			return
		}
	}
	f.formatExpr(child, false)
}

// opPrecedence returns the Pratt precedence for a binary operator.
func opPrecedence(op ast.BinaryOp) int {
	switch op {
	case ast.Union, ast.SmoothUnion, ast.ChamferUnion:
		return 1
	case ast.Subtract, ast.SmoothSubtract, ast.ChamferSubtract, ast.Sub:
		return 2
	case ast.Intersect, ast.SmoothIntersect, ast.ChamferIntersect:
		return 3
	case ast.Eq, ast.Neq, ast.Lt, ast.Gt, ast.Lte, ast.Gte:
		return 4
	case ast.Add:
		return 5
	case ast.Mul, ast.Div, ast.Mod:
		return 6
	}
	return 0
}

type csgPart struct {
	op   string // operator string (empty for the first part)
	expr ast.Expr
}

// flattenCSGChain flattens a left-associative chain of CSG binary exprs.
func flattenCSGChain(e *ast.BinaryExpr) []csgPart {
	var parts []csgPart
	var flatten func(expr ast.Expr)
	flatten = func(expr ast.Expr) {
		be, ok := expr.(*ast.BinaryExpr)
		if !ok || !isCSGOp(be.Op) {
			parts = append(parts, csgPart{expr: expr})
			return
		}
		flatten(be.Left)
		op := binaryOpString(be.Op)
		if be.Blend != nil && isSmoothOrChamferOp(be.Op) {
			op += formatNumberStr(*be.Blend)
		}
		// Mark the operator on the last part added.
		parts = append(parts, csgPart{op: op, expr: be.Right})
	}
	flatten(e)
	return parts
}

func (f *formatter) formatUnaryExpr(e *ast.UnaryExpr) {
	switch e.Op {
	case ast.Neg:
		f.write("-")
	case ast.Not:
		f.write("!")
	}
	f.formatExpr(e.Operand, false)
}

func (f *formatter) formatFuncCall(e *ast.FuncCall, multiLine bool) {
	f.write(e.Name)
	f.write("(")
	// Force multi-line if any arg is inherently multi-line.
	forceMulti := multiLine
	for _, arg := range e.Args {
		if isMultiLineExpr(arg.Value) {
			forceMulti = true
			break
		}
	}
	f.formatArgsWithWidth(e.Args, forceMulti)
	f.write(")")
}

// chainLink represents one segment of a method chain.
type chainLink struct {
	name string
	args []ast.Arg
}

// isMultiLineExpr returns true for expressions that are inherently multi-line
// and should never be collapsed to a single line.
func isMultiLineExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.Block, *ast.ForExpr, *ast.IfExpr:
		return true
	case *ast.GlslEscape:
		return strings.Contains(e.Code, "\n")
	case *ast.BinaryExpr:
		return isMultiLineExpr(e.Left) || isMultiLineExpr(e.Right)
	case *ast.MethodCall:
		if isMultiLineExpr(e.Receiver) {
			return true
		}
		for _, arg := range e.Args {
			if isMultiLineExpr(arg.Value) {
				return true
			}
		}
		return false
	case *ast.FuncCall:
		for _, arg := range e.Args {
			if isMultiLineExpr(arg.Value) {
				return true
			}
		}
		return false
	}
	return false
}

// needsParens returns true if the expression needs parentheses when used
// as the base of a method chain or in other tight-binding contexts.
func needsParens(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.BinaryExpr:
		return true
	case *ast.UnaryExpr:
		return true
	}
	return false
}

// collectMethodChain flattens a nested MethodCall/Swizzle tree into a base + links.
func collectMethodChain(e *ast.MethodCall) (ast.Expr, []chainLink) {
	var chain []chainLink
	var base ast.Expr
	cur := e
	for {
		chain = append([]chainLink{{cur.Name, cur.Args}}, chain...)
		switch r := cur.Receiver.(type) {
		case *ast.MethodCall:
			cur = r
			continue
		case *ast.Swizzle:
			chain = append([]chainLink{{r.Components, nil}}, chain...)
			base = r.Receiver
		default:
			base = cur.Receiver
		}
		break
	}
	return base, chain
}

// chainToString renders a method chain as a single-line string (no recursion into formatMethodChain).
func chainToString(base ast.Expr, chain []chainLink) string {
	var b strings.Builder
	if needsParens(base) {
		b.WriteByte('(')
		b.WriteString(exprToStringSingle(base))
		b.WriteByte(')')
	} else {
		b.WriteString(exprToStringSingle(base))
	}
	for _, link := range chain {
		b.WriteByte('.')
		b.WriteString(link.name)
		if len(link.args) > 0 {
			b.WriteByte('(')
			b.WriteString(argsToString(link.args))
			b.WriteByte(')')
		}
	}
	return b.String()
}

// formatMethodChain collects the full chain and decides whether to break lines.
func (f *formatter) formatMethodChain(e *ast.MethodCall, multiLine bool) {
	base, chain := collectMethodChain(e)

	// Check if base or any args contain multi-line expressions.
	hasMultiLine := isMultiLineExpr(base)
	if !hasMultiLine {
		for _, link := range chain {
			for _, arg := range link.args {
				if isMultiLineExpr(arg.Value) {
					hasMultiLine = true
					break
				}
			}
		}
	}

	// Try single line first (only if no multi-line sub-expressions).
	if !hasMultiLine {
		singleLine := chainToString(base, chain)
		if !multiLine || f.currentLineLen()+len(singleLine) <= maxWidth {
			f.write(singleLine)
			return
		}
	}

	// Multi-line: base on first line, each .method on subsequent lines.
	if needsParens(base) {
		f.write("(")
		f.formatExpr(base, false)
		f.write(")")
	} else {
		f.formatExpr(base, false)
	}
	f.indent++
	for _, link := range chain {
		f.writeln()
		f.writeIndent()
		f.write(".")
		f.write(link.name)
		if len(link.args) > 0 {
			f.write("(")
			f.formatArgsWithWidth(link.args, true)
			f.write(")")
		}
	}
	f.indent--
}

func (f *formatter) formatArgsWithWidth(args []ast.Arg, multiLine bool) {
	if len(args) == 0 {
		return
	}

	// Check for multi-line args that cannot be single-lined.
	hasMultiLine := false
	for _, arg := range args {
		if isMultiLineExpr(arg.Value) {
			hasMultiLine = true
			break
		}
	}

	// Try single line (only if no multi-line sub-expressions).
	if !hasMultiLine {
		singleLine := argsToString(args)
		if !multiLine || f.currentLineLen()+len(singleLine)+1 <= maxWidth {
			f.write(singleLine)
			return
		}
	}

	// Multi-line: one arg per line.
	f.writeln()
	f.indent++
	for i, arg := range args {
		f.writeIndent()
		if arg.Name != "" {
			f.write(arg.Name + ": ")
		}
		f.formatExpr(arg.Value, true)
		if i < len(args)-1 {
			f.write(",")
		}
		f.writeln()
	}
	f.indent--
	f.writeIndent()
}

func (f *formatter) formatBlock(e *ast.Block) {
	f.write("{\n")
	f.indent++

	savedLastLine := f.lastLine
	if len(e.Stmts) > 0 {
		f.lastLine = e.Stmts[0].NodeSpan().Start.Line - 1
	} else if e.Result != nil {
		f.lastLine = e.Result.NodeSpan().Start.Line - 1
	}

	for _, s := range e.Stmts {
		stmtLine := s.NodeSpan().Start.Line
		f.emitCommentsBefore(stmtLine)
		f.emitBlankLineBefore(stmtLine)
		f.writeIndent()
		f.formatStmt(s)
		f.emitTrailingComment(s.NodeSpan().End.Line)
		f.writeln()
		f.lastLine = s.NodeSpan().End.Line
	}
	if e.Result != nil {
		resLine := e.Result.NodeSpan().Start.Line
		f.emitCommentsBefore(resLine)
		f.emitBlankLineBefore(resLine)
		f.writeIndent()
		f.formatExpr(e.Result, true)
		f.emitTrailingComment(e.Result.NodeSpan().End.Line)
		f.writeln()
		f.lastLine = e.Result.NodeSpan().End.Line
	}

	// Emit comments before closing brace.
	f.emitCommentsBefore(e.NodeSpan().End.Line)
	f.lastLine = savedLastLine

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
		f.formatExpr(it.Start, false)
		f.write("..")
		f.formatExpr(it.End, false)
		if it.Step != nil {
			f.write(" step ")
			f.formatExpr(it.Step, false)
		}
	}
	f.write(" ")
	if e.Body != nil {
		f.formatBlock(e.Body)
	}
}

func (f *formatter) formatIf(e *ast.IfExpr) {
	f.write("if ")
	f.formatExpr(e.Cond, false)
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
			f.formatExpr(e.Else, false)
		}
	}
}

func (f *formatter) formatGlsl(e *ast.GlslEscape) {
	f.write("glsl(")
	f.write(e.Param)
	f.write(") {")
	// Preserve multi-line GLSL code.
	lines := strings.Split(e.Code, "\n")
	if len(lines) <= 1 {
		if e.Code != "" {
			f.write(" " + strings.TrimSpace(e.Code) + " ")
		}
		f.write("}")
		return
	}
	f.writeln()
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			f.writeln()
		} else {
			f.writeIndent()
			f.write("  ")
			f.write(strings.TrimRight(line, " \t"))
			f.writeln()
		}
	}
	f.writeIndent()
	f.write("}")
}

// currentLineLen returns the length of the current (unfinished) line.
func (f *formatter) currentLineLen() int {
	s := f.buf.String()
	nl := strings.LastIndexByte(s, '\n')
	if nl < 0 {
		return len(s)
	}
	return len(s) - nl - 1
}

// ---------------------------------------------------------------------------
// Expression-to-string (for width estimation)
// ---------------------------------------------------------------------------

// exprToString formats an expression to a single-line string for width checks.
func exprToString(expr ast.Expr) string {
	return exprToStringSingle(expr)
}

// exprToStringSingle renders an expression to a single-line string without
// triggering multi-line formatting (avoids infinite recursion in width checks).
func exprToStringSingle(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	var b strings.Builder
	writeExprSingle(&b, expr)
	return b.String()
}

func writeExprSingleWithPrec(b *strings.Builder, child ast.Expr, parentOp ast.BinaryOp, isLeft bool) {
	if childBin, ok := child.(*ast.BinaryExpr); ok {
		childPrec := opPrecedence(childBin.Op)
		parentPrec := opPrecedence(parentOp)
		if childPrec < parentPrec || (childPrec == parentPrec && !isLeft) {
			b.WriteByte('(')
			writeExprSingle(b, child)
			b.WriteByte(')')
			return
		}
	}
	writeExprSingle(b, child)
}

func writeExprSingle(b *strings.Builder, expr ast.Expr) {
	switch e := expr.(type) {
	case *ast.NumberLit:
		fmt.Fprintf(b, "%g", e.Value)
	case *ast.BoolLit:
		if e.Value {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case *ast.StringLit:
		b.WriteString("\"" + e.Value + "\"")
	case *ast.HexColorLit:
		r := int(math.Round(e.R * 255))
		g := int(math.Round(e.G * 255))
		bl := int(math.Round(e.B * 255))
		if e.A < 1.0 {
			a := int(math.Round(e.A * 255))
			fmt.Fprintf(b, "#%02x%02x%02x%02x", r, g, bl, a)
		} else {
			fmt.Fprintf(b, "#%02x%02x%02x", r, g, bl)
		}
	case *ast.Ident:
		b.WriteString(e.Name)
	case *ast.VecLit:
		b.WriteByte('[')
		for i, elem := range e.Elems {
			if i > 0 {
				b.WriteString(", ")
			}
			writeExprSingle(b, elem)
		}
		b.WriteByte(']')
	case *ast.BinaryExpr:
		writeExprSingleWithPrec(b, e.Left, e.Op, true)
		b.WriteByte(' ')
		op := binaryOpString(e.Op)
		b.WriteString(op)
		if e.Blend != nil && isSmoothOrChamferOp(e.Op) {
			b.WriteString(formatNumberStr(*e.Blend))
		}
		b.WriteByte(' ')
		writeExprSingleWithPrec(b, e.Right, e.Op, false)
	case *ast.UnaryExpr:
		switch e.Op {
		case ast.Neg:
			b.WriteByte('-')
		case ast.Not:
			b.WriteByte('!')
		}
		writeExprSingle(b, e.Operand)
	case *ast.FuncCall:
		b.WriteString(e.Name)
		b.WriteByte('(')
		for i, arg := range e.Args {
			if i > 0 {
				b.WriteString(", ")
			}
			if arg.Name != "" {
				b.WriteString(arg.Name)
				b.WriteString(": ")
			}
			writeExprSingle(b, arg.Value)
		}
		b.WriteByte(')')
	case *ast.MethodCall:
		base, chain := collectMethodChain(e)
		if needsParens(base) {
			b.WriteByte('(')
			writeExprSingle(b, base)
			b.WriteByte(')')
		} else {
			writeExprSingle(b, base)
		}
		for _, link := range chain {
			b.WriteByte('.')
			b.WriteString(link.name)
			if len(link.args) > 0 {
				b.WriteByte('(')
				for i, arg := range link.args {
					if i > 0 {
						b.WriteString(", ")
					}
					if arg.Name != "" {
						b.WriteString(arg.Name)
						b.WriteString(": ")
					}
					writeExprSingle(b, arg.Value)
				}
				b.WriteByte(')')
			}
		}
	case *ast.Swizzle:
		writeExprSingle(b, e.Receiver)
		b.WriteByte('.')
		b.WriteString(e.Components)
	case *ast.Block:
		b.WriteString("{ ... }")
	case *ast.ForExpr:
		b.WriteString("for ... { ... }")
	case *ast.IfExpr:
		b.WriteString("if ... { ... }")
	case *ast.GlslEscape:
		b.WriteString("glsl(")
		b.WriteString(e.Param)
		b.WriteString(") { ... }")
	}
}

func argsToString(args []ast.Arg) string {
	var parts []string
	for _, arg := range args {
		s := ""
		if arg.Name != "" {
			s = arg.Name + ": "
		}
		s += exprToString(arg.Value)
		parts = append(parts, s)
	}
	return strings.Join(parts, ", ")
}

func formatNumberStr(v float64) string {
	return fmt.Sprintf("%g", v)
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

func isCSGOp(op ast.BinaryOp) bool {
	switch op {
	case ast.Union, ast.SmoothUnion, ast.ChamferUnion,
		ast.Subtract, ast.SmoothSubtract, ast.ChamferSubtract,
		ast.Intersect, ast.SmoothIntersect, ast.ChamferIntersect:
		return true
	}
	return false
}
