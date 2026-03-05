package diagnostic

import (
	"strings"
	"testing"

	"asciishader/pkg/chisel/token"
)

// helper to build a span quickly.
func span(file string, line, col, endLine, endCol int) token.Span {
	return token.Span{
		Start: token.Position{File: file, Line: line, Col: col},
		End:   token.Position{File: file, Line: endLine, Col: endCol},
	}
}

func TestRenderSingleCharError(t *testing.T) {
	source := "x = 1\ny = 2\nsphere + box"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unexpected token '+'",
		Span:     span("scene.chisel", 3, 8, 3, 9),
	}

	got := Render(source, diag, false)
	want := "" +
		"error: unexpected token '+'\n" +
		"  ┌─ scene.chisel:3:8\n" +
		"  │\n" +
		"3 │ sphere + box\n" +
		"  │        ^ unexpected token '+'\n" +
		"  │\n"

	if got != want {
		t.Errorf("TestRenderSingleCharError\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderMultiCharSpan(t *testing.T) {
	source := "sphree"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unknown shape 'sphree'",
		Span:     span("scene.chisel", 1, 1, 1, 7),
	}

	got := Render(source, diag, false)
	want := "" +
		"error: unknown shape 'sphree'\n" +
		"  ┌─ scene.chisel:1:1\n" +
		"  │\n" +
		"1 │ sphree\n" +
		"  │ ^^^^^^ unknown shape 'sphree'\n" +
		"  │\n"

	if got != want {
		t.Errorf("TestRenderMultiCharSpan\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderWithHelp(t *testing.T) {
	source := "x = 1\ny = 2\nsphere + box"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unexpected token '+'",
		Span:     span("scene.chisel", 3, 8, 3, 9),
		Help:     "'+' is arithmetic only. Use '|' for combining shapes.",
	}

	got := Render(source, diag, false)
	want := "" +
		"error: unexpected token '+'\n" +
		"  ┌─ scene.chisel:3:8\n" +
		"  │\n" +
		"3 │ sphere + box\n" +
		"  │        ^ unexpected token '+'\n" +
		"  │\n" +
		"  = help: '+' is arithmetic only. Use '|' for combining shapes.\n"

	if got != want {
		t.Errorf("TestRenderWithHelp\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderWithAdditionalLabel(t *testing.T) {
	source := "x = 1\ny = 2\nsphere | circle"
	diag := Diagnostic{
		Severity: Error,
		Message:  "type mismatch in union",
		Span:     span("test.chisel", 3, 1, 3, 16),
		Help:     "both sides of '|' must be the same SDF type",
		Labels: []Label{
			{
				Span:    span("test.chisel", 3, 10, 3, 16),
				Message: "this is sdf2d",
			},
		},
	}

	got := Render(source, diag, false)
	want := "" +
		"error: type mismatch in union\n" +
		"  ┌─ test.chisel:3:1\n" +
		"  │\n" +
		"3 │ sphere | circle\n" +
		"  │ ^^^^^^^^^^^^^^^ type mismatch in union\n" +
		"  │\n" +
		"3 │ sphere | circle\n" +
		"  │          ------ this is sdf2d\n" +
		"  │\n" +
		"  = help: both sides of '|' must be the same SDF type\n"

	if got != want {
		t.Errorf("TestRenderWithAdditionalLabel\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderWarning(t *testing.T) {
	source := "x = 42\nsphere"
	diag := Diagnostic{
		Severity: Warning,
		Message:  "unused variable 'x'",
		Span:     span("scene.chisel", 1, 1, 1, 2),
	}

	got := Render(source, diag, false)
	want := "" +
		"warning: unused variable 'x'\n" +
		"  ┌─ scene.chisel:1:1\n" +
		"  │\n" +
		"1 │ x = 42\n" +
		"  │ ^ unused variable 'x'\n" +
		"  │\n"

	if got != want {
		t.Errorf("TestRenderWarning\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderErrorOnLine1(t *testing.T) {
	source := "!!!"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unexpected character '!'",
		Span:     span("scene.chisel", 1, 1, 1, 2),
	}

	got := Render(source, diag, false)
	want := "" +
		"error: unexpected character '!'\n" +
		"  ┌─ scene.chisel:1:1\n" +
		"  │\n" +
		"1 │ !!!\n" +
		"  │ ^ unexpected character '!'\n" +
		"  │\n"

	if got != want {
		t.Errorf("TestRenderErrorOnLine1\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderErrorAtEndOfLine(t *testing.T) {
	source := "sphere("
	diag := Diagnostic{
		Severity: Error,
		Message:  "expected ')'",
		Span:     span("scene.chisel", 1, 8, 1, 9),
	}

	got := Render(source, diag, false)
	want := "" +
		"error: expected ')'\n" +
		"  ┌─ scene.chisel:1:8\n" +
		"  │\n" +
		"1 │ sphere(\n" +
		"  │        ^ expected ')'\n" +
		"  │\n"

	if got != want {
		t.Errorf("TestRenderErrorAtEndOfLine\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderMultiLineSource(t *testing.T) {
	source := "camera [0,2,5] -> [0,0,0]\n\nsphere\n  .at(2, 0, 0)\nbox"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unknown method 'att'",
		Span:     span("scene.chisel", 4, 4, 4, 7),
		Help:     "did you mean 'at'?",
	}

	got := Render(source, diag, false)
	want := "" +
		"error: unknown method 'att'\n" +
		"  ┌─ scene.chisel:4:4\n" +
		"  │\n" +
		"4 │   .at(2, 0, 0)\n" +
		"  │    ^^^ unknown method 'att'\n" +
		"  │\n" +
		"  = help: did you mean 'at'?\n"

	if got != want {
		t.Errorf("TestRenderMultiLineSource\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestRenderNoColor(t *testing.T) {
	source := "sphere + box"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unexpected token '+'",
		Span:     span("scene.chisel", 1, 8, 1, 9),
	}

	got := Render(source, diag, false)
	// Verify no ANSI escape sequences are present.
	if strings.Contains(got, "\033[") {
		t.Errorf("TestRenderNoColor: output contains ANSI escapes:\n%s", got)
	}
}

func TestRenderWithColor(t *testing.T) {
	source := "sphere + box"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unexpected token '+'",
		Span:     span("scene.chisel", 1, 8, 1, 9),
	}

	got := Render(source, diag, true)

	// Must contain ANSI escapes.
	if !strings.Contains(got, "\033[") {
		t.Errorf("TestRenderWithColor: output has no ANSI escapes:\n%s", got)
	}

	// Error severity must be red.
	if !strings.Contains(got, "\033[31merror\033[0m") {
		t.Errorf("TestRenderWithColor: expected red 'error' text:\n%s", got)
	}

	// Line number must be blue.
	if !strings.Contains(got, "\033[34m1\033[0m") {
		t.Errorf("TestRenderWithColor: expected blue line number:\n%s", got)
	}

	// Caret must be red (error color).
	if !strings.Contains(got, "\033[31m^\033[0m") {
		t.Errorf("TestRenderWithColor: expected red caret:\n%s", got)
	}
}

func TestRenderWarningColor(t *testing.T) {
	source := "x = 42\nsphere"
	diag := Diagnostic{
		Severity: Warning,
		Message:  "unused variable 'x'",
		Span:     span("scene.chisel", 1, 1, 1, 2),
	}

	got := Render(source, diag, true)

	// Warning severity must be yellow.
	if !strings.Contains(got, "\033[33mwarning\033[0m") {
		t.Errorf("TestRenderWarningColor: expected yellow 'warning' text:\n%s", got)
	}
}

func TestRenderHintColor(t *testing.T) {
	source := "sphere"
	diag := Diagnostic{
		Severity: Hint,
		Message:  "consider using a variable",
		Span:     span("scene.chisel", 1, 1, 1, 7),
	}

	got := Render(source, diag, true)

	// Hint severity must be blue.
	if !strings.Contains(got, "\033[34mhint\033[0m") {
		t.Errorf("TestRenderHintColor: expected blue 'hint' text:\n%s", got)
	}
}

func TestRenderMultiLineSpan(t *testing.T) {
	source := "for i in 0..10 {\n  sphere.at(i, 0, 0)\n  box\n}"
	diag := Diagnostic{
		Severity: Error,
		Message:  "loop body error",
		Span:     span("scene.chisel", 1, 1, 4, 2),
	}

	got := Render(source, diag, false)

	// Should show the first line.
	if !strings.Contains(got, "1 │ for i in 0..10 {") {
		t.Errorf("TestRenderMultiLineSpan: expected first line shown:\n%s", got)
	}
	// Should show ellipsis for skipped lines.
	if !strings.Contains(got, "...") {
		t.Errorf("TestRenderMultiLineSpan: expected '...' for skipped lines:\n%s", got)
	}
	// Should show the last line.
	if !strings.Contains(got, "4 │ }") {
		t.Errorf("TestRenderMultiLineSpan: expected last line shown:\n%s", got)
	}
}

func TestRenderEndOfFile(t *testing.T) {
	source := "sphere"
	diag := Diagnostic{
		Severity: Error,
		Message:  "unexpected end of file",
		// Span points past the last line.
		Span: span("scene.chisel", 2, 1, 2, 2),
	}

	got := Render(source, diag, false)
	// Should still produce output without panicking.
	if !strings.Contains(got, "error: unexpected end of file") {
		t.Errorf("TestRenderEndOfFile: expected error header:\n%s", got)
	}
	if !strings.Contains(got, "scene.chisel:2:1") {
		t.Errorf("TestRenderEndOfFile: expected location:\n%s", got)
	}
}

func TestRenderNoFileInSpan(t *testing.T) {
	source := "sphere"
	diag := Diagnostic{
		Severity: Error,
		Message:  "test",
		Span:     span("", 1, 1, 1, 7),
	}

	got := Render(source, diag, false)
	// Location should show line:col without a filename.
	if !strings.Contains(got, "┌─ 1:1") {
		t.Errorf("TestRenderNoFileInSpan: expected plain position:\n%s", got)
	}
}

func TestRenderGutterWidthWithLargeLineNumber(t *testing.T) {
	// Build a source with 100 lines.
	var sb strings.Builder
	for i := 1; i <= 100; i++ {
		if i > 1 {
			sb.WriteByte('\n')
		}
		sb.WriteString("x = 1")
	}
	source := sb.String()

	diag := Diagnostic{
		Severity: Error,
		Message:  "error on line 100",
		Span:     span("big.chisel", 100, 1, 100, 6),
	}

	got := Render(source, diag, false)

	// Gutter should accommodate 3-digit line numbers.
	if !strings.Contains(got, "100 │ x = 1") {
		t.Errorf("TestRenderGutterWidthWithLargeLineNumber: expected padded gutter:\n%s", got)
	}
}
