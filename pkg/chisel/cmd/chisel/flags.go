package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"asciishader/pkg/chisel/compiler/diagnostic"
)

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
