// Command chisel-lsp is a minimal Language Server Protocol server for the
// Chisel language. It communicates over stdin/stdout using JSON-RPC 2.0 with
// Content-Length headers. No external LSP framework is required.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"asciishader/pkg/chisel/analyzer"
	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/format"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
	"asciishader/pkg/chisel/token"
)

// ---------------------------------------------------------------------------
// JSON-RPC types
// ---------------------------------------------------------------------------

// Request represents an incoming JSON-RPC 2.0 request or notification.
type Request struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      *json.RawMessage `json:"id,omitempty"`
	Method  string           `json:"method"`
	Params  json.RawMessage  `json:"params,omitempty"`
}

// Response is an outgoing JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// RPCError carries a JSON-RPC error code and message.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Notification is an outgoing JSON-RPC 2.0 notification (no id).
type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

// ---------------------------------------------------------------------------
// LSP types (minimal subset)
// ---------------------------------------------------------------------------

type LSPPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type LSPRange struct {
	Start LSPPosition `json:"start"`
	End   LSPPosition `json:"end"`
}

type LSPDiagnostic struct {
	Range    LSPRange `json:"range"`
	Severity int      `json:"severity"`
	Source   string   `json:"source,omitempty"`
	Message  string   `json:"message"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type DidChangeTextDocumentParams struct {
	TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
	ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
}

type DidCloseTextDocumentParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type TextDocumentPositionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     LSPPosition            `json:"position"`
}

type CompletionParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
	Position     LSPPosition            `json:"position"`
}

type DocumentFormattingParams struct {
	TextDocument TextDocumentIdentifier `json:"textDocument"`
}

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"`
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *LSPRange     `json:"range,omitempty"`
}

type MarkupContent struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type TextEdit struct {
	Range   LSPRange `json:"range"`
	NewText string   `json:"newText"`
}

// CompletionItemKind constants.
const (
	CIKText     = 1
	CIKMethod   = 2
	CIKFunction = 3
	CIKVariable = 6
	CIKClass    = 7  // used for shapes
	CIKKeyword  = 14
	CIKSnippet  = 15
	CIKColor    = 16
)

// ---------------------------------------------------------------------------
// Documentation tables
// ---------------------------------------------------------------------------

var shapeDocs = map[string]string{
	"sphere":           "sphere(radius = 1)\nCreates a sphere SDF.",
	"box":              "box(width = 1, height = 1, depth = 1)\nCreates a box SDF.",
	"cylinder":         "cylinder(radius = 0.5, height = 2)\nCreates a cylinder SDF.",
	"torus":            "torus(major = 1, minor = 0.3)\nCreates a torus SDF.",
	"capsule":          "capsule(a, b, radius = 0.25)\nCreates a capsule SDF between two endpoints.",
	"cone":             "cone(bottomRadius, topRadius, height)\nCreates a cone SDF.",
	"plane":            "plane\nInfinite ground plane at y=0.",
	"octahedron":       "octahedron(size = 1)\nCreates an octahedron SDF.",
	"pyramid":          "pyramid(height = 1)\nCreates a pyramid SDF.",
	"ellipsoid":        "ellipsoid(rx, ry, rz)\nCreates an ellipsoid SDF with per-axis radii.",
	"circle":           "circle(radius = 1)\n2D circle SDF. Must be extruded or revolved to render.",
	"rect":             "rect(width = 1, height = 1)\n2D rectangle SDF.",
	"hexagon":          "hexagon(radius = 1)\n2D hexagon SDF.",
	"polygon":          "polygon(points)\n2D polygon SDF from a list of [x,y] points.",
	"rounded_box":      "rounded_box(size, edgeRadius)\nBox with rounded edges.",
	"wireframe_box":    "wireframe_box(size, thickness)\nWireframe box.",
	"rounded_cylinder": "rounded_cylinder(radius, height, rounding)\nCylinder with rounded edges.",
}

var methodDocs = map[string]string{
	"at":        ".at(x, y, z)\nTranslate the shape. Named args: at(x: 2) or at(y: -1).",
	"scale":     ".scale(s) or .scale(x, y, z)\nScale uniformly or non-uniformly.",
	"rot":       ".rot(degrees, axis)\nRotate around an axis (x, y, or z).",
	"orient":    ".orient(axis)\nAlign shape along an axis direction.",
	"mirror":    ".mirror(axes...)\nMirror across axes. O(1) space folding.",
	"rep":       ".rep(spacing) or .rep(spacing, count: N)\nRepeat in space. O(1) cost.",
	"array":     ".array(count, radius: r)\nCircular array of copies.",
	"morph":     ".morph(other, t)\nBlend between two shapes by interpolating SDFs.",
	"shell":     ".shell(thickness)\nHollow out the shape.",
	"onion":     ".onion(thickness)\nCreate concentric shells.",
	"displace":  ".displace(expr)\nDisplace surface using expression with p.",
	"dilate":    ".dilate(amount)\nExpand outward.",
	"erode":     ".erode(amount)\nShrink inward.",
	"round":     ".round(radius)\nRound edges.",
	"elongate":  ".elongate(x, y, z)\nStretch along axes.",
	"twist":     ".twist(amount)\nTwist around Y axis.",
	"bend":      ".bend(amount)\nBend the shape.",
	"color":     ".color(r, g, b) or .color(#hex)\nSet shape color.",
	"metallic":  ".metallic(value)\nSet metallic material property (0..1).",
	"roughness": ".roughness(value)\nSet roughness material property (0..1).",
	"emission":  ".emission(r, g, b) or .emission(intensity)\nSet emissive color or intensity.",
	"opacity":   ".opacity(value)\nSet transparency (0..1).",
	"mat":       ".mat(material)\nApply a named material definition.",
	"extrude":   ".extrude(depth)\nExtrude a 2D shape into 3D.",
	"revolve":   ".revolve(radius)\nRevolve a 2D shape around an axis.",
	"red":       ".red\nShorthand color: red.",
	"blue":      ".blue\nShorthand color: blue.",
	"green":     ".green\nShorthand color: green.",
	"white":     ".white\nShorthand color: white.",
	"black":     ".black\nShorthand color: black.",
	"yellow":    ".yellow\nShorthand color: yellow.",
	"cyan":      ".cyan\nShorthand color: cyan.",
	"magenta":   ".magenta\nShorthand color: magenta.",
	"orange":    ".orange\nShorthand color: orange.",
	"gray":      ".gray\nShorthand color: gray.",
}

var builtinFuncDocs = map[string]string{
	"sin":        "sin(x)\nSine function.",
	"cos":        "cos(x)\nCosine function.",
	"tan":        "tan(x)\nTangent function.",
	"asin":       "asin(x)\nArc sine.",
	"acos":       "acos(x)\nArc cosine.",
	"atan":       "atan(x)\nArc tangent.",
	"atan2":      "atan2(y, x)\nTwo-argument arc tangent.",
	"abs":        "abs(x)\nAbsolute value.",
	"min":        "min(a, b)\nMinimum of two values.",
	"max":        "max(a, b)\nMaximum of two values.",
	"sqrt":       "sqrt(x)\nSquare root.",
	"pow":        "pow(x, n)\nRaise x to the power n.",
	"exp":        "exp(x)\nExponential function.",
	"log":        "log(x)\nNatural logarithm.",
	"floor":      "floor(x)\nRound down to integer.",
	"ceil":       "ceil(x)\nRound up to integer.",
	"fract":      "fract(x)\nFractional part.",
	"sign":       "sign(x)\nSign of x (-1, 0, or 1).",
	"length":     "length(v)\nLength of a vector.",
	"normalize":  "normalize(v)\nNormalize a vector to unit length.",
	"dot":        "dot(a, b)\nDot product of two vectors.",
	"cross":      "cross(a, b)\nCross product of two vec3 vectors.",
	"distance":   "distance(a, b)\nDistance between two points.",
	"reflect":    "reflect(v, n)\nReflect vector v around normal n.",
	"mix":        "mix(a, b, t)\nLinear interpolation between a and b.",
	"smoothstep": "smoothstep(edge0, edge1, x)\nSmooth Hermite interpolation.",
	"step":       "step(edge, x)\nHard threshold: 0 if x < edge, else 1.",
	"clamp":      "clamp(x, lo, hi)\nClamp x to [lo, hi].",
	"saturate":   "saturate(x)\nClamp x to [0, 1].",
	"remap":      "remap(x, a, b, c, d)\nRemap x from [a,b] to [c,d].",
	"mod":        "mod(x, y)\nModulo operation.",
	"noise":      "noise(p)\nPerlin/simplex noise at point p. Returns -1..1.",
	"fbm":        "fbm(p, octaves: 6)\nFractal Brownian motion noise.",
	"voronoi":    "voronoi(p)\nVoronoi cell noise. Returns distance to nearest cell.",
	"rgb":        "rgb(r, g, b)\nColor from 0..255 RGB components.",
	"hsl":        "hsl(h, s, l)\nColor from HSL components.",
	"round":      "round(x)\nRound to nearest integer.",
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
	"raymarch":  "raymarch { steps: 128, precision: 0.001 }\nConfigure raymarching parameters.",
	"post":     "post { gamma: 2.2, bloom: { ... } }\nPost-processing effects.",
	"mat":      "mat name = { color: [...], metallic: 1 }\nDefine a named material.",
	"debug":    "debug normals | steps | distance | ao | uv | depth\nVisualize scene internals.",
	"glsl":     "glsl(p) { ... }\nInline raw GLSL escape hatch.",
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type server struct {
	mu        sync.Mutex
	docs      map[string]string // URI -> content
	writer    *bufio.Writer
	writeMu   sync.Mutex
	shutdown  bool
}

func newServer() *server {
	return &server{
		docs:   make(map[string]string),
		writer: bufio.NewWriter(os.Stdout),
	}
}

// ---------------------------------------------------------------------------
// Transport: reading
// ---------------------------------------------------------------------------

func readMessage(reader *bufio.Reader) ([]byte, error) {
	// Read headers until empty line.
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(line, "Content-Length:") {
			valStr := strings.TrimSpace(strings.TrimPrefix(line, "Content-Length:"))
			n, err := strconv.Atoi(valStr)
			if err == nil {
				contentLength = n
			}
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}

	body := make([]byte, contentLength)
	_, err := io.ReadFull(reader, body)
	if err != nil {
		return nil, err
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// Transport: writing
// ---------------------------------------------------------------------------

func (s *server) sendJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("marshal error: %v", err)
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	s.writer.WriteString(header)
	s.writer.Write(data)
	s.writer.Flush()
}

func (s *server) sendResponse(id interface{}, result interface{}) {
	s.sendJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *server) sendError(id interface{}, code int, message string) {
	s.sendJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

func (s *server) sendNotification(method string, params interface{}) {
	s.sendJSON(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func (s *server) handle(req Request) {
	// Extract id for responses.
	var id interface{}
	if req.ID != nil {
		_ = json.Unmarshal(*req.ID, &id)
	}

	switch req.Method {
	case "initialize":
		s.handleInitialize(id)
	case "initialized":
		// No response needed.
	case "shutdown":
		s.shutdown = true
		s.sendResponse(id, nil)
	case "exit":
		if s.shutdown {
			os.Exit(0)
		}
		os.Exit(1)
	case "textDocument/didOpen":
		s.handleDidOpen(req.Params)
	case "textDocument/didChange":
		s.handleDidChange(req.Params)
	case "textDocument/didClose":
		s.handleDidClose(req.Params)
	case "textDocument/completion":
		s.handleCompletion(id, req.Params)
	case "textDocument/hover":
		s.handleHover(id, req.Params)
	case "textDocument/formatting":
		s.handleFormatting(id, req.Params)
	default:
		if req.ID != nil {
			// Unknown request with an id -- respond with MethodNotFound.
			s.sendError(id, -32601, "method not found: "+req.Method)
		}
		// Unknown notification -- ignore.
	}
}

// ---------------------------------------------------------------------------
// initialize
// ---------------------------------------------------------------------------

func (s *server) handleInitialize(id interface{}) {
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": 1, // Full sync
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", "("},
			},
			"hoverProvider":              true,
			"documentFormattingProvider": true,
			"diagnosticProvider": map[string]interface{}{
				"interFileDependencies": false,
				"workspaceDiagnostics":  false,
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    "chisel-lsp",
			"version": "0.1.0",
		},
	}
	s.sendResponse(id, result)
}

// ---------------------------------------------------------------------------
// textDocument/didOpen
// ---------------------------------------------------------------------------

func (s *server) handleDidOpen(params json.RawMessage) {
	var p DidOpenTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("didOpen unmarshal error: %v", err)
		return
	}
	s.mu.Lock()
	s.docs[p.TextDocument.URI] = p.TextDocument.Text
	s.mu.Unlock()
	s.publishDiagnostics(p.TextDocument.URI, p.TextDocument.Text)
}

// ---------------------------------------------------------------------------
// textDocument/didChange
// ---------------------------------------------------------------------------

func (s *server) handleDidChange(params json.RawMessage) {
	var p DidChangeTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("didChange unmarshal error: %v", err)
		return
	}
	if len(p.ContentChanges) == 0 {
		return
	}
	text := p.ContentChanges[len(p.ContentChanges)-1].Text
	s.mu.Lock()
	s.docs[p.TextDocument.URI] = text
	s.mu.Unlock()
	s.publishDiagnostics(p.TextDocument.URI, text)
}

// ---------------------------------------------------------------------------
// textDocument/didClose
// ---------------------------------------------------------------------------

func (s *server) handleDidClose(params json.RawMessage) {
	var p DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("didClose unmarshal error: %v", err)
		return
	}
	s.mu.Lock()
	delete(s.docs, p.TextDocument.URI)
	s.mu.Unlock()
	// Clear diagnostics for the closed file.
	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         p.TextDocument.URI,
		"diagnostics": []LSPDiagnostic{},
	})
}

// ---------------------------------------------------------------------------
// Diagnostics
// ---------------------------------------------------------------------------

func (s *server) publishDiagnostics(uri, source string) {
	var allDiags []diagnostic.Diagnostic

	// Lex.
	tokens, lexDiags := lexer.Lex(uri, source)
	allDiags = append(allDiags, lexDiags...)

	// Parse.
	prog, parseDiags := parser.Parse(tokens)
	allDiags = append(allDiags, parseDiags...)

	// Analyze (only if we got a program).
	if prog != nil {
		analyzeDiags := analyzer.Analyze(prog)
		allDiags = append(allDiags, analyzeDiags...)
	}

	// Convert to LSP diagnostics.
	lspDiags := make([]LSPDiagnostic, 0, len(allDiags))
	for _, d := range allDiags {
		lspDiag := convertDiagnostic(d)
		lspDiags = append(lspDiags, lspDiag)
	}

	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         uri,
		"diagnostics": lspDiags,
	})
}

func convertDiagnostic(d diagnostic.Diagnostic) LSPDiagnostic {
	sev := 1 // Error
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

	return LSPDiagnostic{
		Range: LSPRange{
			Start: LSPPosition{
				Line:      max(0, d.Span.Start.Line-1), // Convert 1-based to 0-based
				Character: max(0, d.Span.Start.Col-1),
			},
			End: LSPPosition{
				Line:      max(0, d.Span.End.Line-1),
				Character: max(0, d.Span.End.Col-1),
			},
		},
		Severity: sev,
		Source:   "chisel",
		Message:  msg,
	}
}

// ---------------------------------------------------------------------------
// textDocument/completion
// ---------------------------------------------------------------------------

func (s *server) handleCompletion(id interface{}, params json.RawMessage) {
	var p CompletionParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("completion unmarshal error: %v", err)
		s.sendResponse(id, []CompletionItem{})
		return
	}

	s.mu.Lock()
	text := s.docs[p.TextDocument.URI]
	s.mu.Unlock()

	items := s.computeCompletions(text, p.Position)
	s.sendResponse(id, items)
}

func (s *server) computeCompletions(text string, pos LSPPosition) []CompletionItem {
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

	// After dot: method completions.
	if strings.HasSuffix(strings.TrimRight(prefix, " \t"), ".") || isDotContext(prefix) {
		return methodCompletions()
	}

	// At start of line (ignoring whitespace): shape completions + keywords.
	trimmed := strings.TrimLeft(prefix, " \t")
	if trimmed == "" || isStartOfExpression(trimmed) {
		items := shapeCompletions()
		items = append(items, keywordCompletions()...)
		items = append(items, variableCompletions(text)...)
		return items
	}

	// Default: all completions.
	items := shapeCompletions()
	items = append(items, keywordCompletions()...)
	items = append(items, builtinFuncCompletions()...)
	items = append(items, variableCompletions(text)...)
	return items
}

func isDotContext(prefix string) bool {
	// Check if the character before the cursor (ignoring spaces) is a dot.
	trimmed := strings.TrimRight(prefix, " \t")
	return len(trimmed) > 0 && trimmed[len(trimmed)-1] == '.'
}

func isStartOfExpression(trimmed string) bool {
	// If the trimmed prefix is only partial identifier chars, user is typing at start.
	for _, ch := range trimmed {
		if !((ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_') {
			return false
		}
	}
	return true
}

func methodCompletions() []CompletionItem {
	methods := []string{
		"at", "scale", "rot", "orient", "mirror", "rep", "array",
		"morph", "shell", "onion", "displace", "dilate", "erode",
		"round", "elongate", "twist", "bend",
		"color", "metallic", "roughness", "emission", "opacity", "mat",
		"extrude", "revolve",
		"red", "blue", "green", "white", "black", "yellow",
		"cyan", "magenta", "orange", "gray",
	}
	items := make([]CompletionItem, 0, len(methods))
	for _, m := range methods {
		item := CompletionItem{
			Label: m,
			Kind:  CIKMethod,
		}
		if doc, ok := methodDocs[m]; ok {
			item.Documentation = doc
		}
		items = append(items, item)
	}
	return items
}

func shapeCompletions() []CompletionItem {
	shapes := []string{
		"sphere", "box", "cylinder", "torus", "capsule", "cone",
		"plane", "octahedron", "pyramid", "ellipsoid",
		"circle", "rect", "hexagon", "polygon",
	}
	items := make([]CompletionItem, 0, len(shapes))
	for _, name := range shapes {
		item := CompletionItem{
			Label: name,
			Kind:  CIKClass,
		}
		if doc, ok := shapeDocs[name]; ok {
			item.Documentation = doc
		}
		items = append(items, item)
	}
	return items
}

func keywordCompletions() []CompletionItem {
	keywords := []string{
		"for", "if", "else", "light", "camera", "bg",
		"raymarch", "post", "mat", "debug", "glsl",
	}
	items := make([]CompletionItem, 0, len(keywords))
	for _, kw := range keywords {
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
	funcs := []string{
		"sin", "cos", "tan", "asin", "acos", "atan", "atan2",
		"abs", "min", "max", "sqrt", "pow", "exp", "log",
		"floor", "ceil", "fract", "sign",
		"length", "normalize", "dot", "cross", "distance", "reflect",
		"mix", "smoothstep", "step", "clamp", "saturate", "remap",
		"mod", "noise", "fbm", "voronoi",
		"rgb", "hsl",
	}
	items := make([]CompletionItem, 0, len(funcs))
	for _, f := range funcs {
		item := CompletionItem{
			Label: f,
			Kind:  CIKFunction,
		}
		if doc, ok := builtinFuncDocs[f]; ok {
			item.Documentation = doc
		}
		items = append(items, item)
	}
	return items
}

func variableCompletions(text string) []CompletionItem {
	// Parse the document and collect top-level variable/function names.
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

// ---------------------------------------------------------------------------
// textDocument/hover
// ---------------------------------------------------------------------------

func (s *server) handleHover(id interface{}, params json.RawMessage) {
	var p TextDocumentPositionParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("hover unmarshal error: %v", err)
		s.sendResponse(id, nil)
		return
	}

	s.mu.Lock()
	text := s.docs[p.TextDocument.URI]
	s.mu.Unlock()

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

func wordAtPosition(text string, pos LSPPosition) string {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character >= len(line) {
		return ""
	}

	// Find word boundaries.
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

	// Constants.
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

// ---------------------------------------------------------------------------
// textDocument/formatting
// ---------------------------------------------------------------------------

func (s *server) handleFormatting(id interface{}, params json.RawMessage) {
	var p DocumentFormattingParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("formatting unmarshal error: %v", err)
		s.sendResponse(id, nil)
		return
	}

	s.mu.Lock()
	text := s.docs[p.TextDocument.URI]
	s.mu.Unlock()

	if text == "" {
		s.sendResponse(id, nil)
		return
	}

	formatted, err := format.Format(text)
	if err != nil {
		log.Printf("format error: %v", err)
		// If formatting fails (e.g. parse error), return no edits.
		s.sendResponse(id, nil)
		return
	}

	if formatted == text {
		s.sendResponse(id, []TextEdit{})
		return
	}

	// Replace the entire document.
	lines := strings.Split(text, "\n")
	lastLine := len(lines) - 1
	lastChar := len(lines[lastLine])

	edits := []TextEdit{
		{
			Range: LSPRange{
				Start: LSPPosition{Line: 0, Character: 0},
				End:   LSPPosition{Line: lastLine, Character: lastChar},
			},
			NewText: formatted,
		},
	}
	s.sendResponse(id, edits)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// max returns the larger of two ints. (Go < 1.21 compat; but since go.mod says
// 1.25 we have the builtin, however we define it to be explicit.)
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Ensure token package is used (it's referenced transitively via ast.Walk).
var _ = token.TokEOF

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func main() {
	log.SetOutput(os.Stderr)
	log.SetPrefix("[chisel-lsp] ")
	log.SetFlags(log.Ltime)

	log.Println("starting chisel-lsp server")

	srv := newServer()
	reader := bufio.NewReader(os.Stdin)

	for {
		body, err := readMessage(reader)
		if err != nil {
			if err == io.EOF {
				log.Println("stdin closed, exiting")
				os.Exit(0)
			}
			log.Printf("read error: %v", err)
			os.Exit(1)
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			log.Printf("unmarshal error: %v", err)
			continue
		}

		log.Printf("-> %s", req.Method)
		srv.handle(req)
	}
}
