package lsp

import (
	"asciishader/pkg/chisel/analyzer"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
)

func (s *Server) publishDiagnostics(uri, source string) {
	var allDiags []diagnostic.Diagnostic

	tokens, lexDiags := lexer.Lex(uri, source)
	allDiags = append(allDiags, lexDiags...)

	prog, parseDiags := parser.Parse(tokens)
	allDiags = append(allDiags, parseDiags...)

	if prog != nil {
		analyzeDiags := analyzer.Analyze(prog)
		allDiags = append(allDiags, analyzeDiags...)
	}

	lspDiags := make([]Diagnostic, 0, len(allDiags))
	for _, d := range allDiags {
		lspDiags = append(lspDiags, convertDiagnostic(d))
	}

	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         uri,
		"diagnostics": lspDiags,
	})
}

func convertDiagnostic(d diagnostic.Diagnostic) Diagnostic {
	sev := 1
	switch d.Severity {
	case diagnostic.Error:
		sev = 1
	case diagnostic.Warning:
		sev = 2
	case diagnostic.Hint:
		sev = 4
	}

	msg := d.Message
	if d.Help != "" {
		msg += "\n" + d.Help
	}

	return Diagnostic{
		Range: Range{
			Start: Position{
				Line:      max(0, d.Span.Start.Line-1),
				Character: max(0, d.Span.Start.Col-1),
			},
			End: Position{
				Line:      max(0, d.Span.End.Line-1),
				Character: max(0, d.Span.End.Col-1),
			},
		},
		Severity: sev,
		Source:   "chisel",
		Message:  msg,
	}
}
