// Command chisel is the CLI for the Chisel language compiler.
// It can compile .chisel files to GLSL, check for errors, and format source code.
package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"asciishader/pkg/chisel"
	"asciishader/pkg/chisel/analyzer"
	"asciishader/pkg/chisel/ast"
	"asciishader/pkg/chisel/diagnostic"
	"asciishader/pkg/chisel/format"
	"asciishader/pkg/chisel/lexer"
	"asciishader/pkg/chisel/parser"
	"asciishader/pkg/chisel/token"
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

	rest := args[1:]

	switch cmd {
	case "compile":
		return cmdCompile(rest, stdout, stderr)
	case "check":
		return cmdCheck(rest, stdout, stderr)
	case "fmt":
		return cmdFmt(rest, stdout, stderr, stdin)
	default:
		fmt.Fprintf(stderr, "chisel: unknown command %q\n\n", cmd)
		fmt.Fprint(stderr, usage)
		return 1
	}
}

// ---------------------------------------------------------------------------
// Flag parsing helpers
// ---------------------------------------------------------------------------

type flags struct {
	output  string // -o value
	noColor bool   // -no-color
	astFlag bool   // -ast
	tokens  bool   // -tokens
	file    string // positional file argument
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
			// Bare "-" means stdin; treat as a file argument.
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

// useColor returns true if colored output should be used.
func useColor(f flags, w io.Writer) bool {
	if f.noColor {
		return false
	}
	// Check if the writer is a terminal.
	if file, ok := w.(*os.File); ok {
		fi, err := file.Stat()
		if err == nil && (fi.Mode()&os.ModeCharDevice) != 0 {
			return true
		}
	}
	return false
}

// writeOutput writes data to the -o file if specified, otherwise to stdout.
func writeOutput(f flags, stdout io.Writer, data string) error {
	if f.output != "" {
		return os.WriteFile(f.output, []byte(data), 0644)
	}
	_, err := fmt.Fprint(stdout, data)
	return err
}

// readSourceFile reads a source file and returns its contents.
func readSourceFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// printDiags renders diagnostics to stderr and returns true if any are errors.
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
// compile command
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

	source, err := readSourceFile(f.file)
	if err != nil {
		fmt.Fprintf(stderr, "chisel compile: %s\n", err)
		return 1
	}
	color := useColor(f, stderr)

	// If -tokens flag, print the token stream and exit.
	if f.tokens {
		tokens, lexDiags := lexer.Lex(f.file, source)
		if printDiags(source, lexDiags, stderr, color) {
			return 1
		}
		var sb strings.Builder
		for _, tok := range tokens {
			if tok.Kind == token.TokEOF {
				break
			}
			fmt.Fprintf(&sb, "%d:%d %s %q\n", tok.Pos.Line, tok.Pos.Col, tok.Kind, tok.Value)
		}
		if err := writeOutput(f, stdout, sb.String()); err != nil {
			fmt.Fprintf(stderr, "chisel compile: %s\n", err)
			return 1
		}
		return 0
	}

	// If -ast flag, lex + parse + print AST.
	if f.astFlag {
		tokens, lexDiags := lexer.Lex(f.file, source)
		if printDiags(source, lexDiags, stderr, color) {
			return 1
		}
		prog, parseDiags := parser.Parse(tokens)
		if printDiags(source, parseDiags, stderr, color) {
			return 1
		}
		var sb strings.Builder
		printAST(&sb, prog, 0)
		if err := writeOutput(f, stdout, sb.String()); err != nil {
			fmt.Fprintf(stderr, "chisel compile: %s\n", err)
			return 1
		}
		return 0
	}

	// Normal compile: produce GLSL.
	glsl, diags := chisel.Compile(source)
	if printDiags(source, diags, stderr, color) {
		return 1
	}
	if err := writeOutput(f, stdout, glsl); err != nil {
		fmt.Fprintf(stderr, "chisel compile: %s\n", err)
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// check command
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

	source, err := readSourceFile(f.file)
	if err != nil {
		fmt.Fprintf(stderr, "chisel check: %s\n", err)
		return 1
	}
	color := useColor(f, stderr)

	// Lex.
	tokens, lexDiags := lexer.Lex(f.file, source)
	allDiags := lexDiags

	hasLexErrors := false
	for _, d := range lexDiags {
		if d.Severity == diagnostic.Error {
			hasLexErrors = true
		}
	}

	if !hasLexErrors {
		// Parse.
		prog, parseDiags := parser.Parse(tokens)
		allDiags = append(allDiags, parseDiags...)

		hasParseErrors := false
		for _, d := range parseDiags {
			if d.Severity == diagnostic.Error {
				hasParseErrors = true
			}
		}

		if !hasParseErrors {
			// Analyze.
			analyzeDiags := analyzer.Analyze(prog)
			allDiags = append(allDiags, analyzeDiags...)
		}
	}

	hasErrors := printDiags(source, allDiags, stderr, color)

	if !hasErrors {
		fmt.Fprintf(stdout, "%s: ok\n", f.file)
	}

	if hasErrors {
		return 1
	}
	return 0
}

// ---------------------------------------------------------------------------
// fmt command
// ---------------------------------------------------------------------------

func cmdFmt(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	f, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}

	// Reading from stdin.
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
		if err := writeOutput(f, stdout, formatted); err != nil {
			fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
			return 1
		}
		return 0
	}

	if f.file == "" {
		fmt.Fprintln(stderr, "chisel fmt: missing file argument (use - for stdin)")
		return 1
	}

	source, err := readSourceFile(f.file)
	if err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}

	formatted, err := format.Format(source)
	if err != nil {
		fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
		return 1
	}

	// If -o flag is set, write there. Otherwise overwrite the original file.
	if f.output != "" {
		if err := os.WriteFile(f.output, []byte(formatted), 0644); err != nil {
			fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
			return 1
		}
	} else {
		if err := os.WriteFile(f.file, []byte(formatted), 0644); err != nil {
			fmt.Fprintf(stderr, "chisel fmt: %s\n", err)
			return 1
		}
	}

	return 0
}

// ---------------------------------------------------------------------------
// AST pretty printer
// ---------------------------------------------------------------------------

func printAST(w *strings.Builder, node ast.Node, depth int) {
	indent := strings.Repeat("  ", depth)

	switch n := node.(type) {
	case *ast.Program:
		w.WriteString(indent)
		w.WriteString("Program\n")
		for _, s := range n.Statements {
			printAST(w, s, depth+1)
		}

	case *ast.AssignStmt:
		w.WriteString(indent)
		w.WriteString("Assign ")
		w.WriteString(n.Name)
		if n.Params != nil {
			w.WriteString("(")
			for i, p := range n.Params {
				if i > 0 {
					w.WriteString(", ")
				}
				w.WriteString(p.Name)
				if p.Default != nil {
					w.WriteString("=...")
				}
			}
			w.WriteString(")")
		}
		w.WriteString("\n")
		printASTExpr(w, n.Value, depth+1)

	case *ast.ExprStmt:
		w.WriteString(indent)
		w.WriteString("ExprStmt\n")
		printASTExpr(w, n.Expression, depth+1)

	case *ast.SettingStmt:
		w.WriteString(indent)
		fmt.Fprintf(w, "Setting %s\n", n.Kind)
	}
}

func printASTExpr(w *strings.Builder, expr ast.Expr, depth int) {
	if expr == nil {
		return
	}
	indent := strings.Repeat("  ", depth)

	switch e := expr.(type) {
	case *ast.NumberLit:
		fmt.Fprintf(w, "%sNumber(%g)\n", indent, e.Value)

	case *ast.BoolLit:
		fmt.Fprintf(w, "%sBool(%t)\n", indent, e.Value)

	case *ast.StringLit:
		fmt.Fprintf(w, "%sString(%q)\n", indent, e.Value)

	case *ast.HexColorLit:
		fmt.Fprintf(w, "%sHexColor(%.2f, %.2f, %.2f, %.2f)\n", indent, e.R, e.G, e.B, e.A)

	case *ast.Ident:
		fmt.Fprintf(w, "%sIdent(%s)\n", indent, e.Name)

	case *ast.VecLit:
		fmt.Fprintf(w, "%sVec[\n", indent)
		for _, elem := range e.Elems {
			printASTExpr(w, elem, depth+1)
		}
		fmt.Fprintf(w, "%s]\n", indent)

	case *ast.BinaryExpr:
		fmt.Fprintf(w, "%sBinary(%s)\n", indent, e.Op)
		printASTExpr(w, e.Left, depth+1)
		printASTExpr(w, e.Right, depth+1)

	case *ast.UnaryExpr:
		fmt.Fprintf(w, "%sUnary(%s)\n", indent, e.Op)
		printASTExpr(w, e.Operand, depth+1)

	case *ast.FuncCall:
		fmt.Fprintf(w, "%sCall(%s)\n", indent, e.Name)
		for _, a := range e.Args {
			if a.Name != "" {
				fmt.Fprintf(w, "%s  %s:\n", indent, a.Name)
				printASTExpr(w, a.Value, depth+2)
			} else {
				printASTExpr(w, a.Value, depth+1)
			}
		}

	case *ast.MethodCall:
		fmt.Fprintf(w, "%sMethod(.%s)\n", indent, e.Name)
		printASTExpr(w, e.Receiver, depth+1)
		for _, a := range e.Args {
			if a.Name != "" {
				fmt.Fprintf(w, "%s  %s:\n", indent, a.Name)
				printASTExpr(w, a.Value, depth+2)
			} else {
				printASTExpr(w, a.Value, depth+1)
			}
		}

	case *ast.Swizzle:
		fmt.Fprintf(w, "%sSwizzle(.%s)\n", indent, e.Components)
		printASTExpr(w, e.Receiver, depth+1)

	case *ast.Block:
		fmt.Fprintf(w, "%sBlock\n", indent)
		for _, s := range e.Stmts {
			printAST(w, s, depth+1)
		}
		if e.Result != nil {
			fmt.Fprintf(w, "%s  result:\n", indent)
			printASTExpr(w, e.Result, depth+2)
		}

	case *ast.ForExpr:
		fmt.Fprintf(w, "%sFor\n", indent)
		for _, it := range e.Iterators {
			fmt.Fprintf(w, "%s  %s in\n", indent, it.Name)
			printASTExpr(w, it.Start, depth+2)
			fmt.Fprintf(w, "%s  ..\n", indent)
			printASTExpr(w, it.End, depth+2)
			if it.Step != nil {
				fmt.Fprintf(w, "%s  step\n", indent)
				printASTExpr(w, it.Step, depth+2)
			}
		}
		if e.Body != nil {
			printASTExpr(w, e.Body, depth+1)
		}

	case *ast.IfExpr:
		fmt.Fprintf(w, "%sIf\n", indent)
		fmt.Fprintf(w, "%s  cond:\n", indent)
		printASTExpr(w, e.Cond, depth+2)
		if e.Then != nil {
			fmt.Fprintf(w, "%s  then:\n", indent)
			printASTExpr(w, e.Then, depth+2)
		}
		if e.Else != nil {
			fmt.Fprintf(w, "%s  else:\n", indent)
			printASTExpr(w, e.Else, depth+2)
		}

	case *ast.GlslEscape:
		fmt.Fprintf(w, "%sGlslEscape(%s) { %s }\n", indent, e.Param, e.Code)
	}
}
