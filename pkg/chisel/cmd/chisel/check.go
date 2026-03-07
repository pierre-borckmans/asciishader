package main

import (
	"fmt"
	"io"
	"os"

	"asciishader/pkg/chisel/compiler/analyzer"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
)

func cmdCheck(args []string, stdout, stderr io.Writer) int {
	f, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "chisel check: %s\n", err)
		return 1
	}
	if f.file == "" {
		fmt.Fprintln(stderr, "chisel check: missing file argument")
		return 1
	}

	source, err := os.ReadFile(f.file)
	if err != nil {
		fmt.Fprintf(stderr, "chisel check: %s\n", err)
		return 1
	}
	src := string(source)
	color := useColor(f, stderr)

	tokens, lexDiags := lexer.Lex(f.file, src)
	allDiags := lexDiags

	if !hasError(lexDiags) {
		prog, parseDiags := parser.Parse(tokens)
		allDiags = append(allDiags, parseDiags...)
		if !hasError(parseDiags) {
			allDiags = append(allDiags, analyzer.Analyze(prog)...)
		}
	}

	if printDiags(src, allDiags, stderr, color) {
		return 1
	}
	fmt.Fprintf(stdout, "%s: ok\n", f.file)
	return 0
}
