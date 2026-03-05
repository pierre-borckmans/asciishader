package token

import (
	"testing"
)

func TestAllTokenKindsDistinct(t *testing.T) {
	// Every TokenKind constant from 0..tokenKindCount-1 must be distinct.
	// Since they are sequential iota values this is guaranteed by the
	// language, but we verify the count is sane and that String() never
	// returns the fallback for any valid kind.
	count := TokenKindCount()
	if count == 0 {
		t.Fatal("TokenKindCount() returned 0")
	}

	seen := make(map[string]TokenKind)
	for i := 0; i < count; i++ {
		k := TokenKind(i)
		name := k.String()
		if name == "" {
			t.Errorf("TokenKind(%d).String() returned empty string", i)
		}
		if prev, ok := seen[name]; ok {
			t.Errorf("TokenKind(%d) and TokenKind(%d) both have String() == %q", int(prev), i, name)
		}
		seen[name] = k
	}
}

func TestTokenKindStringKnown(t *testing.T) {
	tests := []struct {
		kind TokenKind
		want string
	}{
		{TokInt, "Int"},
		{TokFloat, "Float"},
		{TokHexColor, "HexColor"},
		{TokString, "String"},
		{TokIdent, "Ident"},
		{TokFor, "for"},
		{TokIn, "in"},
		{TokIf, "if"},
		{TokElse, "else"},
		{TokStep, "step"},
		{TokLight, "light"},
		{TokCamera, "camera"},
		{TokBg, "bg"},
		{TokRaymarch, "raymarch"},
		{TokPost, "post"},
		{TokMat, "mat"},
		{TokDebug, "debug"},
		{TokGlsl, "glsl"},
		{TokTrue, "true"},
		{TokFalse, "false"},
		{TokPipe, "|"},
		{TokPipeSmooth, "|~"},
		{TokPipeChamfer, "|/"},
		{TokMinus, "-"},
		{TokMinusSmooth, "-~"},
		{TokMinusChamfer, "-/"},
		{TokAmp, "&"},
		{TokAmpSmooth, "&~"},
		{TokAmpChamfer, "&/"},
		{TokPlus, "+"},
		{TokStar, "*"},
		{TokSlash, "/"},
		{TokPercent, "%"},
		{TokEq, "=="},
		{TokNeq, "!="},
		{TokLt, "<"},
		{TokGt, ">"},
		{TokLte, "<="},
		{TokGte, ">="},
		{TokLParen, "("},
		{TokRParen, ")"},
		{TokLBrack, "["},
		{TokRBrack, "]"},
		{TokLBrace, "{"},
		{TokRBrace, "}"},
		{TokDot, "."},
		{TokComma, ","},
		{TokColon, ":"},
		{TokAssign, "="},
		{TokArrow, "->"},
		{TokDotDot, ".."},
		{TokBang, "!"},
		{TokNewline, "Newline"},
		{TokComment, "Comment"},
		{TokEOF, "EOF"},
	}

	for _, tt := range tests {
		got := tt.kind.String()
		if got != tt.want {
			t.Errorf("TokenKind(%d).String() = %q, want %q", int(tt.kind), got, tt.want)
		}
	}
}

func TestTokenKindStringUnknown(t *testing.T) {
	k := TokenKind(9999)
	s := k.String()
	if s == "" {
		t.Error("unknown TokenKind.String() should not be empty")
	}
	want := "TokenKind(9999)"
	if s != want {
		t.Errorf("got %q, want %q", s, want)
	}
}

func TestTokenConstruction(t *testing.T) {
	tok := Token{
		Kind:  TokIdent,
		Value: "sphere",
		Pos: Position{
			File:   "test.chisel",
			Line:   1,
			Col:    1,
			Offset: 0,
		},
		Len: 6,
	}

	if tok.Kind != TokIdent {
		t.Errorf("Kind = %v, want TokIdent", tok.Kind)
	}
	if tok.Value != "sphere" {
		t.Errorf("Value = %q, want %q", tok.Value, "sphere")
	}
	if tok.Pos.Line != 1 || tok.Pos.Col != 1 {
		t.Errorf("Pos = %v, want line 1 col 1", tok.Pos)
	}
	if tok.Len != 6 {
		t.Errorf("Len = %d, want 6", tok.Len)
	}
}

func TestTokenSpan(t *testing.T) {
	tok := Token{
		Kind:  TokIdent,
		Value: "sphere",
		Pos: Position{
			File:   "test.chisel",
			Line:   3,
			Col:    5,
			Offset: 20,
		},
		Len: 6,
	}

	span := tok.TokenSpan()

	if span.Start.Line != 3 || span.Start.Col != 5 || span.Start.Offset != 20 {
		t.Errorf("span.Start = %v, want line=3 col=5 offset=20", span.Start)
	}
	if span.End.Col != 11 || span.End.Offset != 26 {
		t.Errorf("span.End = %v, want col=11 offset=26", span.End)
	}
	if span.Start.File != "test.chisel" {
		t.Errorf("span.Start.File = %q, want %q", span.Start.File, "test.chisel")
	}
}

func TestPositionString(t *testing.T) {
	p := Position{File: "scene.chisel", Line: 10, Col: 3, Offset: 100}
	got := p.String()
	want := "scene.chisel:10:3"
	if got != want {
		t.Errorf("Position.String() = %q, want %q", got, want)
	}

	p2 := Position{Line: 5, Col: 8}
	got2 := p2.String()
	want2 := "5:8"
	if got2 != want2 {
		t.Errorf("Position.String() without file = %q, want %q", got2, want2)
	}
}

func TestSpanString(t *testing.T) {
	s := Span{
		Start: Position{File: "test.chisel", Line: 1, Col: 1},
		End:   Position{File: "test.chisel", Line: 1, Col: 7},
	}
	got := s.String()
	want := "test.chisel:1:1..1:7"
	if got != want {
		t.Errorf("Span.String() = %q, want %q", got, want)
	}

	s2 := Span{
		Start: Position{Line: 2, Col: 3},
		End:   Position{Line: 2, Col: 10},
	}
	got2 := s2.String()
	want2 := "2:3..2:10"
	if got2 != want2 {
		t.Errorf("Span.String() without file = %q, want %q", got2, want2)
	}
}

func TestTokenString(t *testing.T) {
	tok := Token{Kind: TokIdent, Value: "sphere"}
	got := tok.String()
	want := `Ident("sphere")`
	if got != want {
		t.Errorf("Token.String() = %q, want %q", got, want)
	}

	eof := Token{Kind: TokEOF}
	if eof.String() != "EOF" {
		t.Errorf("EOF token String() = %q, want %q", eof.String(), "EOF")
	}

	nl := Token{Kind: TokNewline}
	if nl.String() != "Newline" {
		t.Errorf("Newline token String() = %q, want %q", nl.String(), "Newline")
	}
}
