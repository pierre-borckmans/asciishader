// Command chisel is the CLI for the Chisel language compiler.
// It can compile .chisel files to GLSL, check for errors, and format source code.
package main

import (
	"fmt"
	"io"
	"os"
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
