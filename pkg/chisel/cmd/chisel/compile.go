package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"asciishader/pkg/chisel"
	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
	"asciishader/pkg/chisel/compiler/token"
)

func cmdCompile(args []string, stdout, stderr io.Writer) int {
	f, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "chisel compile: %s\n", err)
		return 1
	}
	if f.file == "" {
		fmt.Fprintln(stderr, "chisel compile: missing file argument")
		return 1
	}

	source, err := os.ReadFile(f.file)
	if err != nil {
		fmt.Fprintf(stderr, "chisel compile: %s\n", err)
		return 1
	}
	src := string(source)
	color := useColor(f, stderr)

	if f.tokens {
		tokens, lexDiags := lexer.Lex(f.file, src)
		if printDiags(src, lexDiags, stderr, color) {
			return 1
		}
		var sb strings.Builder
		for _, tok := range tokens {
			if tok.Kind == token.TokEOF {
				break
			}
			fmt.Fprintf(&sb, "%d:%d %s %q\n", tok.Pos.Line, tok.Pos.Col, tok.Kind, tok.Value)
		}
		return writeOrFail(f, stdout, stderr, sb.String(), "compile")
	}

	if f.astFlag {
		tokens, lexDiags := lexer.Lex(f.file, src)
		if printDiags(src, lexDiags, stderr, color) {
			return 1
		}
		prog, parseDiags := parser.Parse(tokens)
		if printDiags(src, parseDiags, stderr, color) {
			return 1
		}
		var sb strings.Builder
		ast.Print(&sb, prog, 0)
		return writeOrFail(f, stdout, stderr, sb.String(), "compile")
	}

	glsl, diags := chisel.Compile(src)
	if printDiags(src, diags, stderr, color) {
		return 1
	}
	return writeOrFail(f, stdout, stderr, glsl, "compile")
}
