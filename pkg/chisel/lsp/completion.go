package lsp

import (
	"encoding/json"
	"log"
	"strings"

	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
	"asciishader/pkg/chisel/lang"
)

func (s *Server) handleCompletion(id interface{}, params json.RawMessage) {
	var p CompletionParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("completion unmarshal error: %v", err)
		s.sendResponse(id, []CompletionItem{})
		return
	}

	text := s.getDoc(p.TextDocument.URI)
	items := computeCompletions(text, p.Position)
	s.sendResponse(id, items)
}

func computeCompletions(text string, pos Position) []CompletionItem {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return nil
	}
	line := lines[pos.Line]
	col := pos.Character
	if col > len(line) {
		col = len(line)
	}
	prefix := line[:col]

	if strings.HasSuffix(strings.TrimRight(prefix, " \t"), ".") || isDotContext(prefix) {
		return methodCompletions()
	}

	trimmed := strings.TrimLeft(prefix, " \t")
	if trimmed == "" || isStartOfExpression(trimmed) {
		items := shapeCompletions()
		items = append(items, keywordCompletions()...)
		items = append(items, variableCompletions(text)...)
		return items
	}

	items := shapeCompletions()
	items = append(items, keywordCompletions()...)
	items = append(items, builtinFuncCompletions()...)
	items = append(items, variableCompletions(text)...)
	return items
}

func isDotContext(prefix string) bool {
	trimmed := strings.TrimRight(prefix, " \t")
	return len(trimmed) > 0 && trimmed[len(trimmed)-1] == '.'
}

func isStartOfExpression(trimmed string) bool {
	for _, ch := range trimmed {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && ch != '_' {
			return false
		}
	}
	return true
}

func methodCompletions() []CompletionItem {
	items := make([]CompletionItem, 0, len(lang.Methods))
	for _, m := range lang.Methods {
		kind := CIKMethod
		if m.IsColor {
			kind = CIKColor
		}
		items = append(items, CompletionItem{
			Label:         m.Name,
			Kind:          kind,
			Documentation: m.Doc,
		})
	}
	return items
}

func shapeCompletions() []CompletionItem {
	shapes := lang.ShapeNames()
	items := make([]CompletionItem, 0, len(shapes))
	for _, name := range shapes {
		items = append(items, CompletionItem{
			Label:         name,
			Kind:          CIKClass,
			Documentation: lang.ShapeDoc(name),
		})
	}
	return items
}

func keywordCompletions() []CompletionItem {
	all := make([]string, len(lang.Keywords))
	copy(all, lang.Keywords)
	all = append(all, lang.SettingNames()...)
	items := make([]CompletionItem, 0, len(all))
	for _, kw := range all {
		item := CompletionItem{
			Label: kw,
			Kind:  CIKKeyword,
		}
		if doc, ok := keywordDocs[kw]; ok {
			item.Detail = doc
		}
		items = append(items, item)
	}
	return items
}

func builtinFuncCompletions() []CompletionItem {
	items := make([]CompletionItem, 0, len(lang.Functions))
	for _, f := range lang.Functions {
		items = append(items, CompletionItem{
			Label:         f.Name,
			Kind:          CIKFunction,
			Documentation: f.Doc,
		})
	}
	return items
}

func variableCompletions(text string) []CompletionItem {
	tokens, _ := lexer.Lex("completion", text)
	prog, _ := parser.Parse(tokens)
	if prog == nil {
		return nil
	}

	var items []CompletionItem
	seen := make(map[string]bool)
	ast.Walk(prog, func(n ast.Node) bool {
		if assign, ok := n.(*ast.AssignStmt); ok {
			if !seen[assign.Name] {
				seen[assign.Name] = true
				kind := CIKVariable
				detail := "variable"
				if assign.Params != nil {
					kind = CIKFunction
					detail = "function"
				}
				items = append(items, CompletionItem{
					Label:  assign.Name,
					Kind:   kind,
					Detail: detail,
				})
			}
		}
		return true
	})
	return items
}

var keywordDocs = map[string]string{
	"for":      "for var in start..end { body }\nLoop expression. Returns the union of all iterations.",
	"if":       "if cond { then } else { otherwise }\nConditional expression.",
	"else":     "else { ... } or else if ...\nAlternative branch of an if expression.",
	"let":      "name = value\nVariable assignment.",
	"fn":       "name(params) = expr\nFunction definition.",
	"light":    "light { ... }\nConfigure scene lighting.",
	"camera":   "camera { pos: [...], target: [...], fov: 60 }\nConfigure camera.",
	"bg":       "bg #color or bg { ... }\nSet background color or gradient.",
	"raymarch": "raymarch { steps: 128, precision: 0.001 }\nConfigure raymarching parameters.",
	"post":     "post { gamma: 2.2, bloom: { ... } }\nPost-processing effects.",
	"mat":      "mat name = { color: [...], metallic: 1 }\nDefine a named material.",
	"debug":    "debug normals | steps | distance | ao | uv | depth\nVisualize scene internals.",
	"glsl":     "glsl(p) { ... }\nInline raw GLSL escape hatch.",
}
