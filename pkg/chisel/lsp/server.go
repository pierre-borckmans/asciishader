package lsp

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
)

// Server is the Chisel LSP server.
type Server struct {
	mu       sync.Mutex
	docs     map[string]string // URI -> content
	writer   *bufio.Writer
	writeMu  sync.Mutex
	shutdown bool

	// Virtual builtins document for go-to-definition on built-in names.
	builtinsURI  string         // file:// URI to the builtins file
	builtinsLine map[string]int // name -> 0-based line number
}

// NewServer creates a new LSP server writing to stdout.
func NewServer() *Server {
	s := &Server{
		docs:   make(map[string]string),
		writer: bufio.NewWriter(os.Stdout),
	}
	s.initBuiltinsDoc()
	return s
}

// Run reads JSON-RPC messages from stdin and dispatches them.
func (s *Server) Run() {
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
		s.handle(req)
	}
}

// getDoc returns the document text for a URI, holding the lock briefly.
func (s *Server) getDoc(uri string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docs[uri]
}

// ---------------------------------------------------------------------------
// Transport
// ---------------------------------------------------------------------------

func readMessage(reader *bufio.Reader) ([]byte, error) {
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

func (s *Server) sendJSON(v interface{}) {
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

func (s *Server) sendResponse(id interface{}, result interface{}) {
	s.sendJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) sendError(id interface{}, code int, message string) {
	s.sendJSON(Response{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	})
}

func (s *Server) sendNotification(method string, params interface{}) {
	s.sendJSON(Notification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

// ---------------------------------------------------------------------------
// Dispatch
// ---------------------------------------------------------------------------

func (s *Server) handle(req Request) {
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
	case "textDocument/foldingRange":
		s.handleFoldingRange(id, req.Params)
	case "textDocument/definition":
		s.handleDefinition(id, req.Params)
	case "textDocument/semanticTokens/full":
		s.handleSemanticTokensFull(id, req.Params)
	default:
		if req.ID != nil {
			s.sendError(id, -32601, "method not found: "+req.Method)
		}
	}
}

// ---------------------------------------------------------------------------
// initialize
// ---------------------------------------------------------------------------

func (s *Server) handleInitialize(id interface{}) {
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": 1,
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", "("},
			},
			"hoverProvider":              true,
			"documentFormattingProvider": true,
			"foldingRangeProvider":       true,
			"definitionProvider":         true,
			"semanticTokensProvider": map[string]interface{}{
				"legend": map[string]interface{}{
					"tokenTypes":    semanticTokenTypes,
					"tokenModifiers": semanticTokenModifiers,
				},
				"full": true,
			},
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
// Document sync
// ---------------------------------------------------------------------------

func (s *Server) handleDidOpen(params json.RawMessage) {
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

func (s *Server) handleDidChange(params json.RawMessage) {
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

func (s *Server) handleDidClose(params json.RawMessage) {
	var p DidCloseTextDocumentParams
	if err := json.Unmarshal(params, &p); err != nil {
		log.Printf("didClose unmarshal error: %v", err)
		return
	}
	s.mu.Lock()
	delete(s.docs, p.TextDocument.URI)
	s.mu.Unlock()
	s.sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         p.TextDocument.URI,
		"diagnostics": []Diagnostic{},
	})
}
