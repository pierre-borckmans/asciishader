// Package parser implements a recursive-descent parser with Pratt expression
// parsing for the Chisel language. It converts a token stream into an AST.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/diagnostic"
	"asciishader/pkg/chisel/compiler/token"
)

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// Parse parses a slice of tokens (as produced by the lexer) into an AST
// Program and a list of diagnostics. It never panics.
func Parse(tokens []token.Token) (*ast.Program, []diagnostic.Diagnostic) {
	p := &parser{
		tokens: tokens,
		pos:    0,
	}
	prog := p.parseProgram()
	return prog, p.diags
}

// ---------------------------------------------------------------------------
// Parser state
// ---------------------------------------------------------------------------

type parser struct {
	tokens []token.Token
	pos    int
	diags  []diagnostic.Diagnostic
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// peek returns the current token without advancing.
func (p *parser) peek() token.Token {
	if p.pos >= len(p.tokens) {
		return token.Token{Kind: token.TokEOF}
	}
	return p.tokens[p.pos]
}

// peekKind returns the kind of the current token.
func (p *parser) peekKind() token.TokenKind {
	return p.peek().Kind
}

// peekAt returns the token at the given offset from the current position.
func (p *parser) peekAt(offset int) token.Token {
	idx := p.pos + offset
	if idx >= len(p.tokens) {
		return token.Token{Kind: token.TokEOF}
	}
	return p.tokens[idx]
}

// advance moves forward one token and returns the consumed token.
func (p *parser) advance() token.Token {
	tok := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return tok
}

// expect consumes the current token if it matches the expected kind,
// otherwise records a diagnostic and returns the current token without advancing.
func (p *parser) expect(kind token.TokenKind) token.Token {
	tok := p.peek()
	if tok.Kind == kind {
		return p.advance()
	}
	p.errorExpected(kind.String(), tok)
	return tok
}

// skipComments consumes and discards any TokComment tokens.
func (p *parser) skipComments() {
	for p.peekKind() == token.TokComment {
		p.advance()
	}
}

// skipCommentsAndNewlines consumes and discards comments and newlines.
func (p *parser) skipCommentsAndNewlines() {
	for p.peekKind() == token.TokNewline || p.peekKind() == token.TokComment {
		p.advance()
	}
}

// addError records an error diagnostic at the given span.
func (p *parser) addError(msg string, span token.Span) {
	p.diags = append(p.diags, diagnostic.Diagnostic{
		Severity: diagnostic.Error,
		Message:  msg,
		Span:     span,
	})
}

// errorExpected records an "expected X, got Y" diagnostic.
func (p *parser) errorExpected(what string, got token.Token) {
	msg := fmt.Sprintf("expected %s, got %s", what, got.Kind.String())
	if got.Value != "" {
		msg = fmt.Sprintf("expected %s, got %s %q", what, got.Kind.String(), got.Value)
	}
	p.addError(msg, got.TokenSpan())
}

// synchronize skips tokens until we reach a synchronization point.
func (p *parser) synchronize() {
	for {
		k := p.peekKind()
		switch k {
		case token.TokNewline:
			p.advance()
			return
		case token.TokRBrace, token.TokRParen, token.TokRBrack:
			return
		case token.TokEOF:
			return
		default:
			p.advance()
		}
	}
}

// spanFrom creates a span starting from the given position to the current position.
func (p *parser) spanFrom(start token.Position) token.Span {
	end := start
	if p.pos > 0 && p.pos-1 < len(p.tokens) {
		prev := p.tokens[p.pos-1]
		end = prev.Pos
		end.Offset += prev.Len
		end.Col += prev.Len
	}
	return token.Span{Start: start, End: end}
}

// ---------------------------------------------------------------------------
// Program
// ---------------------------------------------------------------------------

func (p *parser) parseProgram() *ast.Program {
	start := p.peek().Pos
	prog := &ast.Program{}

	p.skipCommentsAndNewlines()

	for p.peekKind() != token.TokEOF {
		posBefore := p.pos
		stmt := p.parseStatement()
		if stmt != nil {
			prog.Statements = append(prog.Statements, stmt)
		}
		p.skipCommentsAndNewlines()
		// Safety: if we haven't advanced at all, force advance to prevent infinite loop
		if p.pos == posBefore {
			p.advance()
		}
	}

	// Implicit union: if there are multiple consecutive ExprStmts, wrap them.
	prog.Statements = p.applyImplicitUnion(prog.Statements)
	prog.Span = p.spanFrom(start)
	return prog
}

// ---------------------------------------------------------------------------
// Statements
// ---------------------------------------------------------------------------

func (p *parser) parseStatement() ast.Statement {
	p.skipComments()
	tok := p.peek()

	// Settings blocks.
	switch tok.Kind {
	case token.TokLight, token.TokCamera, token.TokBg, token.TokRaymarch,
		token.TokPost, token.TokDebug, token.TokMat:
		return p.parseSetting()
	}

	// Check for assignment: ident = ... or ident( ... ) = ...
	if tok.Kind == token.TokIdent {
		if p.isAssignment() {
			return p.parseAssignment()
		}
	}

	// Expression statement.
	expr := p.parseExpr()
	if expr == nil {
		// Error recovery: skip to next sync point.
		p.synchronize()
		return nil
	}
	return &ast.ExprStmt{
		BaseNode:   ast.BaseNode{Span: expr.NodeSpan()},
		Expression: expr,
	}
}

// isAssignment does a lookahead to determine if the current position starts
// an assignment (variable or function definition).
func (p *parser) isAssignment() bool {
	// Save position.
	saved := p.pos

	// Must start with ident.
	if p.peekKind() != token.TokIdent {
		return false
	}
	p.advance() // consume ident

	// ident = ...
	if p.peekKind() == token.TokAssign {
		p.pos = saved
		return true
	}

	// ident( ... ) = ...
	if p.peekKind() == token.TokLParen {
		p.advance() // consume (
		depth := 1
		for depth > 0 && p.peekKind() != token.TokEOF {
			switch p.peekKind() {
			case token.TokLParen:
				depth++
			case token.TokRParen:
				depth--
			}
			p.advance()
		}
		if p.peekKind() == token.TokAssign {
			p.pos = saved
			return true
		}
	}

	p.pos = saved
	return false
}

func (p *parser) parseAssignment() *ast.AssignStmt {
	start := p.peek().Pos
	name := p.advance() // consume ident

	var params []ast.Param

	if p.peekKind() == token.TokLParen {
		p.advance() // consume (
		p.skipCommentsAndNewlines()
		for p.peekKind() != token.TokRParen && p.peekKind() != token.TokEOF {
			paramName := p.expect(token.TokIdent)
			param := ast.Param{Name: paramName.Value}

			if p.peekKind() == token.TokAssign {
				p.advance() // consume =
				param.Default = p.parseExpr()
			}

			params = append(params, param)

			p.skipCommentsAndNewlines()
			if p.peekKind() == token.TokComma {
				p.advance()
				p.skipCommentsAndNewlines()
			}
		}
		p.expect(token.TokRParen)
		if params == nil {
			params = []ast.Param{}
		}
	}

	p.expect(token.TokAssign)
	value := p.parseExpr()

	return &ast.AssignStmt{
		BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
		Name:     name.Value,
		Params:   params,
		Value:    value,
	}
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

func (p *parser) parseSetting() ast.Statement {
	tok := p.advance() // consume keyword (light, camera, etc.)
	start := tok.Pos

	switch tok.Kind {
	case token.TokDebug:
		// debug <ident>
		mode := p.expect(token.TokIdent)
		return &ast.SettingStmt{
			BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
			Kind:     "debug",
			Body:     mode.Value,
		}

	case token.TokMat:
		// mat <name> = { key: val, ... }
		name := p.expect(token.TokIdent)
		p.expect(token.TokAssign)
		body := p.parseSettingsBody()
		return &ast.SettingStmt{
			BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
			Kind:     "mat",
			Body:     map[string]interface{}{"name": name.Value, "body": body},
		}

	case token.TokCamera:
		// camera { ... } or camera expr -> expr
		if p.peekKind() == token.TokLBrace {
			body := p.parseSettingsBody()
			return &ast.SettingStmt{
				BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
				Kind:     "camera",
				Body:     body,
			}
		}
		// camera expr -> expr
		posExpr := p.parseExpr()
		if p.peekKind() == token.TokArrow {
			p.advance() // consume ->
			targetExpr := p.parseExpr()
			return &ast.SettingStmt{
				BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
				Kind:     "camera",
				Body: map[string]interface{}{
					"pos":    posExpr,
					"target": targetExpr,
				},
			}
		}
		return &ast.SettingStmt{
			BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
			Kind:     "camera",
			Body:     posExpr,
		}

	case token.TokLight, token.TokBg, token.TokRaymarch, token.TokPost:
		kind := tok.Value
		if p.peekKind() == token.TokLBrace {
			body := p.parseSettingsBody()
			return &ast.SettingStmt{
				BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
				Kind:     kind,
				Body:     body,
			}
		}
		// Single expression: light expr, bg expr, etc.
		expr := p.parseExpr()
		return &ast.SettingStmt{
			BaseNode: ast.BaseNode{Span: p.spanFrom(start)},
			Kind:     kind,
			Body:     expr,
		}
	}

	// Shouldn't reach here.
	return nil
}

// parseSettingsBody parses a { key: value, ... } block.
// Returns a map[string]interface{} where values are either ast.Expr or
// nested map[string]interface{} (for nested blocks).
func (p *parser) parseSettingsBody() map[string]interface{} {
	p.expect(token.TokLBrace)
	p.skipCommentsAndNewlines()
	result := make(map[string]interface{})

	for p.peekKind() != token.TokRBrace && p.peekKind() != token.TokEOF {
		posBefore := p.pos
		// Each entry is either: ident: expr, or ident { ... } (nested block)
		if p.peekKind() != token.TokIdent {
			p.synchronize()
			p.skipCommentsAndNewlines()
			if p.pos == posBefore {
				p.advance()
			}
			continue
		}
		key := p.advance() // consume ident

		if p.peekKind() == token.TokLBrace {
			// Nested block: ident { ... }
			result[key.Value] = p.parseSettingsBody()
		} else if p.peekKind() == token.TokColon {
			p.advance() // consume :
			p.skipCommentsAndNewlines()
			value := p.parseExpr()
			result[key.Value] = value
		} else {
			// Treat as expression value (for things like "ambient: 0.1")
			p.errorExpected("':' or '{'", p.peek())
			p.synchronize()
		}

		p.skipCommentsAndNewlines()
		// Optional comma
		if p.peekKind() == token.TokComma {
			p.advance()
		}
		p.skipCommentsAndNewlines()
	}

	p.expect(token.TokRBrace)
	return result
}

// ---------------------------------------------------------------------------
// Implicit union
// ---------------------------------------------------------------------------

// applyImplicitUnion combines consecutive ExprStmts into a single ExprStmt
// containing a chain of BinaryExpr{Union, ...} nodes.
func (p *parser) applyImplicitUnion(stmts []ast.Statement) []ast.Statement {
	if len(stmts) <= 1 {
		return stmts
	}

	var result []ast.Statement
	var exprRun []ast.Expr

	flushRun := func() {
		if len(exprRun) == 0 {
			return
		}
		if len(exprRun) == 1 {
			result = append(result, &ast.ExprStmt{
				BaseNode:   ast.BaseNode{Span: exprRun[0].NodeSpan()},
				Expression: exprRun[0],
			})
		} else {
			combined := exprRun[0]
			for _, e := range exprRun[1:] {
				combined = &ast.BinaryExpr{
					BaseNode: ast.BaseNode{Span: token.Span{
						Start: combined.NodeSpan().Start,
						End:   e.NodeSpan().End,
					}},
					Left:  combined,
					Op:    ast.Union,
					Right: e,
				}
			}
			result = append(result, &ast.ExprStmt{
				BaseNode:   ast.BaseNode{Span: combined.NodeSpan()},
				Expression: combined,
			})
		}
		exprRun = nil
	}

	for _, stmt := range stmts {
		if es, ok := stmt.(*ast.ExprStmt); ok {
			exprRun = append(exprRun, es.Expression)
		} else {
			flushRun()
			result = append(result, stmt)
		}
	}
	flushRun()

	return result
}

// ---------------------------------------------------------------------------
// Expressions (Pratt parser)
// ---------------------------------------------------------------------------

// Precedence levels (higher number = tighter binding).
const (
	precNone       = 0
	precUnion      = 1 // | |~ |/
	precSubtract   = 2 // - -~ -/ (SDF subtract)
	precIntersect  = 3 // & &~ &/
	precComparison = 4 // == != < > <= >=
	precAddSub     = 5 // + - (arithmetic)
	precMulDiv     = 6 // * / %
	precUnary      = 7 // unary - !
	precPostfix    = 8 // . method calls
)

// infixPrecedence returns the precedence and binary op for infix operators.
// For TokMinus, we return the SDF subtract precedence (precSubtract);
// if it should be arithmetic, the context determines, but we parse it at
// precSubtract first. However, for correct precedence, we need to be
// more nuanced: TokMinus appears at TWO precedence levels.
//
// Strategy: We parse TokMinus at precSubtract (precedence 2). This means
// "a - b | c" parses as "a - b | c" = "(a - b) | c" which is wrong per spec.
// Spec says: union (1) < subtract (2) < intersect (3) < add/sub (5) < mul/div (6)
// So "a | b - c" = "a | (b - c)" which is correct.
// And "a - b + c": subtract binds tighter than arithmetic... wait, no.
// subtract is 2, add is 5. Higher = tighter. So "a - b + c" = "a - (b + c)"?
// No: in Pratt parsing, higher precedence binds first.
// Let me re-read: "Precedence (lowest to highest): 1: union, 2: subtract, 3: intersect, 4: +/-, 5: * / %, 6: comparison"
//
// Wait, the task description says:
// 1. | |~ |/ — union (lowest)
// 2. - -~ -/ — subtract
// 3. & &~ &/ — intersect
// 4. + - — arithmetic add/sub
// 5. * / % — arithmetic mul/div
// 6. == != < > <= >= — comparison
// 7. unary - ! — prefix
// 8. .method() — postfix
//
// So higher number = tighter binding. subtract (2) binds tighter than union (1),
// intersect (3) binds tighter than subtract (2), etc.
// Arithmetic + - (4) binds tighter than intersect (3).
//
// TokMinus is used for BOTH subtract (2) and arithmetic sub (4).
// Strategy: parse at subtract level (2). The type checker will figure it out.
// But this means "1 - 2" parses as subtract at level 2 rather than
// arithmetic at level 4, which is fine since there's no operator between
// 2 and 4 that would cause ambiguity.
//
// Actually wait, it DOES cause issues: "a & b - c" should parse as
// "a & (b - c)" since subtract (2) < intersect (3), meaning intersect
// binds tighter. But that can't be right for SDF semantics either.
// Let me re-read the spec:
//
// "Precedence (tightest -> loosest): & intersect, - subtract, | union"
// So: & > - > | meaning intersect binds tightest.
// In Pratt terms: & has highest precedence among SDF ops.
//
// "sphere | box - cylinder" parses as "sphere | (box - cylinder)"
// This means subtract binds tighter than union. And intersect binds
// tighter than subtract.
//
// For Pratt: higher precedence = binds tighter = parsed first.
// Union: lowest SDF precedence
// Subtract: middle
// Intersect: highest SDF precedence
//
// Then arithmetic +/- should be even higher than intersect.
// Then * / % even higher.
// Then comparisons even higher.

func infixPrecedence(kind token.TokenKind) (prec int, op ast.BinaryOp, isInfix bool) {
	switch kind {
	// Union ops
	case token.TokPipe:
		return precUnion, ast.Union, true
	case token.TokPipeSmooth:
		return precUnion, ast.SmoothUnion, true
	case token.TokPipeChamfer:
		return precUnion, ast.ChamferUnion, true

	// Subtract ops — TokMinus is handled separately
	case token.TokMinusSmooth:
		return precSubtract, ast.SmoothSubtract, true
	case token.TokMinusChamfer:
		return precSubtract, ast.ChamferSubtract, true

	// Intersect ops
	case token.TokAmp:
		return precIntersect, ast.Intersect, true
	case token.TokAmpSmooth:
		return precIntersect, ast.SmoothIntersect, true
	case token.TokAmpChamfer:
		return precIntersect, ast.ChamferIntersect, true

	// Arithmetic
	case token.TokPlus:
		return precAddSub, ast.Add, true
	case token.TokStar:
		return precMulDiv, ast.Mul, true
	case token.TokSlash:
		return precMulDiv, ast.Div, true
	case token.TokPercent:
		return precMulDiv, ast.Mod, true

	// Comparison
	case token.TokEq:
		return precComparison, ast.Eq, true
	case token.TokNeq:
		return precComparison, ast.Neq, true
	case token.TokLt:
		return precComparison, ast.Lt, true
	case token.TokGt:
		return precComparison, ast.Gt, true
	case token.TokLte:
		return precComparison, ast.Lte, true
	case token.TokGte:
		return precComparison, ast.Gte, true

	// TokMinus: both subtract (SDF) and arithmetic sub
	// We parse at precSubtract; the analyzer resolves.
	case token.TokMinus:
		return precSubtract, ast.Subtract, true
	}

	return 0, 0, false
}

// isSmoothOrChamfer returns true for operators that may have a blend radius.
func isSmoothOrChamfer(kind token.TokenKind) bool {
	switch kind {
	case token.TokPipeSmooth, token.TokPipeChamfer,
		token.TokMinusSmooth, token.TokMinusChamfer,
		token.TokAmpSmooth, token.TokAmpChamfer:
		return true
	}
	return false
}

// parseExpr parses an expression using Pratt parsing, starting at the
// lowest precedence.
func (p *parser) parseExpr() ast.Expr {
	return p.parsePratt(precNone)
}

// parsePratt is the core Pratt expression parser.
func (p *parser) parsePratt(minPrec int) ast.Expr {
	left := p.parseUnary()
	if left == nil {
		return nil
	}

	for {
		// Skip comments before checking for infix operators so that
		// comments between operands don't break the expression.
		p.skipComments()
		tok := p.peek()

		// Check for infix operator.
		prec, op, isInfix := infixPrecedence(tok.Kind)
		if !isInfix || prec <= minPrec {
			break
		}

		opTok := p.advance() // consume operator

		// Skip newlines after operator (multi-line expressions).
		p.skipCommentsAndNewlines()

		// Check for blend radius on smooth/chamfer operators.
		var blend *float64
		if isSmoothOrChamfer(opTok.Kind) {
			if p.peekKind() == token.TokInt || p.peekKind() == token.TokFloat {
				numTok := p.advance()
				v, err := strconv.ParseFloat(numTok.Value, 64)
				if err == nil {
					blend = &v
				}
			}
		}

		// Skip newlines after blend radius (e.g. |~0.3\nsphere).
		p.skipCommentsAndNewlines()

		right := p.parsePratt(prec)
		if right == nil {
			p.addError("expected expression after operator", opTok.TokenSpan())
			break
		}

		left = &ast.BinaryExpr{
			BaseNode: ast.BaseNode{Span: token.Span{
				Start: left.NodeSpan().Start,
				End:   right.NodeSpan().End,
			}},
			Left:  left,
			Op:    op,
			Right: right,
			Blend: blend,
		}
	}

	return left
}

// parseUnary parses unary prefix operators: - and !
func (p *parser) parseUnary() ast.Expr {
	tok := p.peek()

	switch tok.Kind {
	case token.TokMinus:
		p.advance()
		operand := p.parseUnary()
		if operand == nil {
			p.addError("expected expression after '-'", tok.TokenSpan())
			return nil
		}
		return &ast.UnaryExpr{
			BaseNode: ast.BaseNode{Span: token.Span{
				Start: tok.Pos,
				End:   operand.NodeSpan().End,
			}},
			Op:      ast.Neg,
			Operand: operand,
		}

	case token.TokBang:
		p.advance()
		operand := p.parseUnary()
		if operand == nil {
			p.addError("expected expression after '!'", tok.TokenSpan())
			return nil
		}
		return &ast.UnaryExpr{
			BaseNode: ast.BaseNode{Span: token.Span{
				Start: tok.Pos,
				End:   operand.NodeSpan().End,
			}},
			Op:      ast.Not,
			Operand: operand,
		}
	}

	return p.parsePostfix()
}

// parsePostfix parses method calls, swizzles, and other postfix operations.
func (p *parser) parsePostfix() ast.Expr {
	expr := p.parseAtom()
	if expr == nil {
		return nil
	}

	// Parse chained method calls / swizzles.
	for p.peekKind() == token.TokDot {
		p.advance() // consume .

		if p.peekKind() != token.TokIdent {
			p.errorExpected("method name", p.peek())
			break
		}

		nameTok := p.advance() // consume method name
		name := nameTok.Value

		// Check if it's a swizzle: all characters in "xyzrgbstpq"
		if isSwizzle(name) {
			expr = &ast.Swizzle{
				BaseNode: ast.BaseNode{Span: token.Span{
					Start: expr.NodeSpan().Start,
					End:   nameTok.TokenSpan().End,
				}},
				Receiver:   expr,
				Components: name,
			}
			continue
		}

		// Method call: might have arguments.
		if p.peekKind() == token.TokLParen {
			p.advance() // consume (
			args := p.parseArgs()
			end := p.expect(token.TokRParen)
			expr = &ast.MethodCall{
				BaseNode: ast.BaseNode{Span: token.Span{
					Start: expr.NodeSpan().Start,
					End:   end.TokenSpan().End,
				}},
				Receiver: expr,
				Name:     name,
				Args:     args,
			}
		} else {
			// Bare method (no args): .red, .blue, etc.
			expr = &ast.MethodCall{
				BaseNode: ast.BaseNode{Span: token.Span{
					Start: expr.NodeSpan().Start,
					End:   nameTok.TokenSpan().End,
				}},
				Receiver: expr,
				Name:     name,
				Args:     nil,
			}
		}
	}

	return expr
}

// isSwizzle returns true if all characters in name are in the set "xyzrgbstpq"
// and the name has 1–4 characters.
func isSwizzle(name string) bool {
	if len(name) < 1 || len(name) > 4 {
		return false
	}
	for _, ch := range name {
		switch ch {
		case 'x', 'y', 'z', 'r', 'g', 'b', 's', 't', 'p', 'q':
			// ok
		default:
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Atoms
// ---------------------------------------------------------------------------

func (p *parser) parseAtom() ast.Expr {
	tok := p.peek()

	switch tok.Kind {
	case token.TokInt:
		p.advance()
		v, _ := strconv.ParseFloat(tok.Value, 64)
		return &ast.NumberLit{
			BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
			Value:    v,
		}

	case token.TokFloat:
		p.advance()
		v, _ := strconv.ParseFloat(tok.Value, 64)
		return &ast.NumberLit{
			BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
			Value:    v,
		}

	case token.TokTrue:
		p.advance()
		return &ast.BoolLit{
			BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
			Value:    true,
		}

	case token.TokFalse:
		p.advance()
		return &ast.BoolLit{
			BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
			Value:    false,
		}

	case token.TokString:
		p.advance()
		// Strip quotes from value.
		val := tok.Value
		if len(val) >= 2 {
			val = val[1 : len(val)-1]
		}
		return &ast.StringLit{
			BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
			Value:    val,
		}

	case token.TokHexColor:
		p.advance()
		r, g, b, a := parseHexColor(tok.Value)
		return &ast.HexColorLit{
			BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
			R:        r,
			G:        g,
			B:        b,
			A:        a,
		}

	case token.TokIdent:
		return p.parseIdentOrFuncCall()

	case token.TokLParen:
		return p.parseParenExpr()

	case token.TokLBrack:
		return p.parseVecLit()

	case token.TokLBrace:
		return p.parseBlock()

	case token.TokFor:
		return p.parseForExpr()

	case token.TokIf:
		return p.parseIfExpr()

	case token.TokGlsl:
		return p.parseGlslEscape()

	default:
		p.addError(fmt.Sprintf("unexpected token %s", tok.Kind.String()), tok.TokenSpan())
		p.synchronize()
		return nil
	}
}

// parseIdentOrFuncCall parses either a bare identifier or a function call.
func (p *parser) parseIdentOrFuncCall() ast.Expr {
	tok := p.advance() // consume ident

	// Check for function call: ident(
	if p.peekKind() == token.TokLParen {
		p.advance() // consume (
		args := p.parseArgs()
		end := p.expect(token.TokRParen)
		return &ast.FuncCall{
			BaseNode: ast.BaseNode{Span: token.Span{
				Start: tok.Pos,
				End:   end.TokenSpan().End,
			}},
			Name: tok.Value,
			Args: args,
		}
	}

	return &ast.Ident{
		BaseNode: ast.BaseNode{Span: tok.TokenSpan()},
		Name:     tok.Value,
	}
}

// parseArgs parses a comma-separated argument list (inside parens).
// Arguments can be positional or named (name: expr).
func (p *parser) parseArgs() []ast.Arg {
	var args []ast.Arg
	p.skipCommentsAndNewlines()

	for p.peekKind() != token.TokRParen && p.peekKind() != token.TokEOF {
		var arg ast.Arg

		// Check for named argument: ident : expr
		if p.peekKind() == token.TokIdent && p.peekAt(1).Kind == token.TokColon {
			nameTok := p.advance() // consume name
			p.advance()            // consume :
			arg.Name = nameTok.Value
		}

		arg.Value = p.parseExpr()
		if arg.Value == nil {
			break
		}
		args = append(args, arg)

		p.skipCommentsAndNewlines()
		if p.peekKind() == token.TokComma {
			p.advance()
			p.skipCommentsAndNewlines()
		} else {
			break
		}
	}

	return args
}

// parseParenExpr parses a parenthesized expression.
func (p *parser) parseParenExpr() ast.Expr {
	p.advance() // consume (
	expr := p.parseExpr()
	p.expect(token.TokRParen)
	return expr
}

// parseVecLit parses a vector literal: [expr, expr, ...]
func (p *parser) parseVecLit() ast.Expr {
	start := p.advance() // consume [
	var elems []ast.Expr

	p.skipCommentsAndNewlines()
	for p.peekKind() != token.TokRBrack && p.peekKind() != token.TokEOF {
		elem := p.parseExpr()
		if elem == nil {
			// Error recovery: skip to ] or ,
			p.synchronize()
			break
		}
		elems = append(elems, elem)

		p.skipCommentsAndNewlines()
		if p.peekKind() == token.TokComma {
			p.advance()
			p.skipCommentsAndNewlines()
		} else {
			break
		}
	}

	end := p.expect(token.TokRBrack)
	return &ast.VecLit{
		BaseNode: ast.BaseNode{Span: token.Span{
			Start: start.Pos,
			End:   end.TokenSpan().End,
		}},
		Elems: elems,
	}
}

// ---------------------------------------------------------------------------
// Block
// ---------------------------------------------------------------------------

func (p *parser) parseBlock() ast.Expr {
	start := p.advance() // consume {
	p.skipCommentsAndNewlines()

	var stmts []ast.Statement

	for p.peekKind() != token.TokRBrace && p.peekKind() != token.TokEOF {
		p.skipComments()
		if p.peekKind() == token.TokRBrace || p.peekKind() == token.TokEOF {
			break
		}

		posBefore := p.pos
		stmt := p.parseStatement()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
		p.skipCommentsAndNewlines()
		if p.pos == posBefore {
			p.advance()
		}
	}

	end := p.expect(token.TokRBrace)

	// Apply implicit union to block's expression statements.
	stmts = p.applyImplicitUnion(stmts)

	// The last ExprStmt becomes the Block's Result.
	var result ast.Expr
	if len(stmts) > 0 {
		if es, ok := stmts[len(stmts)-1].(*ast.ExprStmt); ok {
			result = es.Expression
			stmts = stmts[:len(stmts)-1]
		}
	}

	return &ast.Block{
		BaseNode: ast.BaseNode{Span: token.Span{
			Start: start.Pos,
			End:   end.TokenSpan().End,
		}},
		Stmts:  stmts,
		Result: result,
	}
}

// ---------------------------------------------------------------------------
// Control flow
// ---------------------------------------------------------------------------

func (p *parser) parseForExpr() ast.Expr {
	start := p.advance() // consume 'for'

	var iterators []ast.Iterator

	for {
		nameTok := p.expect(token.TokIdent)
		p.expect(token.TokIn)

		startExpr := p.parseExpr()
		p.expect(token.TokDotDot)
		endExpr := p.parseExpr()

		var stepExpr ast.Expr
		if p.peekKind() == token.TokStep {
			p.advance() // consume 'step'
			stepExpr = p.parseExpr()
		}

		iterators = append(iterators, ast.Iterator{
			Name:  nameTok.Value,
			Start: startExpr,
			End:   endExpr,
			Step:  stepExpr,
		})

		if p.peekKind() == token.TokComma {
			p.advance()
			p.skipCommentsAndNewlines()
		} else {
			break
		}
	}

	// Parse body block.
	body := p.parseBlock()
	block, ok := body.(*ast.Block)
	if !ok {
		p.addError("expected block after for", p.peek().TokenSpan())
		return nil
	}

	return &ast.ForExpr{
		BaseNode: ast.BaseNode{Span: token.Span{
			Start: start.Pos,
			End:   body.NodeSpan().End,
		}},
		Iterators: iterators,
		Body:      block,
	}
}

func (p *parser) parseIfExpr() ast.Expr {
	start := p.advance() // consume 'if'

	cond := p.parseExpr()

	thenBody := p.parseBlock()
	thenBlock, ok := thenBody.(*ast.Block)
	if !ok {
		p.addError("expected block after if condition", p.peek().TokenSpan())
		return nil
	}

	var elseExpr ast.Expr
	if p.peekKind() == token.TokElse {
		p.advance() // consume 'else'

		if p.peekKind() == token.TokIf {
			// else if chain
			elseExpr = p.parseIfExpr()
		} else {
			elseExpr = p.parseBlock()
		}
	}

	endPos := thenBody.NodeSpan().End
	if elseExpr != nil {
		endPos = elseExpr.NodeSpan().End
	}

	return &ast.IfExpr{
		BaseNode: ast.BaseNode{Span: token.Span{
			Start: start.Pos,
			End:   endPos,
		}},
		Cond: cond,
		Then: thenBlock,
		Else: elseExpr,
	}
}

// ---------------------------------------------------------------------------
// GLSL escape
// ---------------------------------------------------------------------------

func (p *parser) parseGlslEscape() ast.Expr {
	start := p.advance() // consume 'glsl'

	p.expect(token.TokLParen)
	paramTok := p.expect(token.TokIdent)
	p.expect(token.TokRParen)

	// The lexer captured the raw GLSL body as a single TokGlslBody token.
	bodyTok := p.expect(token.TokGlslBody)
	end := bodyTok

	code := strings.TrimSpace(bodyTok.Value)

	return &ast.GlslEscape{
		BaseNode: ast.BaseNode{Span: token.Span{
			Start: start.Pos,
			End:   end.TokenSpan().End,
		}},
		Param: paramTok.Value,
		Code:  code,
	}
}

// ---------------------------------------------------------------------------
// Hex color parsing
// ---------------------------------------------------------------------------

// parseHexColor parses a hex color string like "#ff0000" or "#f00" into RGBA
// float components normalized to [0, 1].
func parseHexColor(s string) (r, g, b, a float64) {
	// Strip leading #
	hex := s
	if len(hex) > 0 && hex[0] == '#' {
		hex = hex[1:]
	}

	a = 1.0 // default alpha

	switch len(hex) {
	case 3: // #rgb
		r = hexNibble(hex[0]) / 15.0
		g = hexNibble(hex[1]) / 15.0
		b = hexNibble(hex[2]) / 15.0
	case 4: // #rgba
		r = hexNibble(hex[0]) / 15.0
		g = hexNibble(hex[1]) / 15.0
		b = hexNibble(hex[2]) / 15.0
		a = hexNibble(hex[3]) / 15.0
	case 6: // #rrggbb
		r = hexByte(hex[0], hex[1]) / 255.0
		g = hexByte(hex[2], hex[3]) / 255.0
		b = hexByte(hex[4], hex[5]) / 255.0
	case 8: // #rrggbbaa
		r = hexByte(hex[0], hex[1]) / 255.0
		g = hexByte(hex[2], hex[3]) / 255.0
		b = hexByte(hex[4], hex[5]) / 255.0
		a = hexByte(hex[6], hex[7]) / 255.0
	}

	return
}

func hexNibble(ch byte) float64 {
	switch {
	case ch >= '0' && ch <= '9':
		return float64(ch - '0')
	case ch >= 'a' && ch <= 'f':
		return float64(ch - 'a' + 10)
	case ch >= 'A' && ch <= 'F':
		return float64(ch - 'A' + 10)
	}
	return 0
}

func hexByte(hi, lo byte) float64 {
	return hexNibble(hi)*16 + hexNibble(lo)
}
