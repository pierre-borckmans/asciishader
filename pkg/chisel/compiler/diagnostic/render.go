package diagnostic

import (
	"fmt"
	"strings"
)

// Render produces a Rust/Elm-style diagnostic message from source code and a
// Diagnostic. When useColor is true, ANSI escape codes are used for severity
// and line-number coloring.
//
// Example output:
//
//	error: unexpected token '+'
//	  ┌─ scene.chisel:3:8
//	  │
//	3 │ sphere + box
//	  │        ^ did you mean '|' for union?
//	  │
//	  = help: '+' is arithmetic only. Use '|' for combining shapes.
func Render(source string, diag Diagnostic, useColor bool) string {
	const (
		colorRed    = "\033[31m"
		colorYellow = "\033[33m"
		colorBlue   = "\033[34m"
		colorReset  = "\033[0m"
	)

	// Choose severity color.
	var sevColor string
	if useColor {
		switch diag.Severity {
		case Error:
			sevColor = colorRed
		case Warning:
			sevColor = colorYellow
		case Hint:
			sevColor = colorBlue
		}
	}

	// Helper to colorize text.
	colorize := func(color, text string) string {
		if !useColor || color == "" {
			return text
		}
		return color + text + colorReset
	}

	// Split source into lines.
	lines := strings.Split(source, "\n")

	// Determine the line number gutter width: the widest line number we'll
	// display. We need at least enough width for the primary span's line(s)
	// and any label lines.
	maxLine := diag.Span.Start.Line
	if diag.Span.End.Line > maxLine {
		maxLine = diag.Span.End.Line
	}
	for _, lbl := range diag.Labels {
		if lbl.Span.Start.Line > maxLine {
			maxLine = lbl.Span.Start.Line
		}
		if lbl.Span.End.Line > maxLine {
			maxLine = lbl.Span.End.Line
		}
	}
	gutterWidth := len(fmt.Sprintf("%d", maxLine))
	if gutterWidth < 1 {
		gutterWidth = 1
	}

	// Blank gutter: same width as line numbers, filled with spaces.
	blankGutter := strings.Repeat(" ", gutterWidth)

	var b strings.Builder

	// Line 1: severity + message.
	b.WriteString(colorize(sevColor, diag.Severity.String()))
	b.WriteString(": ")
	b.WriteString(diag.Message)
	b.WriteByte('\n')

	// Line 2: location header  "  ┌─ file:line:col"
	b.WriteString(blankGutter)
	b.WriteString(" ┌─ ")
	b.WriteString(diag.Span.Start.String())
	b.WriteByte('\n')

	// Line 3: blank separator  "  │"
	writeBlankBar(&b, blankGutter)

	// Render the primary span source line(s) with underline carets.
	startLine := diag.Span.Start.Line
	endLine := diag.Span.End.Line
	multiLine := startLine != endLine && startLine > 0 && endLine > 0

	if startLine > 0 && startLine <= len(lines) {
		if !multiLine {
			// Single-line span.
			writeSourceLine(&b, lines, startLine, gutterWidth, useColor, colorBlue, colorReset)
			writeCarets(&b, blankGutter, diag.Span.Start.Col, diag.Span.End.Col, diag.Message, useColor, sevColor, colorReset)
		} else {
			// Multi-line span: show first line with carets to end of line.
			writeSourceLine(&b, lines, startLine, gutterWidth, useColor, colorBlue, colorReset)
			firstLineEnd := len(lines[startLine-1]) + 1
			writeCarets(&b, blankGutter, diag.Span.Start.Col, firstLineEnd, "", useColor, sevColor, colorReset)

			// Ellipsis if there are skipped lines in between.
			if endLine-startLine > 1 {
				b.WriteString(blankGutter)
				b.WriteString(" │ ...\n")
			}

			// Show last line with carets from column 1 to End.Col.
			if endLine <= len(lines) {
				writeSourceLine(&b, lines, endLine, gutterWidth, useColor, colorBlue, colorReset)
				writeCarets(&b, blankGutter, 1, diag.Span.End.Col, diag.Message, useColor, sevColor, colorReset)
			}
		}
	} else {
		// Diagnostic beyond source bounds (e.g. end-of-file): show an
		// empty source line and a single caret.
		lineNum := startLine
		if lineNum < 1 {
			lineNum = 1
		}
		lineNumStr := fmt.Sprintf("%*d", gutterWidth, lineNum)
		if useColor {
			lineNumStr = colorBlue + lineNumStr + colorReset
		}
		b.WriteString(lineNumStr)
		b.WriteString(" │ \n")
		writeCarets(&b, blankGutter, 1, 2, diag.Message, useColor, sevColor, colorReset)
	}

	// Render additional labels on different spans.
	for _, lbl := range diag.Labels {
		lblLine := lbl.Span.Start.Line
		if lblLine > 0 && lblLine <= len(lines) {
			writeBlankBar(&b, blankGutter)
			writeSourceLine(&b, lines, lblLine, gutterWidth, useColor, colorBlue, colorReset)
			writeDashes(&b, blankGutter, lbl.Span.Start.Col, lbl.Span.End.Col, lbl.Message, useColor, colorBlue, colorReset)
		}
	}

	// Blank separator.
	writeBlankBar(&b, blankGutter)

	// Help text.
	if diag.Help != "" {
		b.WriteString(blankGutter)
		b.WriteString(" = help: ")
		b.WriteString(diag.Help)
		b.WriteByte('\n')
	}

	return b.String()
}

// writeBlankBar writes a gutter-only line: "  │\n".
func writeBlankBar(b *strings.Builder, blankGutter string) {
	b.WriteString(blankGutter)
	b.WriteString(" │\n")
}

// writeSourceLine writes "N │ <source text>\n" with the line number right-aligned.
func writeSourceLine(b *strings.Builder, lines []string, lineNum, gutterWidth int, useColor bool, colorBlue, colorReset string) {
	numStr := fmt.Sprintf("%*d", gutterWidth, lineNum)
	if useColor {
		numStr = colorBlue + numStr + colorReset
	}
	text := ""
	if lineNum >= 1 && lineNum <= len(lines) {
		text = lines[lineNum-1]
	}
	b.WriteString(numStr)
	b.WriteString(" │ ")
	b.WriteString(text)
	b.WriteByte('\n')
}

// writeCarets writes "  │ <padding>^^^^ <message>\n".
// startCol is 1-based inclusive, endCol is 1-based exclusive.
func writeCarets(b *strings.Builder, blankGutter string, startCol, endCol int, message string, useColor bool, sevColor, colorReset string) {
	n := endCol - startCol
	if n < 1 {
		n = 1
	}
	padding := strings.Repeat(" ", startCol-1)
	carets := strings.Repeat("^", n)
	if useColor && sevColor != "" {
		carets = sevColor + carets + colorReset
	}

	b.WriteString(blankGutter)
	b.WriteString(" │ ")
	b.WriteString(padding)
	b.WriteString(carets)
	if message != "" {
		b.WriteByte(' ')
		b.WriteString(message)
	}
	b.WriteByte('\n')
}

// writeDashes writes "  │ <padding>------ <message>\n" for secondary labels.
func writeDashes(b *strings.Builder, blankGutter string, startCol, endCol int, message string, useColor bool, colorBlue, colorReset string) {
	n := endCol - startCol
	if n < 1 {
		n = 1
	}
	padding := strings.Repeat(" ", startCol-1)
	dashes := strings.Repeat("-", n)
	if useColor && colorBlue != "" {
		dashes = colorBlue + dashes + colorReset
	}

	b.WriteString(blankGutter)
	b.WriteString(" │ ")
	b.WriteString(padding)
	b.WriteString(dashes)
	if message != "" {
		b.WriteByte(' ')
		b.WriteString(message)
	}
	b.WriteByte('\n')
}
