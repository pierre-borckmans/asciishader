package diagnostic

import (
	"testing"

	"asciishader/pkg/chisel/compiler/token"
)

func TestSeverityString(t *testing.T) {
	tests := []struct {
		sev  Severity
		want string
	}{
		{Error, "error"},
		{Warning, "warning"},
		{Hint, "hint"},
		{Severity(99), "Severity(99)"},
	}

	for _, tt := range tests {
		got := tt.sev.String()
		if got != tt.want {
			t.Errorf("Severity(%d).String() = %q, want %q", int(tt.sev), got, tt.want)
		}
	}
}

func TestDiagnosticError(t *testing.T) {
	d := Diagnostic{
		Severity: Error,
		Message:  "unexpected token '+'",
		Span: token.Span{
			Start: token.Position{File: "scene.chisel", Line: 3, Col: 8, Offset: 20},
			End:   token.Position{File: "scene.chisel", Line: 3, Col: 9, Offset: 21},
		},
		Help: "did you mean '|' for union?",
	}

	got := d.Error()
	want := "error: unexpected token '+' [scene.chisel:3:8]"
	if got != want {
		t.Errorf("Diagnostic.Error() = %q, want %q", got, want)
	}
}

func TestDiagnosticErrorNoFile(t *testing.T) {
	d := Diagnostic{
		Severity: Warning,
		Message:  "unused variable 'x'",
		Span: token.Span{
			Start: token.Position{Line: 5, Col: 1},
			End:   token.Position{Line: 5, Col: 2},
		},
	}

	got := d.Error()
	want := "warning: unused variable 'x' [5:1]"
	if got != want {
		t.Errorf("Diagnostic.Error() = %q, want %q", got, want)
	}
}

func TestDiagnosticErrorZeroSpan(t *testing.T) {
	d := Diagnostic{
		Severity: Hint,
		Message:  "consider using a shorter name",
	}

	got := d.Error()
	want := "hint: consider using a shorter name"
	if got != want {
		t.Errorf("Diagnostic.Error() = %q, want %q", got, want)
	}
}

func TestDiagnosticImplementsError(t *testing.T) {
	d := Diagnostic{
		Severity: Error,
		Message:  "test error",
	}

	// Verify it satisfies the error interface.
	var err error = d
	if err.Error() != "error: test error" {
		t.Errorf("error interface: got %q", err.Error())
	}
}

func TestDiagnosticWithLabels(t *testing.T) {
	d := Diagnostic{
		Severity: Error,
		Message:  "type mismatch in union",
		Span: token.Span{
			Start: token.Position{File: "test.chisel", Line: 3, Col: 1},
			End:   token.Position{File: "test.chisel", Line: 3, Col: 15},
		},
		Help: "both sides of '|' must be the same SDF type",
		Labels: []Label{
			{
				Span: token.Span{
					Start: token.Position{File: "test.chisel", Line: 3, Col: 1},
					End:   token.Position{File: "test.chisel", Line: 3, Col: 7},
				},
				Message: "this is sdf3d",
			},
			{
				Span: token.Span{
					Start: token.Position{File: "test.chisel", Line: 3, Col: 11},
					End:   token.Position{File: "test.chisel", Line: 3, Col: 15},
				},
				Message: "this is sdf2d",
			},
		},
	}

	// Verify construction: labels are present and addressable.
	if len(d.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(d.Labels))
	}
	if d.Labels[0].Message != "this is sdf3d" {
		t.Errorf("label[0].Message = %q", d.Labels[0].Message)
	}
	if d.Labels[1].Message != "this is sdf2d" {
		t.Errorf("label[1].Message = %q", d.Labels[1].Message)
	}
	if d.Help != "both sides of '|' must be the same SDF type" {
		t.Errorf("Help = %q", d.Help)
	}

	// Error() still produces the top-level summary.
	got := d.Error()
	want := "error: type mismatch in union [test.chisel:3:1]"
	if got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestSeverityValues(t *testing.T) {
	// Ensure the three severities are distinct.
	if Error == Warning || Warning == Hint || Error == Hint {
		t.Error("severity values must be distinct")
	}
}
