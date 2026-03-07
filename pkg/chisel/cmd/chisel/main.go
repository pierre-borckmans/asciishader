// Command chisel is the CLI for the Chisel language compiler.
// It can compile .chisel files to GLSL, check for errors, and format source code.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"asciishader/pkg/chisel"
	"asciishader/pkg/chisel/compiler/analyzer"
	"asciishader/pkg/chisel/compiler/ast"
	"asciishader/pkg/chisel/compiler/diagnostic"
	"asciishader/pkg/chisel/compiler/lexer"
	"asciishader/pkg/chisel/compiler/parser"
	"asciishader/pkg/chisel/compiler/token"
	"asciishader/pkg/chisel/format"
)

const usage = `Usage: chisel <command> [options] [file]

Commands:
  compile <file>     Compile .chisel to GLSL (output to stdout)
  check <file>       Check for errors without compiling
  fmt <file>         Format a .chisel file (in-place or stdout)
  fmt -              Format stdin to stdout

Options:
  -o <file>          Write output to file instead of stdout
  -no-color          Disable colored error output
  -ast               Print AST instead of GLSL (for debugging)
  -tokens            Print token stream (for debugging)

Examples:
  chisel compile scene.chisel                # print GLSL to stdout
  chisel compile scene.chisel -o scene.glsl  # write to file
  chisel check scene.chisel                  # check for errors
  chisel fmt scene.chisel                    # format in place
  cat scene.chisel | chisel fmt -            # format stdin
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}

func run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 1
	}

	cmd := args[0]
	if cmd == "-h" || cmd == "--help" || cmd == "help" {
		fmt.Fprint(stdout, usage)
		return 0
	}

	switch cmd {
	case "compile":
		return cmdCompile(args[1:], stdout, stderr)
	case "check":
		return cmdCheck(args[1:], stdout, stderr)
	case "fmt":
		return cmdFmt(args[1:], stdout, stderr, stdin)
	default:
		fmt.Fprintf(stderr, "chisel: unknown command %q\n\n", cmd)
		fmt.Fprint(stderr, usage)
		return 1
	}
}

// ---------------------------------------------------------------------------
// Flag parsing
// ---------------------------------------------------------------------------

type flags struct {
	output  string
	noColor bool
	astFlag bool
	tokens  bool
	file    string
}

func parseFlags(args []string) (flags, error) {
	var f flags
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-o":
			if i+1 >= len(args) {
				return f, fmt.Errorf("-o requires a filename argument")
			}
			i++
			f.output = args[i]
		case a == "-no-color":
			f.noColor = true
		case a == "-ast":
			f.astFlag = true
		case a == "-tokens":
			f.tokens = true
		case a == "-":
			if f.file != "" {
				return f, fmt.Errorf("unexpected argument %q", a)
			}
			f.file = a
		case strings.HasPrefix(a, "-"):
			return f, fmt.Errorf("unknown flag %q", a)
		default:
			if f.file != "" {
				return f, fmt.Errorf("unexpected argument %q", a)
			}
			f.file = a
		}
	}
	return f, nil
}

func useColor(f flags, w io.Writer) bool {
	if f.noColor {
		return false
	}
	if file, ok := w.(*os.File); ok {
		fi, err := file.Stat()
		if err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			return true
		}
	}
	return false
}

func writeOutput(f flags, stdout io.Writer, data string) error {
	if f.output != "" {
		return os.WriteFile(f.output, []byte(data), 0644)
	}
	_, err := fmt.Fprint(stdout, data)
	return err
}

func printDiags(source string, diags []diagnostic.Diagnostic, stderr io.Writer, color bool) bool {
	hasErrors := false
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			hasErrors = true
		}
		fmt.Fprint(stderr, diagnostic.Render(source, d, color))
	}
	return hasErrors
}

// ---------------------------------------------------------------------------
// compile
// ---------------------------------------------------------------------------

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
	color := useColor(f, stderr)
	src := string(source)

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

// ---------------------------------------------------------------------------
// check
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// fmt
// ---------------------------------------------------------------------------

func cmdFmt(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	f, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}

	if f.file == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "chisel fmt: reading stdin: %s\n", err)
			return 1
		}
		formatted, err := format.Format(string(data))
		if err != nil {
			fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
			return 1
		}
		return writeOrFail(f, stdout, stderr, formatted, "fmt")
	}

	if f.file == "" {
		fmt.Fprintln(stderr, "chisel fmt: missing file argument (use - for stdin)")
		return 1
	}

	source, err := os.ReadFile(f.file)
	if err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}

	formatted, err := format.Format(string(source))
	if err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}

	target := f.file
	if f.output != "" {
		target = f.output
	}
	if err := os.WriteFile(target, []byte(formatted), 0644); err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func hasError(diags []diagnostic.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}

func writeOrFail(f flags, stdout, stderr io.Writer, data, cmd string) int {
	if err := writeOutput(f, stdout, data); err != nil {
		fmt.Fprintf(stderr, "chisel %s: %s\n", cmd, err)
		return 1
	}
	return 0
}
