package lsp

import (
	"encoding/json"

	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
)

func (s *Server) handleFoldingRange(id interface{}, params json.RawMessage) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendResponse(id, nil)
		return
	}

	text := s.getDoc(p.TextDocument.URI)
	if text == "" {
		s.sendResponse(id, []interface{}{})
		return
	}

	tokens, _ := lexer.Lex(p.TextDocument.URI, text)
	prog, _ := parser.Parse(tokens)
	if prog == nil {
		s.sendResponse(id, []interface{}{})
		return
	}

	var ranges []map[string]interface{}
	ast.Walk(prog, func(n ast.Node) bool {
		if n == nil {
			return false
		}
		span := n.NodeSpan()
		if span.Start.Line >= span.End.Line {
			return true
		}
		kind := ""
		switch n.(type) {
		case *ast.Block:
			kind = "region"
		case *ast.ForExpr:
			kind = "region"
		case *ast.IfExpr:
			kind = "region"
		case *ast.GlslEscape:
			kind = "region"
		case *ast.SettingStmt:
			kind = "region"
		default:
			return true
		}
		ranges = append(ranges, map[string]interface{}{
			"startLine":      span.Start.Line - 1,
			"startCharacter": span.Start.Col - 1,
			"endLine":        span.End.Line - 1,
			"endCharacter":   span.End.Col - 1,
			"kind":           kind,
		})
		return true
	})

	s.sendResponse(id, ranges)
}
