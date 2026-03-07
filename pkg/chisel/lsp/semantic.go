package lsp

import (
	"encoding/json"

	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/lang"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
	"asciishader/pkg/chisel/token"
)

// Semantic token types — indices into this slice are used in the encoded data.
var semanticTokenTypes = []string{
	"variable",   // 0
	"function",   // 1
	"parameter",  // 2
	"keyword",    // 3
	"number",     // 4
	"string",     // 5
	"comment",    // 6
	"operator",   // 7
	"property",   // 8  (settings keys, swizzle)
	"type",       // 9  (shape names)
	"enumMember", // 10 (named colors, constants)
	"method",     // 11
}

const (
	stVariable   = 0
	stFunction   = 1
	stParameter  = 2
	stKeyword    = 3
	stNumber     = 4
	stString     = 5
	stComment    = 6
	stOperator   = 7
	stProperty   = 8
	stType       = 9
	stEnumMember = 10
	stMethod     = 11
)

// Semantic token modifiers — bit flags.
var semanticTokenModifiers = []string{
	"declaration",    // 0
	"definition",     // 1
	"readonly",       // 2
	"defaultLibrary", // 3
}

const (
	smDeclaration    = 1 << 0
	smDefinition     = 1 << 1
	smReadonly       = 1 << 2
	smDefaultLibrary = 1 << 3
)

type semToken struct {
	line, col, length int
	tokenType         int
	modifiers         int
}

func (s *Server) handleSemanticTokensFull(id interface{}, params json.RawMessage) {
	var p struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		s.sendResponse(id, nil)
		return
	}

	text := s.getDoc(p.TextDocument.URI)
	if text == "" {
		s.sendResponse(id, map[string]interface{}{"data": []int{}})
		return
	}

	tokens, _ := lexer.Lex(p.TextDocument.URI, text)
	prog, _ := parser.Parse(tokens)

	var semTokens []semToken

	// Pass 1: lexer tokens (comments, numbers, strings, keywords, operators).
	for _, tok := range tokens {
		if tok.Len == 0 {
			continue
		}
		line := tok.Pos.Line - 1
		col := tok.Pos.Col - 1
		switch tok.Kind {
		case token.TokComment:
			semTokens = append(semTokens, semToken{line, col, tok.Len, stComment, 0})
		case token.TokInt, token.TokFloat, token.TokHexColor:
			semTokens = append(semTokens, semToken{line, col, tok.Len, stNumber, 0})
		case token.TokString:
			semTokens = append(semTokens, semToken{line, col, tok.Len, stString, 0})
		case token.TokFor, token.TokIn, token.TokIf, token.TokElse, token.TokStep,
			token.TokGlsl, token.TokTrue, token.TokFalse,
			token.TokLight, token.TokCamera, token.TokBg, token.TokRaymarch,
			token.TokPost, token.TokMat, token.TokDebug:
			semTokens = append(semTokens, semToken{line, col, tok.Len, stKeyword, 0})
		case token.TokPipe, token.TokPipeSmooth, token.TokPipeChamfer,
			token.TokMinus, token.TokMinusSmooth, token.TokMinusChamfer,
			token.TokAmp, token.TokAmpSmooth, token.TokAmpChamfer,
			token.TokPlus, token.TokStar, token.TokSlash, token.TokPercent,
			token.TokEq, token.TokNeq, token.TokLt, token.TokGt, token.TokLte, token.TokGte,
			token.TokBang:
			semTokens = append(semTokens, semToken{line, col, tok.Len, stOperator, 0})
		}
	}

	// Pass 2: AST-based identifier classification.
	if prog != nil {
		c := newClassifier()
		// Collect user definitions.
		ast.Walk(prog, func(n ast.Node) bool {
			if a, ok := n.(*ast.AssignStmt); ok {
				if a.Params != nil {
					c.userFuncs[a.Name] = true
				} else {
					c.userVars[a.Name] = true
				}
			}
			return true
		})

		ast.Walk(prog, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				span := node.NodeSpan()
				line := span.Start.Line - 1
				col := span.Start.Col - 1
				if node.Params != nil {
					semTokens = append(semTokens, semToken{line, col, len(node.Name), stFunction, smDefinition})
					for _, param := range node.Params {
						for _, tok := range tokens {
							if tok.Kind == token.TokIdent && tok.Value == param.Name &&
								tok.Pos.Line == span.Start.Line &&
								tok.Pos.Offset > span.Start.Offset {
								semTokens = append(semTokens, semToken{
									tok.Pos.Line - 1, tok.Pos.Col - 1, len(param.Name),
									stParameter, smDeclaration,
								})
								break
							}
						}
					}
				} else {
					semTokens = append(semTokens, semToken{line, col, len(node.Name), stVariable, smDefinition})
				}

			case *ast.Ident:
				span := node.NodeSpan()
				tt, mod := c.classify(node.Name)
				semTokens = append(semTokens, semToken{span.Start.Line - 1, span.Start.Col - 1, len(node.Name), tt, mod})

			case *ast.FuncCall:
				span := node.NodeSpan()
				tt, mod := c.classifyCall(node.Name)
				semTokens = append(semTokens, semToken{span.Start.Line - 1, span.Start.Col - 1, len(node.Name), tt, mod})

			case *ast.MethodCall:
				for _, tok := range tokens {
					if tok.Kind == token.TokIdent && tok.Value == node.Name &&
						tok.Pos.Offset >= node.Receiver.NodeSpan().End.Offset {
						semTokens = append(semTokens, semToken{
							tok.Pos.Line - 1, tok.Pos.Col - 1, len(node.Name), stMethod, 0,
						})
						break
					}
				}

			case *ast.Swizzle:
				span := node.NodeSpan()
				semTokens = append(semTokens, semToken{
					span.End.Line - 1, span.End.Col - 1 - len(node.Components),
					len(node.Components), stProperty, 0,
				})

			case *ast.ForExpr:
				for _, it := range node.Iterators {
					for _, tok := range tokens {
						if tok.Kind == token.TokIdent && tok.Value == it.Name &&
							tok.Pos.Line >= node.NodeSpan().Start.Line &&
							tok.Pos.Line <= node.NodeSpan().End.Line {
							semTokens = append(semTokens, semToken{
								tok.Pos.Line - 1, tok.Pos.Col - 1, len(it.Name), stVariable, smDeclaration,
							})
							break
						}
					}
				}
			}
			return true
		})
	}

	sortSemTokens(semTokens)
	data := encodeSemTokens(semTokens)
	s.sendResponse(id, map[string]interface{}{"data": data})
}

// classifier holds built-in name sets for fast lookup.
type classifier struct {
	shapes    map[string]bool
	funcs     map[string]bool
	consts    map[string]bool
	colors    map[string]bool
	userFuncs map[string]bool
	userVars  map[string]bool
}

func newClassifier() *classifier {
	c := &classifier{
		shapes:    make(map[string]bool),
		funcs:     make(map[string]bool),
		consts:    make(map[string]bool),
		colors:    make(map[string]bool),
		userFuncs: make(map[string]bool),
		userVars:  make(map[string]bool),
	}
	for _, s := range lang.Shapes3D {
		c.shapes[s.Name] = true
	}
	for _, s := range lang.Shapes2D {
		c.shapes[s.Name] = true
	}
	for _, f := range lang.Functions {
		c.funcs[f.Name] = true
	}
	for _, cn := range lang.Constants {
		c.consts[cn.Name] = true
	}
	for _, col := range lang.Colors {
		c.colors[col.Name] = true
	}
	return c
}

func (c *classifier) classify(name string) (tokenType, modifiers int) {
	switch {
	case c.shapes[name]:
		return stType, smDefaultLibrary
	case c.funcs[name]:
		return stFunction, smDefaultLibrary
	case c.consts[name]:
		return stEnumMember, smReadonly | smDefaultLibrary
	case c.colors[name]:
		return stEnumMember, smDefaultLibrary
	case c.userFuncs[name]:
		return stFunction, 0
	case c.userVars[name]:
		return stVariable, 0
	default:
		return stVariable, 0
	}
}

func (c *classifier) classifyCall(name string) (tokenType, modifiers int) {
	switch {
	case c.shapes[name]:
		return stType, smDefaultLibrary
	case c.funcs[name]:
		return stFunction, smDefaultLibrary
	case c.userFuncs[name]:
		return stFunction, 0
	default:
		return stFunction, 0
	}
}

func sortSemTokens(tokens []semToken) {
	for i := 1; i < len(tokens); i++ {
		j := i
		for j > 0 && semTokenLess(tokens[j], tokens[j-1]) {
			tokens[j], tokens[j-1] = tokens[j-1], tokens[j]
			j--
		}
	}
}

func semTokenLess(a, b semToken) bool {
	if a.line != b.line {
		return a.line < b.line
	}
	return a.col < b.col
}

func encodeSemTokens(tokens []semToken) []int {
	if len(tokens) > 1 {
		deduped := tokens[:1]
		for i := 1; i < len(tokens); i++ {
			prev := deduped[len(deduped)-1]
			cur := tokens[i]
			if cur.line == prev.line && cur.col == prev.col {
				if cur.tokenType > prev.tokenType || cur.modifiers > prev.modifiers {
					deduped[len(deduped)-1] = cur
				}
				continue
			}
			deduped = append(deduped, cur)
		}
		tokens = deduped
	}

	data := make([]int, 0, len(tokens)*5)
	prevLine := 0
	prevCol := 0
	for _, t := range tokens {
		deltaLine := t.line - prevLine
		deltaCol := t.col
		if deltaLine == 0 {
			deltaCol = t.col - prevCol
		}
		data = append(data, deltaLine, deltaCol, t.length, t.tokenType, t.modifiers)
		prevLine = t.line
		prevCol = t.col
	}
	return data
}
