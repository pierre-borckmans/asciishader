// Command chisel-lsp starts the Chisel Language Server Protocol server.
// It communicates over stdin/stdout using JSON-RPC 2.0.
package main

import (
	"log"
	"os"

	"asciishader/pkg/chisel/lsp"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetPrefix("[chisel-lsp] ")
	log.SetFlags(log.Ltime)

	log.Println("starting chisel-lsp server")
	lsp.NewServer().Run()
}
