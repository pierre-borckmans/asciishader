package lsp

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/lang"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
	"asciishader/pkg/chisel/token"
)

func (s *Server) handleDefinition(id interface{}, params json.RawMessage) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
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

	tokens, _ := lexer.Lex(p.TextDocument.URI, text)
	prog, _ := parser.Parse(tokens)
	if prog == nil {
		s.sendResponse(id, nil)
		return
	}

	// Walk the AST to find an AssignStmt with this name.
	var defSpan *token.Span
	ast.Walk(prog, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			if assign.Name == word {
				span := assign.NodeSpan()
				defSpan = &span
				return false
			}
		}
		return true
	})

	if defSpan != nil {
		s.sendResponse(id, map[string]interface{}{
			"uri": p.TextDocument.URI,
			"range": Range{
				Start: Position{
					Line:      max(0, defSpan.Start.Line-1),
					Character: max(0, defSpan.Start.Col-1),
				},
				End: Position{
					Line:      max(0, defSpan.Start.Line-1),
					Character: max(0, defSpan.Start.Col-1+len(word)),
				},
			},
		})
		return
	}

	// Fall back to built-in definitions.
	if line, ok := s.builtinsLine[word]; ok && s.builtinsURI != "" {
		s.sendResponse(id, map[string]interface{}{
			"uri": s.builtinsURI,
			"range": Range{
				Start: Position{Line: line, Character: 0},
				End:   Position{Line: line, Character: len(word)},
			},
		})
		return
	}

	s.sendResponse(id, nil)
}

// initBuiltinsDoc generates a virtual .chisel file containing all built-in
// definitions with doc comments for go-to-definition navigation.
func (s *Server) initBuiltinsDoc() {
	var b strings.Builder
	s.builtinsLine = make(map[string]int)
	line := 0

	w := func(text string) {
		b.WriteString(text)
		b.WriteByte('\n')
		line++
	}

	writeDoc := func(name, doc string) {
		for _, dl := range strings.Split(doc, "\n") {
			w("// " + dl)
		}
		s.builtinsLine[name] = line
	}

	w("// Chisel Built-in Reference")
	w("// This file is auto-generated for go-to-definition support.")
	w("")

	w("// ── 3D Shapes ──────────────────────────────────────────")
	w("")
	for _, shape := range lang.Shapes3D {
		writeDoc(shape.Name, shape.Doc)
		w(shape.Name + " = /* builtin 3D shape */")
		w("")
	}

	w("// ── 2D Shapes ──────────────────────────────────────────")
	w("")
	for _, shape := range lang.Shapes2D {
		writeDoc(shape.Name, shape.Doc)
		w(shape.Name + " = /* builtin 2D shape */")
		w("")
	}

	w("// ── Functions ──────────────────────────────────────────")
	w("")
	for _, fn := range lang.Functions {
		writeDoc(fn.Name, fn.Doc)
		w(fn.Name + " = /* builtin function */")
		w("")
	}

	w("// ── Constants ──────────────────────────────────────────")
	w("")
	for _, c := range lang.Constants {
		writeDoc(c.Name, c.Doc)
		w(c.Name + " = /* builtin constant */")
		w("")
	}

	w("// ── Named Colors ───────────────────────────────────────")
	w("")
	for _, c := range lang.Colors {
		s.builtinsLine[c.Name] = line
		w(c.Name + " = /* builtin color */")
		w("")
	}

	tmpDir := os.TempDir()
	path := filepath.Join(tmpDir, "chisel-builtins.chisel")
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		log.Printf("failed to write builtins doc: %v", err)
		return
	}
	s.builtinsURI = "file://" + path
	log.Printf("builtins doc: %s", path)
}
