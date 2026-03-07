package lsp

import (
	"encoding/json"
	"log"
	"strings"

	"asciishader/pkg/chisel/lang"
)

func (s *Server) handleHover(id interface{}, params json.RawMessage) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("hover unmarshal error: %v", err)
		s.sendResponse(id, nil)
		return
	}

	text := s.getDoc(p.TextDocument.URI)
	if text == "" {
		s.sendResponse(id, nil)
		return
	}

	word := wordAtPosition(text, p.Position)
	if word == "" {
		s.sendResponse(id, nil)
		return
	}

	doc := lookupDoc(word)
	if doc == "" {
		s.sendResponse(id, nil)
		return
	}

	s.sendResponse(id, Hover{
		Contents: MarkupContent{
			Kind:  "markdown",
			Value: "```\n" + doc + "\n```",
		},
	})
}

func wordAtPosition(text string, pos Position) string {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character >= len(line) {
		return ""
	}

	start := pos.Character
	for start > 0 && isIdentChar(line[start-1]) {
		start--
	}
	end := pos.Character
	for end < len(line) && isIdentChar(line[end]) {
		end++
	}
	if start == end {
		return ""
	}
	return line[start:end]
}

func isIdentChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

// Documentation tables — derived from the lang registry.
var shapeDocs = func() map[string]string {
	m := make(map[string]string)
	for _, s := range lang.Shapes3D {
		m[s.Name] = s.Doc
	}
	for _, s := range lang.Shapes2D {
		m[s.Name] = s.Doc
	}
	return m
}()

var methodDocs = func() map[string]string {
	m := make(map[string]string)
	for _, method := range lang.Methods {
		m[method.Name] = method.Doc
	}
	return m
}()

var builtinFuncDocs = func() map[string]string {
	m := make(map[string]string)
	for _, f := range lang.Functions {
		m[f.Name] = f.Doc
	}
	return m
}()

func lookupDoc(word string) string {
	if doc, ok := shapeDocs[word]; ok {
		return doc
	}
	if doc, ok := methodDocs[word]; ok {
		return doc
	}
	if doc, ok := builtinFuncDocs[word]; ok {
		return doc
	}
	if doc, ok := keywordDocs[word]; ok {
		return doc
	}

	switch word {
	case "PI":
		return "PI = 3.14159265...\nThe ratio of a circle's circumference to its diameter."
	case "TAU":
		return "TAU = 6.28318530...\nTwo times PI."
	case "E":
		return "E = 2.71828182...\nEuler's number."
	case "t":
		return "t\nTime in seconds since start. Always available."
	case "p":
		return "p\nCurrent evaluation point (vec3). Available in materials, noise, displacement."
	case "x":
		return "x\nAxis constant: vec3(1, 0, 0)."
	case "y":
		return "y\nAxis constant: vec3(0, 1, 0)."
	case "z":
		return "z\nAxis constant: vec3(0, 0, 1)."
	}

	return ""
}
