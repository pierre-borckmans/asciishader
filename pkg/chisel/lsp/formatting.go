package lsp

import (
	"encoding/json"
	"log"
	"strings"

	"asciishader/pkg/chisel/format"
)

func (s *Server) handleFormatting(id interface{}, params json.RawMessage) {
	var p DocumentFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("formatting unmarshal error: %v", err)
		s.sendResponse(id, nil)
		return
	}

	text := s.getDoc(p.TextDocument.URI)
	if text == "" {
		s.sendResponse(id, nil)
		return
	}

	formatted, err := format.Format(text)
	if err != nil {
		log.Printf("format error: %v", err)
		s.sendResponse(id, nil)
		return
	}

	if formatted == text {
		s.sendResponse(id, []TextEdit{})
		return
	}

	lines := strings.Split(text, "\n")
	lastLine := len(lines) - 1
	lastChar := len(lines[lastLine])

	edits := []TextEdit{
		{
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: lastLine, Character: lastChar},
			},
			NewText: formatted,
		},
	}
	s.sendResponse(id, edits)
}
