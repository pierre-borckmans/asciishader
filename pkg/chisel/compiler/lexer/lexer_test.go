package lexer

import (
	"fmt"
	"strings"
	"testing"

	"asciishader/pkg/chisel/compiler/token"
)

// tokInfo is a compact representation used in test expectations.
type tokInfo struct {
	Kind  token.TokenKind
	Value string // if empty, don't check value
}

func tok(kind token.TokenKind, value ...string) tokInfo {
	v := ""
	if len(value) > 0 {
		v = value[0]
	}
	return tokInfo{Kind: kind, Value: v}
}

// checkTokens is a test helper that verifies the token stream matches expectations.
func checkTokens(t *testing.T, source string, expected []tokInfo) {
	t.Helper()
	tokens, diags := Lex("test.chisel", source)

	if len(diags) > 0 {
		for _, d := range diags {
			t.Logf("diagnostic: %s", d.Error())
		}
	}

	if len(tokens) != len(expected) {
		t.Errorf("Lex(%q): got %d tokens, want %d", source, len(tokens), len(expected))
		for i, tok := range tokens {
			t.Logf("  token[%d]: %s", i, tok.String())
		}
		return
	}

	for i, exp := range expected {
		got := tokens[i]
		if got.Kind != exp.Kind {
			t.Errorf("Lex(%q): token[%d].Kind = %s, want %s", source, i, got.Kind, exp.Kind)
		}
		if exp.Value != "" && got.Value != exp.Value {
			t.Errorf("Lex(%q): token[%d].Value = %q, want %q", source, i, got.Value, exp.Value)
		}
	}
}

// --- Basic token tests ---

func TestEmptySource(t *testing.T) {
	checkTokens(t, "", []tokInfo{
		tok(token.TokEOF),
	})
}

func TestInteger(t *testing.T) {
	checkTokens(t, "42", []tokInfo{
		tok(token.TokInt, "42"),
		tok(token.TokEOF),
	})
}

func TestFloat(t *testing.T) {
	checkTokens(t, "3.14", []tokInfo{
		tok(token.TokFloat, "3.14"),
		tok(token.TokEOF),
	})
}

func TestScientificNotation(t *testing.T) {
	checkTokens(t, "1e-5", []tokInfo{
		tok(token.TokFloat, "1e-5"),
		tok(token.TokEOF),
	})
}

func TestScientificNotationWithFloat(t *testing.T) {
	checkTokens(t, "1.5e3", []tokInfo{
		tok(token.TokFloat, "1.5e3"),
		tok(token.TokEOF),
	})
}

func TestIdentifier(t *testing.T) {
	checkTokens(t, "sphere", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokEOF),
	})
}

func TestKeywordFor(t *testing.T) {
	checkTokens(t, "for", []tokInfo{
		tok(token.TokFor, "for"),
		tok(token.TokEOF),
	})
}

func TestKeywordTrue(t *testing.T) {
	checkTokens(t, "true", []tokInfo{
		tok(token.TokTrue, "true"),
		tok(token.TokEOF),
	})
}

func TestKeywordFalse(t *testing.T) {
	checkTokens(t, "false", []tokInfo{
		tok(token.TokFalse, "false"),
		tok(token.TokEOF),
	})
}

func TestHexColor6(t *testing.T) {
	checkTokens(t, "#ff0000", []tokInfo{
		tok(token.TokHexColor, "#ff0000"),
		tok(token.TokEOF),
	})
}

func TestHexColor3(t *testing.T) {
	checkTokens(t, "#f00", []tokInfo{
		tok(token.TokHexColor, "#f00"),
		tok(token.TokEOF),
	})
}

func TestHexColor8(t *testing.T) {
	checkTokens(t, "#ff000080", []tokInfo{
		tok(token.TokHexColor, "#ff000080"),
		tok(token.TokEOF),
	})
}

func TestHexColor4(t *testing.T) {
	checkTokens(t, "#f008", []tokInfo{
		tok(token.TokHexColor, "#f008"),
		tok(token.TokEOF),
	})
}

func TestLineComment(t *testing.T) {
	checkTokens(t, "// comment", []tokInfo{
		tok(token.TokComment, "// comment"),
		tok(token.TokEOF),
	})
}

func TestBlockComment(t *testing.T) {
	checkTokens(t, "/* block */sphere", []tokInfo{
		tok(token.TokComment, "/* block */"),
		tok(token.TokIdent, "sphere"),
		tok(token.TokEOF),
	})
}

func TestLineCommentBeforeNewline(t *testing.T) {
	checkTokens(t, "// comment\nsphere", []tokInfo{
		tok(token.TokComment),
		tok(token.TokIdent, "sphere"),
		tok(token.TokEOF),
	})
}

func TestDoubleQuoteString(t *testing.T) {
	checkTokens(t, `"hello"`, []tokInfo{
		tok(token.TokString, `"hello"`),
		tok(token.TokEOF),
	})
}

func TestSingleQuoteString(t *testing.T) {
	checkTokens(t, `'world'`, []tokInfo{
		tok(token.TokString, `'world'`),
		tok(token.TokEOF),
	})
}

// --- Operator tests ---

func TestPipe(t *testing.T) {
	checkTokens(t, "a | b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipe),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestPipeSmooth(t *testing.T) {
	checkTokens(t, "a |~ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipeSmooth, "|~"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestPipeChamfer(t *testing.T) {
	checkTokens(t, "a |/ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipeChamfer, "|/"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestMinus(t *testing.T) {
	checkTokens(t, "a - b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinus),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestMinusSmooth(t *testing.T) {
	checkTokens(t, "a -~ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinusSmooth, "-~"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestMinusChamfer(t *testing.T) {
	checkTokens(t, "a -/ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinusChamfer, "-/"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestAmp(t *testing.T) {
	checkTokens(t, "a & b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokAmp),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestAmpSmooth(t *testing.T) {
	checkTokens(t, "a &~ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokAmpSmooth, "&~"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestAmpChamfer(t *testing.T) {
	checkTokens(t, "a &/ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokAmpChamfer, "&/"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestArithmetic(t *testing.T) {
	checkTokens(t, "a * b + c", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokStar),
		tok(token.TokIdent, "b"),
		tok(token.TokPlus),
		tok(token.TokIdent, "c"),
		tok(token.TokEOF),
	})
}

func TestDivision(t *testing.T) {
	checkTokens(t, "a / b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokSlash),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestModulo(t *testing.T) {
	checkTokens(t, "a % b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPercent),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestArrow(t *testing.T) {
	checkTokens(t, "a -> b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokArrow, "->"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

// --- Newline insertion tests ---

func TestNewlineBetweenIdents(t *testing.T) {
	checkTokens(t, "sphere\nbox", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokNewline),
		tok(token.TokIdent, "box"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterPipe(t *testing.T) {
	checkTokens(t, "sphere |\nbox", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokPipe),
		tok(token.TokIdent, "box"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedBeforeDot(t *testing.T) {
	checkTokens(t, "sphere\n  .at(1,0,0)", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokDot),
		tok(token.TokIdent, "at"),
		tok(token.TokLParen),
		tok(token.TokInt, "1"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestCollapsedNewlines(t *testing.T) {
	checkTokens(t, "sphere\n\n\nbox", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokNewline),
		tok(token.TokIdent, "box"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterComma(t *testing.T) {
	checkTokens(t, "f(x,\ny)", []tokInfo{
		tok(token.TokIdent, "f"),
		tok(token.TokLParen),
		tok(token.TokIdent, "x"),
		tok(token.TokComma),
		tok(token.TokIdent, "y"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterLParen(t *testing.T) {
	checkTokens(t, "f(\nx)", []tokInfo{
		tok(token.TokIdent, "f"),
		tok(token.TokLParen),
		tok(token.TokIdent, "x"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterAssign(t *testing.T) {
	checkTokens(t, "x =\n1", []tokInfo{
		tok(token.TokIdent, "x"),
		tok(token.TokAssign),
		tok(token.TokInt, "1"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterPlus(t *testing.T) {
	checkTokens(t, "a +\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPlus),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterMinus(t *testing.T) {
	checkTokens(t, "a -\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinus),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterRParen(t *testing.T) {
	checkTokens(t, "f(x)\ng(y)", []tokInfo{
		tok(token.TokIdent, "f"),
		tok(token.TokLParen),
		tok(token.TokIdent, "x"),
		tok(token.TokRParen),
		tok(token.TokNewline),
		tok(token.TokIdent, "g"),
		tok(token.TokLParen),
		tok(token.TokIdent, "y"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterRBrace(t *testing.T) {
	checkTokens(t, "{ a }\nb", []tokInfo{
		tok(token.TokLBrace),
		tok(token.TokIdent, "a"),
		tok(token.TokRBrace),
		tok(token.TokNewline),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterRBrack(t *testing.T) {
	checkTokens(t, "[1]\nb", []tokInfo{
		tok(token.TokLBrack),
		tok(token.TokInt, "1"),
		tok(token.TokRBrack),
		tok(token.TokNewline),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterInt(t *testing.T) {
	checkTokens(t, "42\n43", []tokInfo{
		tok(token.TokInt, "42"),
		tok(token.TokNewline),
		tok(token.TokInt, "43"),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterFloat(t *testing.T) {
	checkTokens(t, "3.14\n2.71", []tokInfo{
		tok(token.TokFloat, "3.14"),
		tok(token.TokNewline),
		tok(token.TokFloat, "2.71"),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterHexColor(t *testing.T) {
	checkTokens(t, "#ff0000\n#00ff00", []tokInfo{
		tok(token.TokHexColor, "#ff0000"),
		tok(token.TokNewline),
		tok(token.TokHexColor, "#00ff00"),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterTrue(t *testing.T) {
	checkTokens(t, "true\nfalse", []tokInfo{
		tok(token.TokTrue),
		tok(token.TokNewline),
		tok(token.TokFalse),
		tok(token.TokEOF),
	})
}

func TestNewlineAfterString(t *testing.T) {
	checkTokens(t, "\"hello\"\n\"world\"", []tokInfo{
		tok(token.TokString, "\"hello\""),
		tok(token.TokNewline),
		tok(token.TokString, "\"world\""),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterLBrace(t *testing.T) {
	checkTokens(t, "{\nsphere\n}", []tokInfo{
		tok(token.TokLBrace),
		tok(token.TokIdent, "sphere"),
		tok(token.TokNewline),
		tok(token.TokRBrace),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterSmoothOps(t *testing.T) {
	checkTokens(t, "a |~\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipeSmooth),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a -~\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinusSmooth),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a &~\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokAmpSmooth),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterChamferOps(t *testing.T) {
	checkTokens(t, "a |/\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipeChamfer),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a -/\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinusChamfer),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a &/\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokAmpChamfer),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterDot(t *testing.T) {
	checkTokens(t, "a.\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokDot),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterColon(t *testing.T) {
	checkTokens(t, "x:\n1", []tokInfo{
		tok(token.TokIdent, "x"),
		tok(token.TokColon),
		tok(token.TokInt, "1"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterComparisons(t *testing.T) {
	for _, op := range []string{"==", "!=", "<", ">", "<=", ">="} {
		t.Run(op, func(t *testing.T) {
			src := fmt.Sprintf("a %s\nb", op)
			tokens, _ := Lex("test.chisel", src)
			// Should have: Ident, Op, Ident, EOF (no newline)
			found := false
			for _, tok := range tokens {
				if tok.Kind == token.TokNewline {
					found = true
				}
			}
			if found {
				t.Errorf("expected no newline after %q continuation", op)
			}
		})
	}
}

// --- Punctuation tests ---

func TestVectorLiteral(t *testing.T) {
	checkTokens(t, "[1, 2, 3]", []tokInfo{
		tok(token.TokLBrack),
		tok(token.TokInt, "1"),
		tok(token.TokComma),
		tok(token.TokInt, "2"),
		tok(token.TokComma),
		tok(token.TokInt, "3"),
		tok(token.TokRBrack),
		tok(token.TokEOF),
	})
}

func TestFunctionCallWithNamedArg(t *testing.T) {
	checkTokens(t, "f(x, y: 2)", []tokInfo{
		tok(token.TokIdent, "f"),
		tok(token.TokLParen),
		tok(token.TokIdent, "x"),
		tok(token.TokComma),
		tok(token.TokIdent, "y"),
		tok(token.TokColon),
		tok(token.TokInt, "2"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestDotDot(t *testing.T) {
	checkTokens(t, "0..8", []tokInfo{
		tok(token.TokInt, "0"),
		tok(token.TokDotDot),
		tok(token.TokInt, "8"),
		tok(token.TokEOF),
	})
}

func TestComparisons(t *testing.T) {
	checkTokens(t, "a == b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokEq),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a != b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokNeq),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a <= b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokLte),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
	checkTokens(t, "a >= b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokGte),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestBang(t *testing.T) {
	checkTokens(t, "!a", []tokInfo{
		tok(token.TokBang),
		tok(token.TokIdent, "a"),
		tok(token.TokEOF),
	})
}

// --- Error handling tests ---

func TestUnterminatedString(t *testing.T) {
	tokens, diags := Lex("test.chisel", `"unterminated`)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "unterminated string") {
		t.Errorf("expected unterminated string diagnostic, got: %s", diags[0].Message)
	}
	// Should still produce tokens (the partial string + EOF).
	found := false
	for _, tok := range tokens {
		if tok.Kind == token.TokString {
			found = true
		}
	}
	if !found {
		t.Error("expected a String token even for unterminated string")
	}
}

func TestUnterminatedBlockComment(t *testing.T) {
	tokens, diags := Lex("test.chisel", "/* unterminated")
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "unterminated block comment") {
		t.Errorf("expected unterminated block comment diagnostic, got: %s", diags[0].Message)
	}
	// Should still produce a comment token.
	found := false
	for _, tok := range tokens {
		if tok.Kind == token.TokComment {
			found = true
		}
	}
	if !found {
		t.Error("expected a Comment token even for unterminated block comment")
	}
}

func TestInvalidHexColor(t *testing.T) {
	tokens, diags := Lex("test.chisel", "#xyz")
	// '#' followed by non-hex chars: 'x' is not hex, so 0 hex digits.
	// Actually x is a valid hex digit! Let's use a truly invalid one.
	_ = tokens
	_ = diags

	tokens2, diags2 := Lex("test.chisel", "#ab")
	if len(diags2) != 1 {
		t.Fatalf("expected 1 diagnostic for #ab, got %d", len(diags2))
	}
	if !strings.Contains(diags2[0].Message, "invalid hex color") {
		t.Errorf("expected invalid hex color diagnostic, got: %s", diags2[0].Message)
	}
	found := false
	for _, tok := range tokens2 {
		if tok.Kind == token.TokHexColor {
			found = true
		}
	}
	if !found {
		t.Error("expected a HexColor token even for invalid hex color")
	}
}

func TestInvalidHexColorBadLength(t *testing.T) {
	// #xyz has x as hex but only 1 non-hex char... actually x IS a hex digit (0-9, a-f).
	// So #xyz: x=hex, y=not hex. So 1 hex digit.
	_, diags := Lex("test.chisel", "#xy")
	// x is hex, y is not hex -> 1 hex digit -> invalid
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for #xy, got %d", len(diags))
	}
}

func TestUnexpectedCharacter(t *testing.T) {
	_, diags := Lex("test.chisel", "@")
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if !strings.Contains(diags[0].Message, "unexpected character") {
		t.Errorf("expected unexpected character diagnostic, got: %s", diags[0].Message)
	}
}

// --- Position tracking tests ---

func TestPositionTracking(t *testing.T) {
	tokens, _ := Lex("test.chisel", "sphere box")
	if len(tokens) < 2 {
		t.Fatal("expected at least 2 tokens")
	}

	// "sphere" starts at line 1, col 1
	if tokens[0].Pos.Line != 1 || tokens[0].Pos.Col != 1 {
		t.Errorf("sphere pos = %d:%d, want 1:1", tokens[0].Pos.Line, tokens[0].Pos.Col)
	}
	if tokens[0].Pos.Offset != 0 {
		t.Errorf("sphere offset = %d, want 0", tokens[0].Pos.Offset)
	}
	if tokens[0].Len != 6 {
		t.Errorf("sphere len = %d, want 6", tokens[0].Len)
	}

	// "box" starts at line 1, col 8 (after "sphere ")
	if tokens[1].Pos.Line != 1 || tokens[1].Pos.Col != 8 {
		t.Errorf("box pos = %d:%d, want 1:8", tokens[1].Pos.Line, tokens[1].Pos.Col)
	}
}

func TestPositionTrackingMultiline(t *testing.T) {
	tokens, _ := Lex("test.chisel", "a\nb")
	// a: line 1, col 1
	// newline: between
	// b: line 2, col 1
	bTok := token.Token{}
	for _, tok := range tokens {
		if tok.Kind == token.TokIdent && tok.Value == "b" {
			bTok = tok
			break
		}
	}
	if bTok.Pos.Line != 2 || bTok.Pos.Col != 1 {
		t.Errorf("b pos = %d:%d, want 2:1", bTok.Pos.Line, bTok.Pos.Col)
	}
}

func TestPositionTrackingBlockComment(t *testing.T) {
	tokens, _ := Lex("test.chisel", "/* line1\nline2 */x")
	// x should be on line 2
	xTok := token.Token{}
	for _, tok := range tokens {
		if tok.Kind == token.TokIdent && tok.Value == "x" {
			xTok = tok
			break
		}
	}
	if xTok.Pos.Line != 2 {
		t.Errorf("x pos line = %d, want 2", xTok.Pos.Line)
	}
}

// --- Complex expression tests ---

func TestForLoopTokens(t *testing.T) {
	checkTokens(t, "for i in 0..8", []tokInfo{
		tok(token.TokFor),
		tok(token.TokIdent, "i"),
		tok(token.TokIn),
		tok(token.TokInt, "0"),
		tok(token.TokDotDot),
		tok(token.TokInt, "8"),
		tok(token.TokEOF),
	})
}

func TestMethodChain(t *testing.T) {
	checkTokens(t, "sphere.at(1,0,0).red", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokDot),
		tok(token.TokIdent, "at"),
		tok(token.TokLParen),
		tok(token.TokInt, "1"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokRParen),
		tok(token.TokDot),
		tok(token.TokIdent, "red"),
		tok(token.TokEOF),
	})
}

func TestAssignment(t *testing.T) {
	checkTokens(t, "r = 1.5", []tokInfo{
		tok(token.TokIdent, "r"),
		tok(token.TokAssign),
		tok(token.TokFloat, "1.5"),
		tok(token.TokEOF),
	})
}

func TestSmoothUnionWithBlendRadius(t *testing.T) {
	checkTokens(t, "a |~0.3 b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipeSmooth),
		tok(token.TokFloat, "0.3"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestCameraOneliner(t *testing.T) {
	checkTokens(t, "camera [0,2,5] -> [0,0,0]", []tokInfo{
		tok(token.TokCamera),
		tok(token.TokLBrack),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "2"),
		tok(token.TokComma),
		tok(token.TokInt, "5"),
		tok(token.TokRBrack),
		tok(token.TokArrow),
		tok(token.TokLBrack),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokRBrack),
		tok(token.TokEOF),
	})
}

func TestAllKeywords(t *testing.T) {
	for kw, kind := range keywords {
		t.Run(kw, func(t *testing.T) {
			tokens, _ := Lex("test.chisel", kw)
			if len(tokens) < 1 {
				t.Fatal("expected at least 1 token")
			}
			if tokens[0].Kind != kind {
				t.Errorf("keyword %q: got kind %s, want %s", kw, tokens[0].Kind, kind)
			}
			if tokens[0].Value != kw {
				t.Errorf("keyword %q: got value %q", kw, tokens[0].Value)
			}
		})
	}
}

func TestIdentifiersNotKeywords(t *testing.T) {
	// Identifiers that are prefixes of keywords or contain keyword substrings.
	ids := []string{"forEach", "inform", "iffy", "format", "stepping", "mat_gold", "_for"}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			tokens, _ := Lex("test.chisel", id)
			if tokens[0].Kind != token.TokIdent {
				t.Errorf("%q: got kind %s, want Ident", id, tokens[0].Kind)
			}
		})
	}
}

func TestMultilineMethodChain(t *testing.T) {
	// Method chains across lines: newline before dot should be suppressed.
	src := "sphere\n  .at(1, 0, 0)\n  .scale(2)"
	checkTokens(t, src, []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokDot),
		tok(token.TokIdent, "at"),
		tok(token.TokLParen),
		tok(token.TokInt, "1"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokRParen),
		tok(token.TokDot),
		tok(token.TokIdent, "scale"),
		tok(token.TokLParen),
		tok(token.TokInt, "2"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestWhitespaceOnly(t *testing.T) {
	checkTokens(t, "   \t\t  ", []tokInfo{
		tok(token.TokEOF),
	})
}

func TestNewlinesOnly(t *testing.T) {
	checkTokens(t, "\n\n\n", []tokInfo{
		tok(token.TokEOF),
	})
}

func TestCarriageReturnLineFeed(t *testing.T) {
	checkTokens(t, "a\r\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokNewline),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestCommentInMiddle(t *testing.T) {
	checkTokens(t, "a /* mid */ b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokComment),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestInlineCommentAfterExpr(t *testing.T) {
	checkTokens(t, "sphere // inline comment\nbox", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokComment),
		tok(token.TokNewline),
		tok(token.TokIdent, "box"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterDotDot(t *testing.T) {
	checkTokens(t, "0..\n8", []tokInfo{
		tok(token.TokInt, "0"),
		tok(token.TokDotDot),
		tok(token.TokInt, "8"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterArrow(t *testing.T) {
	checkTokens(t, "a ->\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokArrow),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterStar(t *testing.T) {
	checkTokens(t, "a *\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokStar),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterSlash(t *testing.T) {
	checkTokens(t, "a /\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokSlash),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterPercent(t *testing.T) {
	checkTokens(t, "a %\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPercent),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterLBrack(t *testing.T) {
	checkTokens(t, "[\n1]", []tokInfo{
		tok(token.TokLBrack),
		tok(token.TokInt, "1"),
		tok(token.TokRBrack),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterBang(t *testing.T) {
	checkTokens(t, "!\na", []tokInfo{
		tok(token.TokBang),
		tok(token.TokIdent, "a"),
		tok(token.TokEOF),
	})
}

func TestEscapedStringQuote(t *testing.T) {
	checkTokens(t, `"he\"llo"`, []tokInfo{
		tok(token.TokString, `"he\"llo"`),
		tok(token.TokEOF),
	})
}

func TestMultipleErrors(t *testing.T) {
	_, diags := Lex("test.chisel", "@$")
	if len(diags) != 2 {
		t.Errorf("expected 2 diagnostics for @$, got %d", len(diags))
		for _, d := range diags {
			t.Logf("  diag: %s", d.Error())
		}
	}
}

func TestSmoothSubtractWithRadius(t *testing.T) {
	checkTokens(t, "a -~0.2 b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinusSmooth),
		tok(token.TokFloat, "0.2"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestChamferSubtractWithRadius(t *testing.T) {
	checkTokens(t, "a -/0.3 b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokMinusChamfer),
		tok(token.TokFloat, "0.3"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestComplexExpression(t *testing.T) {
	// sphere(2) - cylinder(0.5, 6).orient(x)
	checkTokens(t, "sphere(2) - cylinder(0.5, 6).orient(x)", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokLParen),
		tok(token.TokInt, "2"),
		tok(token.TokRParen),
		tok(token.TokMinus),
		tok(token.TokIdent, "cylinder"),
		tok(token.TokLParen),
		tok(token.TokFloat, "0.5"),
		tok(token.TokComma),
		tok(token.TokInt, "6"),
		tok(token.TokRParen),
		tok(token.TokDot),
		tok(token.TokIdent, "orient"),
		tok(token.TokLParen),
		tok(token.TokIdent, "x"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestIfElse(t *testing.T) {
	checkTokens(t, "if x > 1 { a } else { b }", []tokInfo{
		tok(token.TokIf),
		tok(token.TokIdent, "x"),
		tok(token.TokGt),
		tok(token.TokInt, "1"),
		tok(token.TokLBrace),
		tok(token.TokIdent, "a"),
		tok(token.TokRBrace),
		tok(token.TokElse),
		tok(token.TokLBrace),
		tok(token.TokIdent, "b"),
		tok(token.TokRBrace),
		tok(token.TokEOF),
	})
}

func TestForInStep(t *testing.T) {
	checkTokens(t, "for i in 0..1 step 0.1", []tokInfo{
		tok(token.TokFor),
		tok(token.TokIdent, "i"),
		tok(token.TokIn),
		tok(token.TokInt, "0"),
		tok(token.TokDotDot),
		tok(token.TokInt, "1"),
		tok(token.TokStep),
		tok(token.TokFloat, "0.1"),
		tok(token.TokEOF),
	})
}

func TestBgHexColor(t *testing.T) {
	checkTokens(t, "bg #1a1a2e", []tokInfo{
		tok(token.TokBg),
		tok(token.TokHexColor, "#1a1a2e"),
		tok(token.TokEOF),
	})
}

func TestDebugMode(t *testing.T) {
	checkTokens(t, "debug normals", []tokInfo{
		tok(token.TokDebug),
		tok(token.TokIdent, "normals"),
		tok(token.TokEOF),
	})
}

func TestMatDefinition(t *testing.T) {
	checkTokens(t, "mat gold = { color: [1, 0, 0] }", []tokInfo{
		tok(token.TokMat),
		tok(token.TokIdent, "gold"),
		tok(token.TokAssign),
		tok(token.TokLBrace),
		tok(token.TokIdent, "color"),
		tok(token.TokColon),
		tok(token.TokLBrack),
		tok(token.TokInt, "1"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokComma),
		tok(token.TokInt, "0"),
		tok(token.TokRBrack),
		tok(token.TokRBrace),
		tok(token.TokEOF),
	})
}

func TestGlslKeyword(t *testing.T) {
	checkTokens(t, "glsl(p)", []tokInfo{
		tok(token.TokGlsl),
		tok(token.TokLParen),
		tok(token.TokIdent, "p"),
		tok(token.TokRParen),
		tok(token.TokEOF),
	})
}

func TestRaymarch(t *testing.T) {
	checkTokens(t, "raymarch { steps: 128 }", []tokInfo{
		tok(token.TokRaymarch),
		tok(token.TokLBrace),
		tok(token.TokIdent, "steps"),
		tok(token.TokColon),
		tok(token.TokInt, "128"),
		tok(token.TokRBrace),
		tok(token.TokEOF),
	})
}

func TestNeverPanics(t *testing.T) {
	// Feed various nasty inputs and ensure we never panic.
	inputs := []string{
		"",
		"\x00",
		"#",
		"##",
		"#g",
		"\"",
		"'",
		"/*",
		"/",
		".",
		"..",
		"...",
		"->",
		"--",
		"---",
		"|||",
		"&&&",
		"===",
		"!!=",
		"\n\n\n\n",
		"\r\n\r\n",
		"@#$%^",
		string([]byte{0xFF, 0xFE}),
	}
	for _, input := range inputs {
		t.Run(fmt.Sprintf("%q", input), func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Lex panicked on input %q: %v", input, r)
				}
			}()
			Lex("test.chisel", input)
		})
	}
}

func TestScientificNotationVariants(t *testing.T) {
	cases := []struct {
		input string
		kind  token.TokenKind
		value string
	}{
		{"1e5", token.TokFloat, "1e5"},
		{"1E5", token.TokFloat, "1E5"},
		{"1e+5", token.TokFloat, "1e+5"},
		{"1e-5", token.TokFloat, "1e-5"},
		{"2.5e10", token.TokFloat, "2.5e10"},
		{"3.14e-2", token.TokFloat, "3.14e-2"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			tokens, _ := Lex("test.chisel", tc.input)
			if tokens[0].Kind != tc.kind {
				t.Errorf("kind = %s, want %s", tokens[0].Kind, tc.kind)
			}
			if tokens[0].Value != tc.value {
				t.Errorf("value = %q, want %q", tokens[0].Value, tc.value)
			}
		})
	}
}

func TestDotFollowedByDigitIsFloat(t *testing.T) {
	// "0.5" should be Float, not Int Dot Int
	checkTokens(t, "0.5", []tokInfo{
		tok(token.TokFloat, "0.5"),
		tok(token.TokEOF),
	})
}

func TestDotDotNotFloat(t *testing.T) {
	// "0..8" should be Int DotDot Int, not a float
	checkTokens(t, "0..8", []tokInfo{
		tok(token.TokInt, "0"),
		tok(token.TokDotDot),
		tok(token.TokInt, "8"),
		tok(token.TokEOF),
	})
}

func TestDotAfterIntNotFloat(t *testing.T) {
	// "0.at" should be Int Dot Ident, not float (since .a is not a digit)
	checkTokens(t, "0.at", []tokInfo{
		tok(token.TokInt, "0"),
		tok(token.TokDot),
		tok(token.TokIdent, "at"),
		tok(token.TokEOF),
	})
}

func TestCommentDoesNotBreakNewlineLogic(t *testing.T) {
	// Comment between identifier and newline: the newline should still
	// be inserted after the ident.
	checkTokens(t, "sphere // comment\nbox", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokComment),
		tok(token.TokNewline),
		tok(token.TokIdent, "box"),
		tok(token.TokEOF),
	})
}

func TestHashOnlyProducesDiag(t *testing.T) {
	_, diags := Lex("test.chisel", "#")
	if len(diags) != 1 {
		t.Errorf("expected 1 diagnostic for bare #, got %d", len(diags))
	}
}

func TestHexColorUppercase(t *testing.T) {
	checkTokens(t, "#FF00AA", []tokInfo{
		tok(token.TokHexColor, "#FF00AA"),
		tok(token.TokEOF),
	})
}

func TestHexColorMixedCase(t *testing.T) {
	checkTokens(t, "#fF0AaB", []tokInfo{
		tok(token.TokHexColor, "#fF0AaB"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterPipeSmooth(t *testing.T) {
	// pipe smooth + newline + number (blend radius) + ident
	checkTokens(t, "a |~\n0.3 b", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipeSmooth),
		tok(token.TokFloat, "0.3"),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestMultilineProgramNewlines(t *testing.T) {
	src := "r = 1.5\nsphere(r)\nbox"
	checkTokens(t, src, []tokInfo{
		tok(token.TokIdent, "r"),
		tok(token.TokAssign),
		tok(token.TokFloat, "1.5"),
		tok(token.TokNewline),
		tok(token.TokIdent, "sphere"),
		tok(token.TokLParen),
		tok(token.TokIdent, "r"),
		tok(token.TokRParen),
		tok(token.TokNewline),
		tok(token.TokIdent, "box"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterAmp(t *testing.T) {
	checkTokens(t, "a &\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokAmp),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNewlineSuppressedAfterPipeContinuation(t *testing.T) {
	checkTokens(t, "a |\nb", []tokInfo{
		tok(token.TokIdent, "a"),
		tok(token.TokPipe),
		tok(token.TokIdent, "b"),
		tok(token.TokEOF),
	})
}

func TestNoLeadingNewline(t *testing.T) {
	checkTokens(t, "\n\nsphere", []tokInfo{
		tok(token.TokIdent, "sphere"),
		tok(token.TokEOF),
	})
}

func TestFilePosition(t *testing.T) {
	tokens, _ := Lex("scene.chisel", "x")
	if tokens[0].Pos.File != "scene.chisel" {
		t.Errorf("expected file name 'scene.chisel', got %q", tokens[0].Pos.File)
	}
}
