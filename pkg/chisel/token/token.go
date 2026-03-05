// Package token defines token types, positions, and spans for the Chisel lexer.
package token

import "fmt"

// TokenKind represents the type of a lexical token.
type TokenKind int

const (
	// Literals
	TokInt      TokenKind = iota // 42
	TokFloat                     // 3.14, 1e-5
	TokHexColor                  // #ff0000, #f00
	TokString                    // "hello"

	// Identifiers & Keywords
	TokIdent    // myVar, sphere, x, y, z
	TokFor      // for
	TokIn       // in
	TokIf       // if
	TokElse     // else
	TokStep     // step
	TokLight    // light
	TokCamera   // camera
	TokBg       // bg
	TokRaymarch // raymarch
	TokPost     // post
	TokMat      // mat
	TokDebug    // debug
	TokGlsl     // glsl
	TokTrue     // true
	TokFalse    // false

	// Operators -- boolean (SDF)
	TokPipe         // |
	TokPipeSmooth   // |~
	TokPipeChamfer  // |/
	TokMinus        // -
	TokMinusSmooth  // -~
	TokMinusChamfer // -/
	TokAmp          // &
	TokAmpSmooth    // &~
	TokAmpChamfer   // &/

	// Operators -- arithmetic
	TokPlus    // +
	TokStar    // *
	TokSlash   // /
	TokPercent // %

	// Comparison
	TokEq  // ==
	TokNeq // !=
	TokLt  // <
	TokGt  // >
	TokLte // <=
	TokGte // >=

	// Punctuation
	TokLParen // (
	TokRParen // )
	TokLBrack // [
	TokRBrack // ]
	TokLBrace // {
	TokRBrace // }
	TokDot    // .
	TokComma  // ,
	TokColon  // :
	TokAssign // =
	TokArrow  // ->
	TokDotDot // ..
	TokBang   // !

	// Special
	TokNewline // significant for implicit union
	TokComment // // ... or /* ... */
	TokEOF

	// tokenKindCount is not a real token; it marks the end of the enum
	// for testing purposes.
	tokenKindCount
)

// TokenKindCount returns the number of defined TokenKind values
// (excluding the sentinel). Useful for tests.
func TokenKindCount() int {
	return int(tokenKindCount)
}

var tokenKindNames = [...]string{
	TokInt:      "Int",
	TokFloat:    "Float",
	TokHexColor: "HexColor",
	TokString:   "String",

	TokIdent:    "Ident",
	TokFor:      "for",
	TokIn:       "in",
	TokIf:       "if",
	TokElse:     "else",
	TokStep:     "step",
	TokLight:    "light",
	TokCamera:   "camera",
	TokBg:       "bg",
	TokRaymarch: "raymarch",
	TokPost:     "post",
	TokMat:      "mat",
	TokDebug:    "debug",
	TokGlsl:     "glsl",
	TokTrue:     "true",
	TokFalse:    "false",

	TokPipe:         "|",
	TokPipeSmooth:   "|~",
	TokPipeChamfer:  "|/",
	TokMinus:        "-",
	TokMinusSmooth:  "-~",
	TokMinusChamfer: "-/",
	TokAmp:          "&",
	TokAmpSmooth:    "&~",
	TokAmpChamfer:   "&/",

	TokPlus:    "+",
	TokStar:    "*",
	TokSlash:   "/",
	TokPercent: "%",

	TokEq:  "==",
	TokNeq: "!=",
	TokLt:  "<",
	TokGt:  ">",
	TokLte: "<=",
	TokGte: ">=",

	TokLParen: "(",
	TokRParen: ")",
	TokLBrack: "[",
	TokRBrack: "]",
	TokLBrace: "{",
	TokRBrace: "}",
	TokDot:    ".",
	TokComma:  ",",
	TokColon:  ":",
	TokAssign: "=",
	TokArrow:  "->",
	TokDotDot: "..",
	TokBang:   "!",

	TokNewline: "Newline",
	TokComment: "Comment",
	TokEOF:     "EOF",
}

// String returns a human-readable name for the token kind.
func (k TokenKind) String() string {
	if int(k) >= 0 && int(k) < len(tokenKindNames) {
		if s := tokenKindNames[k]; s != "" {
			return s
		}
	}
	return fmt.Sprintf("TokenKind(%d)", int(k))
}

// Position represents a location in a source file.
type Position struct {
	File   string // source file name
	Line   int    // 1-based line number
	Col    int    // 1-based column (byte offset within line)
	Offset int    // byte offset from start of file
}

// String returns a human-readable representation like "file.chisel:3:8".
func (p Position) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Col)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

// Span represents a range in the source text, from Start (inclusive) to End (exclusive).
type Span struct {
	Start Position
	End   Position
}

// String returns a human-readable representation of the span.
func (s Span) String() string {
	if s.Start.File != "" {
		return fmt.Sprintf("%s:%d:%d..%d:%d", s.Start.File, s.Start.Line, s.Start.Col, s.End.Line, s.End.Col)
	}
	return fmt.Sprintf("%d:%d..%d:%d", s.Start.Line, s.Start.Col, s.End.Line, s.End.Col)
}

// Token is a single lexical token produced by the lexer.
type Token struct {
	Kind  TokenKind
	Value string   // raw text from source
	Pos   Position // start position in source
	Len   int      // byte length in source
}

// TokenSpan returns the source span covered by this token.
func (t Token) TokenSpan() Span {
	end := t.Pos
	end.Offset += t.Len
	end.Col += t.Len // approximate; accurate for single-line tokens
	return Span{Start: t.Pos, End: end}
}

// String returns a debug representation like `Ident("sphere")` or `EOF`.
func (t Token) String() string {
	switch t.Kind {
	case TokEOF, TokNewline:
		return t.Kind.String()
	default:
		return fmt.Sprintf("%s(%q)", t.Kind, t.Value)
	}
}
