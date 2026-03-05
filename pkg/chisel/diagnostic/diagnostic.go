// Package diagnostic defines diagnostic types for the Chisel compiler,
// including errors, warnings, and hints with source spans.
package diagnostic

import (
	"fmt"
	"strings"

	"asciishader/pkg/chisel/token"
)

// Severity indicates the severity level of a diagnostic.
type Severity int

const (
	Error   Severity = iota // compilation error -- must be fixed
	Warning                 // potential issue -- code will compile
	Hint                    // suggestion for improvement
)

// String returns a human-readable name for the severity.
func (s Severity) String() string {
	switch s {
	case Error:
		return "error"
	case Warning:
		return "warning"
	case Hint:
		return "hint"
	default:
		return fmt.Sprintf("Severity(%d)", int(s))
	}
}

// Label is an additional annotated span within a diagnostic,
// providing extra context about the error.
type Label struct {
	Span    token.Span
	Message string
}

// Diagnostic represents a compiler diagnostic (error, warning, or hint)
// tied to a location in the source code.
type Diagnostic struct {
	Severity Severity
	Message  string
	Span     token.Span
	Help     string  // optional suggestion for how to fix the issue
	Labels   []Label // optional additional labeled spans
}

// Error implements the error interface, returning a formatted string
// like "error: unexpected token '+' [file.chisel:3:8]".
func (d Diagnostic) Error() string {
	var b strings.Builder
	b.WriteString(d.Severity.String())
	b.WriteString(": ")
	b.WriteString(d.Message)

	if d.Span.Start.File != "" || d.Span.Start.Line != 0 {
		b.WriteString(" [")
		b.WriteString(d.Span.Start.String())
		b.WriteString("]")
	}

	return b.String()
}
