// Package lexer implements lexical analysis for the Chisel language.
// It converts source text into a stream of tokens with position tracking,
// automatic newline insertion (Go-style), and error diagnostics.
package lexer

import (
	"fmt"
	"unicode"

	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/token"
)

// keywords maps reserved words to their token kinds.
var keywords = map[string]token.TokenKind{
	"for":      token.TokFor,
	"in":       token.TokIn,
	"if":       token.TokIf,
	"else":     token.TokElse,
	"step":     token.TokStep,
	"light":    token.TokLight,
	"camera":   token.TokCamera,
	"bg":       token.TokBg,
	"raymarch": token.TokRaymarch,
	"post":     token.TokPost,
	"mat":      token.TokMat,
	"debug":    token.TokDebug,
	"glsl":     token.TokGlsl,
	"true":     token.TokTrue,
	"false":    token.TokFalse,
}

// canEndExpr returns true if the given token kind can end an expression,
// meaning a newline after it should be treated as a statement terminator.
func canEndExpr(k token.TokenKind) bool {
	switch k {
	case token.TokIdent, token.TokInt, token.TokFloat, token.TokHexColor,
		token.TokString, token.TokRParen, token.TokRBrack, token.TokRBrace,
		token.TokTrue, token.TokFalse:
		return true
	}
	return false
}

// isContinuation returns true if the given token kind is a continuation
// operator, meaning a newline after it should be suppressed.
func isContinuation(k token.TokenKind) bool {
	switch k {
	case token.TokPipe, token.TokPipeSmooth, token.TokPipeChamfer,
		token.TokAmp, token.TokAmpSmooth, token.TokAmpChamfer,
		token.TokMinus, token.TokMinusSmooth, token.TokMinusChamfer,
		token.TokPlus, token.TokStar, token.TokSlash, token.TokPercent,
		token.TokComma, token.TokAssign,
		token.TokLParen, token.TokLBrack, token.TokLBrace,
		token.TokDot, token.TokColon, token.TokArrow, token.TokDotDot,
		token.TokEq, token.TokNeq, token.TokLt, token.TokGt,
		token.TokLte, token.TokGte,
		token.TokBang:
		return true
	}
	return false
}

// isContinuationStart returns true if the given token kind at the START of a
// new line should suppress the preceding newline. This allows multi-line
// expressions like:
//
//	sphere
//	  .at(1, 0, 0)    ← dot continues
//	|~0.3              ← operator continues
//	box
func isContinuationStart(k token.TokenKind) bool {
	switch k {
	case token.TokDot,
		token.TokPipe, token.TokPipeSmooth, token.TokPipeChamfer,
		token.TokAmp, token.TokAmpSmooth, token.TokAmpChamfer,
		token.TokMinusSmooth, token.TokMinusChamfer,
		token.TokElse:
		// Note: TokMinus is NOT here — bare `-` at start of line is ambiguous
		// (could be unary negation for a new expression). Smooth/chamfer
		// variants (-~ -/) are unambiguous continuations.
		// TokElse continues an if expression across lines.
		return true
	}
	return false
}

// lexer holds the state during tokenization.
type lexer struct {
	filename string
	source   string
	pos      int // current byte offset
	line     int // 1-based line number
	col      int // 1-based column number

	tokens []token.Token
	diags  []diagnostic.Diagnostic
}

// Lex tokenizes the given source and returns a list of tokens (always ending
// with TokEOF) and any diagnostics encountered. It never panics.
func Lex(filename, source string) ([]token.Token, []diagnostic.Diagnostic) {
	l := &lexer{
		filename: filename,
		source:   source,
		pos:      0,
		line:     1,
		col:      1,
	}

	l.scan()
	l.insertNewlines()

	return l.tokens, l.diags
}

// currentPos returns the current source position.
func (l *lexer) currentPos() token.Position {
	return token.Position{
		File:   l.filename,
		Line:   l.line,
		Col:    l.col,
		Offset: l.pos,
	}
}

// peek returns the current byte, or 0 if at end.
func (l *lexer) peek() byte {
	if l.pos >= len(l.source) {
		return 0
	}
	return l.source[l.pos]
}

// peekAt returns the byte at offset i from current position, or 0 if out of bounds.
func (l *lexer) peekAt(i int) byte {
	p := l.pos + i
	if p >= len(l.source) {
		return 0
	}
	return l.source[p]
}

// advance moves forward one byte and updates line/col tracking.
func (l *lexer) advance() byte {
	if l.pos >= len(l.source) {
		return 0
	}
	ch := l.source[l.pos]
	l.pos++
	if ch == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return ch
}

// emit adds a token to the output list.
func (l *lexer) emit(kind token.TokenKind, value string, pos token.Position, length int) {
	l.tokens = append(l.tokens, token.Token{
		Kind:  kind,
		Value: value,
		Pos:   pos,
		Len:   length,
	})
}

// addDiag adds a diagnostic error at the given span.
func (l *lexer) addDiag(sev diagnostic.Severity, msg string, start, end token.Position) {
	l.diags = append(l.diags, diagnostic.Diagnostic{
		Severity: sev,
		Message:  msg,
		Span:     token.Span{Start: start, End: end},
	})
}

// scan performs the core lexical analysis, producing raw tokens without
// newline insertion logic.
func (l *lexer) scan() {
	for l.pos < len(l.source) {
		ch := l.peek()

		// Skip spaces and tabs (NOT newlines).
		if ch == ' ' || ch == '\t' {
			l.advance()
			continue
		}

		// Newlines are tracked but emitted as raw markers for later processing.
		if ch == '\n' {
			pos := l.currentPos()
			l.advance()
			l.emit(token.TokNewline, "\n", pos, 1)
			continue
		}

		// Carriage return: skip if followed by newline, otherwise treat as whitespace.
		if ch == '\r' {
			l.advance()
			continue
		}

		pos := l.currentPos()

		// Comments.
		if ch == '/' && l.peekAt(1) == '/' {
			l.scanLineComment(pos)
			continue
		}
		if ch == '/' && l.peekAt(1) == '*' {
			l.scanBlockComment(pos)
			continue
		}

		// Numbers.
		if ch >= '0' && ch <= '9' {
			l.scanNumber(pos)
			continue
		}

		// Hex colors.
		if ch == '#' {
			l.scanHexColor(pos)
			continue
		}

		// Strings.
		if ch == '"' || ch == '\'' {
			l.scanString(pos, ch)
			continue
		}

		// Identifiers and keywords.
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			l.scanIdentifier(pos)
			continue
		}

		// Operators and punctuation.
		l.scanPunctuation(pos)
	}

	// Always end with EOF.
	l.emit(token.TokEOF, "", l.currentPos(), 0)
}

// scanLineComment scans a // comment until end of line.
func (l *lexer) scanLineComment(pos token.Position) {
	start := l.pos
	// Skip the //
	l.advance()
	l.advance()
	for l.pos < len(l.source) && l.peek() != '\n' {
		l.advance()
	}
	value := l.source[start:l.pos]
	l.emit(token.TokComment, value, pos, len(value))
}

// scanBlockComment scans a /* ... */ comment.
func (l *lexer) scanBlockComment(pos token.Position) {
	start := l.pos
	// Skip the /*
	l.advance()
	l.advance()
	for l.pos < len(l.source) {
		if l.peek() == '*' && l.peekAt(1) == '/' {
			l.advance() // *
			l.advance() // /
			value := l.source[start:l.pos]
			l.emit(token.TokComment, value, pos, len(value))
			return
		}
		l.advance()
	}
	// Unterminated block comment.
	value := l.source[start:l.pos]
	l.emit(token.TokComment, value, pos, len(value))
	end := l.currentPos()
	l.addDiag(diagnostic.Error, "unterminated block comment", pos, end)
}

// scanNumber scans an integer or float literal, including scientific notation.
func (l *lexer) scanNumber(pos token.Position) {
	start := l.pos
	isFloat := false

	// Consume integer part.
	for l.pos < len(l.source) && l.peek() >= '0' && l.peek() <= '9' {
		l.advance()
	}

	// Decimal point: only if followed by a digit (so 0..8 lexes as Int DotDot Int).
	if l.pos < len(l.source) && l.peek() == '.' && l.peekAt(1) >= '0' && l.peekAt(1) <= '9' {
		isFloat = true
		l.advance() // consume '.'
		for l.pos < len(l.source) && l.peek() >= '0' && l.peek() <= '9' {
			l.advance()
		}
	}

	// Scientific notation: e or E, optionally followed by + or -.
	if l.pos < len(l.source) && (l.peek() == 'e' || l.peek() == 'E') {
		isFloat = true
		l.advance() // consume 'e'/'E'
		if l.pos < len(l.source) && (l.peek() == '+' || l.peek() == '-') {
			l.advance()
		}
		for l.pos < len(l.source) && l.peek() >= '0' && l.peek() <= '9' {
			l.advance()
		}
	}

	value := l.source[start:l.pos]
	if isFloat {
		l.emit(token.TokFloat, value, pos, len(value))
	} else {
		l.emit(token.TokInt, value, pos, len(value))
	}
}

// isHexDigit returns true if ch is a valid hexadecimal digit.
func isHexDigit(ch byte) bool {
	return (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') || (ch >= 'A' && ch <= 'F')
}

// scanHexColor scans a hex color literal (#rgb, #rrggbb, #rgba, #rrggbbaa).
func (l *lexer) scanHexColor(pos token.Position) {
	start := l.pos
	l.advance() // consume '#'

	// Count hex digits.
	hexStart := l.pos
	for l.pos < len(l.source) && isHexDigit(l.peek()) {
		l.advance()
	}
	hexLen := l.pos - hexStart

	value := l.source[start:l.pos]

	// Valid lengths: 3 (#rgb), 4 (#rgba), 6 (#rrggbb), 8 (#rrggbbaa).
	switch hexLen {
	case 3, 4, 6, 8:
		l.emit(token.TokHexColor, value, pos, len(value))
	default:
		l.emit(token.TokHexColor, value, pos, len(value))
		end := l.currentPos()
		l.addDiag(diagnostic.Error,
			fmt.Sprintf("invalid hex color %q: expected 3, 4, 6, or 8 hex digits, got %d", value, hexLen),
			pos, end)
	}
}

// scanString scans a double- or single-quoted string literal.
func (l *lexer) scanString(pos token.Position, quote byte) {
	start := l.pos
	l.advance() // consume opening quote

	for l.pos < len(l.source) {
		ch := l.peek()
		if ch == quote {
			l.advance() // consume closing quote
			value := l.source[start:l.pos]
			l.emit(token.TokString, value, pos, len(value))
			return
		}
		if ch == '\\' {
			l.advance() // consume backslash
			if l.pos < len(l.source) {
				l.advance() // consume escaped char
			}
			continue
		}
		if ch == '\n' {
			// Unterminated string at newline.
			break
		}
		l.advance()
	}

	// Unterminated string.
	value := l.source[start:l.pos]
	l.emit(token.TokString, value, pos, len(value))
	end := l.currentPos()
	l.addDiag(diagnostic.Error,
		fmt.Sprintf("unterminated string literal %s", value),
		pos, end)
}

// scanIdentifier scans an identifier or keyword.
func (l *lexer) scanIdentifier(pos token.Position) {
	start := l.pos
	for l.pos < len(l.source) {
		ch := l.peek()
		if ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') {
			l.advance()
		} else {
			break
		}
	}

	value := l.source[start:l.pos]
	if kind, ok := keywords[value]; ok {
		l.emit(kind, value, pos, len(value))
	} else {
		l.emit(token.TokIdent, value, pos, len(value))
	}
}

// scanPunctuation scans operator and punctuation tokens.
func (l *lexer) scanPunctuation(pos token.Position) {
	ch := l.peek()
	next := l.peekAt(1)

	switch ch {
	case '|':
		if next == '~' {
			l.advance()
			l.advance()
			l.emit(token.TokPipeSmooth, "|~", pos, 2)
		} else if next == '/' {
			l.advance()
			l.advance()
			l.emit(token.TokPipeChamfer, "|/", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokPipe, "|", pos, 1)
		}
	case '&':
		if next == '~' {
			l.advance()
			l.advance()
			l.emit(token.TokAmpSmooth, "&~", pos, 2)
		} else if next == '/' {
			l.advance()
			l.advance()
			l.emit(token.TokAmpChamfer, "&/", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokAmp, "&", pos, 1)
		}
	case '-':
		if next == '~' {
			l.advance()
			l.advance()
			l.emit(token.TokMinusSmooth, "-~", pos, 2)
		} else if next == '/' {
			l.advance()
			l.advance()
			l.emit(token.TokMinusChamfer, "-/", pos, 2)
		} else if next == '>' {
			l.advance()
			l.advance()
			l.emit(token.TokArrow, "->", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokMinus, "-", pos, 1)
		}
	case '+':
		l.advance()
		l.emit(token.TokPlus, "+", pos, 1)
	case '*':
		l.advance()
		l.emit(token.TokStar, "*", pos, 1)
	case '/':
		// Not preceded by |, &, or - (those are handled above), so this is division.
		l.advance()
		l.emit(token.TokSlash, "/", pos, 1)
	case '%':
		l.advance()
		l.emit(token.TokPercent, "%", pos, 1)
	case '(':
		l.advance()
		l.emit(token.TokLParen, "(", pos, 1)
	case ')':
		l.advance()
		l.emit(token.TokRParen, ")", pos, 1)
	case '[':
		l.advance()
		l.emit(token.TokLBrack, "[", pos, 1)
	case ']':
		l.advance()
		l.emit(token.TokRBrack, "]", pos, 1)
	case '{':
		l.advance()
		l.emit(token.TokLBrace, "{", pos, 1)
	case '}':
		l.advance()
		l.emit(token.TokRBrace, "}", pos, 1)
	case '.':
		if next == '.' {
			l.advance()
			l.advance()
			l.emit(token.TokDotDot, "..", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokDot, ".", pos, 1)
		}
	case ',':
		l.advance()
		l.emit(token.TokComma, ",", pos, 1)
	case ':':
		l.advance()
		l.emit(token.TokColon, ":", pos, 1)
	case '=':
		if next == '=' {
			l.advance()
			l.advance()
			l.emit(token.TokEq, "==", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokAssign, "=", pos, 1)
		}
	case '!':
		if next == '=' {
			l.advance()
			l.advance()
			l.emit(token.TokNeq, "!=", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokBang, "!", pos, 1)
		}
	case '<':
		if next == '=' {
			l.advance()
			l.advance()
			l.emit(token.TokLte, "<=", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokLt, "<", pos, 1)
		}
	case '>':
		if next == '=' {
			l.advance()
			l.advance()
			l.emit(token.TokGte, ">=", pos, 2)
		} else {
			l.advance()
			l.emit(token.TokGt, ">", pos, 1)
		}
	default:
		// Unexpected character.
		l.advance()
		end := l.currentPos()
		r := rune(ch)
		if ch >= 0x80 {
			// Try to identify the unicode character.
			r = []rune(l.source[pos.Offset:])[0]
		}
		l.addDiag(diagnostic.Error,
			fmt.Sprintf("unexpected character %q", r),
			pos, end)
	}
}

// insertNewlines processes the raw token stream to implement Go-style
// automatic newline insertion:
//  1. After tokens that can end an expression, if a raw newline follows,
//     insert a single TokNewline.
//  2. Suppress newline after continuation tokens.
//  3. Suppress newline before '.' (to preserve method chains across lines).
//  4. Collapse multiple consecutive TokNewline tokens into one.
func (l *lexer) insertNewlines() {
	raw := l.tokens
	l.tokens = make([]token.Token, 0, len(raw))

	for i := 0; i < len(raw); i++ {
		tok := raw[i]

		// Skip raw newline tokens; they are processed contextually below.
		if tok.Kind == token.TokNewline {
			continue
		}

		// Skip comment tokens, but note that they should not affect newline insertion.
		// Comments are emitted as-is. Newline logic looks at the previous non-comment,
		// non-newline token.
		l.tokens = append(l.tokens, tok)

		if tok.Kind == token.TokEOF {
			break
		}

		// Check if any raw newlines follow (possibly across comments).
		// We need to find the next non-newline, non-comment token and see if
		// there were newlines between here and there.
		// We also track which comments appear before vs after the first newline,
		// so that "sphere // comment\nbox" emits [Ident, Comment, Newline, Ident].
		hasNewline := false
		j := i + 1
		var commentsBefore []token.Token // comments before first newline
		var commentsAfter []token.Token  // comments after first newline
		var firstNlPos token.Position
		for j < len(raw) {
			if raw[j].Kind == token.TokNewline {
				if !hasNewline {
					firstNlPos = raw[j].Pos
				}
				hasNewline = true
				j++
			} else if raw[j].Kind == token.TokComment {
				if hasNewline {
					commentsAfter = append(commentsAfter, raw[j])
				} else {
					commentsBefore = append(commentsBefore, raw[j])
				}
				j++
			} else {
				break
			}
		}

		if !hasNewline {
			// No newline encountered; emit comments and continue.
			for _, c := range commentsBefore {
				l.tokens = append(l.tokens, c)
			}
			i = j - 1
			continue
		}

		// There is at least one newline. Decide whether to emit it.

		// Determine the effective previous kind: if there are comments before
		// the newline, the last significant token for newline purposes is still
		// the token we emitted (not the comment). However, if the token before
		// the newline is a comment and the token before that canEndExpr, we
		// still consider it as ending an expression.
		prevKind := tok.Kind

		// Find the next non-comment, non-newline token kind.
		var nextKind token.TokenKind
		if j < len(raw) {
			nextKind = raw[j].Kind
		} else {
			nextKind = token.TokEOF
		}

		shouldEmit := false
		if canEndExpr(prevKind) {
			// Suppress newline before continuation tokens:
			// dot (method chain), operators (multi-line boolean expressions)
			if !isContinuationStart(nextKind) {
				shouldEmit = true
			}
		}

		// Emit comments that appeared before the newline.
		for _, c := range commentsBefore {
			l.tokens = append(l.tokens, c)
		}

		if shouldEmit {
			l.tokens = append(l.tokens, token.Token{
				Kind:  token.TokNewline,
				Value: "\n",
				Pos:   firstNlPos,
				Len:   1,
			})
		}

		// Emit comments that appeared after the newline.
		for _, c := range commentsAfter {
			l.tokens = append(l.tokens, c)
		}
		i = j - 1
	}

	// Ensure the last token is always EOF.
	if len(l.tokens) == 0 || l.tokens[len(l.tokens)-1].Kind != token.TokEOF {
		l.tokens = append(l.tokens, token.Token{
			Kind: token.TokEOF,
			Pos:  token.Position{File: l.filename, Line: l.line, Col: l.col, Offset: l.pos},
		})
	}

	// Collapse consecutive newlines: remove any TokNewline that is immediately
	// followed by another TokNewline.
	collapsed := make([]token.Token, 0, len(l.tokens))
	for i, t := range l.tokens {
		if t.Kind == token.TokNewline && i+1 < len(l.tokens) && l.tokens[i+1].Kind == token.TokNewline {
			continue
		}
		collapsed = append(collapsed, t)
	}
	l.tokens = collapsed

	// Also strip leading newlines (before any real token).
	for len(l.tokens) > 0 && l.tokens[0].Kind == token.TokNewline {
		l.tokens = l.tokens[1:]
	}

	// Handle unicode for unexpected char diagnostics (fix any byte-level issues).
	_ = unicode.IsLetter // ensure unicode is used
}
