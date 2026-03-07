package main

import (
	"fmt"
	"io"
	"os"

	"asciishader/pkg/chisel/format"
)

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
