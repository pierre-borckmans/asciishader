package format

import (
	"testing"
)

func TestFormatUnionNoSpaces(t *testing.T) {
	got, err := Format("sphere|box")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere | box\n"
	if got != want {
		t.Errorf("Format(\"sphere|box\")\ngot:  %q\nwant: %q", got, want)
	}
}

func TestFormatMethodCallSpaces(t *testing.T) {
	got, err := Format("sphere.at(2,0,0)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere.at(2, 0, 0)\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatSmoothOperator(t *testing.T) {
	got, err := Format("sphere|~0.3 box")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere |~0.3 box\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatIdempotent(t *testing.T) {
	input := "sphere|box"
	first, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error on first format: %v", err)
	}
	second, err := Format(first)
	if err != nil {
		t.Fatalf("unexpected error on second format: %v", err)
	}
	if first != second {
		t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestFormatImplicitUnion(t *testing.T) {
	// Implicit union: two shapes on separate lines become | union in AST.
	got, err := Format("sphere\nbox.at(2,0,0)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere | box.at(2, 0, 0)\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatTrailingNewline(t *testing.T) {
	got, err := Format("sphere")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatAssignment(t *testing.T) {
	got, err := Format("r = 1.5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "r = 1.5\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatFunctionDef(t *testing.T) {
	got, err := Format("f(x) = sphere(x)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "f(x) = sphere(x)\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatVecLit(t *testing.T) {
	got, err := Format("[1,2,3]")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "[1, 2, 3]\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatUnaryNeg(t *testing.T) {
	got, err := Format("-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "-1\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatIdempotentComplex(t *testing.T) {
	input := "sphere.at(2, 0, 0).red | box.blue"
	first, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := Format(first)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first != second {
		t.Errorf("not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestFormatParseError(t *testing.T) {
	_, err := Format("((((")
	if err == nil {
		t.Errorf("expected error for unparseable input")
	}
}

func TestFormatNamedArgs(t *testing.T) {
	got, err := Format("fbm(p,octaves: 6)")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "fbm(p, octaves: 6)\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatBareMethod(t *testing.T) {
	got, err := Format("sphere.red")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "sphere.red\n"
	if got != want {
		t.Errorf("got:  %q\nwant: %q", got, want)
	}
}

func TestFormatPreservesComments(t *testing.T) {
	input := "// My scene\nsphere\n"
	got, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("comments not preserved:\ngot:  %q\nwant: %q", got, input)
	}
}

func TestFormatPreservesBlankLines(t *testing.T) {
	input := "r = 1\n\nsphere(r)\n"
	got, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("blank lines not preserved:\ngot:  %q\nwant: %q", got, input)
	}
}

func TestFormatSettingsBlock(t *testing.T) {
	input := "raymarch { steps: 200, precision: 0.001 }"
	got, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Settings should not contain Go map internals.
	if contains(got, "map[") || contains(got, "0x") {
		t.Errorf("settings block has Go internals:\n%s", got)
	}
	// Should contain the keys.
	if !contains(got, "steps") || !contains(got, "precision") {
		t.Errorf("settings block missing keys:\n%s", got)
	}
}

func TestFormatGlslMultiLine(t *testing.T) {
	input := "glsl(p) {\n  float d = length(p) - 1.0;\n  return d;\n}"
	got, err := Format(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should preserve multi-line structure.
	lines := countLines(got)
	if lines < 3 {
		t.Errorf("GLSL should be multi-line, got %d lines:\n%s", lines, got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func countLines(s string) int {
	n := 1
	for _, ch := range s {
		if ch == '\n' {
			n++
		}
	}
	return n
}
