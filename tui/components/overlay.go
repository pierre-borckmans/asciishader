package components

import (
	"strings"
)

// OverlayPanel places a multi-line panel on top of a background string at the
// given (startRow, startCol) position. Both panel and background are expected
// to be newline-separated strings. ANSI sequences in the background are
// preserved outside the overlaid region.
func OverlayPanel(background, panel string, startRow, startCol int) string {
	bgLines := strings.Split(background, "\n")
	panelLines := strings.Split(panel, "\n")

	for i, pLine := range panelLines {
		bgIdx := startRow + i
		if bgIdx < 0 || bgIdx >= len(bgLines) {
			continue
		}
		pWidth := visibleWidth(pLine)
		bgLine := bgLines[bgIdx]

		// Ensure background line is wide enough
		bgW := visibleWidth(bgLine)
		if bgW < startCol+pWidth {
			bgLine += strings.Repeat(" ", startCol+pWidth-bgW)
		}

		bgLines[bgIdx] = spliceLine(bgLine, startCol, pLine, pWidth)
	}

	return strings.Join(bgLines, "\n")
}

// OverlayCentered places a panel centered on the background.
func OverlayCentered(background, panel string, bgWidth, bgHeight int) string {
	panelLines := strings.Split(panel, "\n")
	panelH := len(panelLines)
	panelW := 0
	for _, l := range panelLines {
		if w := visibleWidth(l); w > panelW {
			panelW = w
		}
	}

	startRow := (bgHeight - panelH) / 2
	startCol := (bgWidth - panelW) / 2
	if startRow < 0 {
		startRow = 0
	}
	if startCol < 0 {
		startCol = 0
	}

	return OverlayPanel(background, panel, startRow, startCol)
}

// visibleWidth returns the display width of a string, skipping ANSI escapes.
// Assumes single-width characters for ASCII; uses rune counting for non-ASCII.
func visibleWidth(s string) int {
	w := 0
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			continue
		}
		w++
	}
	return w
}

// spliceLine overwrites `width` visible characters in `line` starting at
// visible column `col` with `rendered`, preserving ANSI sequences outside the
// splice region. After the splice, the ANSI styling context from the original
// line is restored so subsequent characters render correctly.
func spliceLine(line string, col int, rendered string, width int) string {
	var before strings.Builder
	var after strings.Builder

	runes := []rune(line)
	i := 0
	visiblePos := 0
	inEscape := false

	// Collect everything before col (including ANSI codes)
	for i < len(runes) && visiblePos < col {
		r := runes[i]
		if r == '\x1b' {
			inEscape = true
			before.WriteRune(r)
			i++
			continue
		}
		if inEscape {
			before.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			i++
			continue
		}
		before.WriteRune(r)
		visiblePos++
		i++
	}

	// Skip over characters that the overlay covers
	skipped := 0
	for i < len(runes) && skipped < width {
		r := runes[i]
		if r == '\x1b' {
			inEscape = true
			i++
			continue
		}
		if inEscape {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
			i++
			continue
		}
		skipped++
		i++
	}

	// Collect everything after the splice region
	for i < len(runes) {
		after.WriteRune(runes[i])
		i++
	}

	// Restore the ANSI context that was active at the splice boundary so
	// characters after the overlay render with the correct styling.
	restoreSeq := lastANSISequences(line, col+width)

	return before.String() + "\x1b[0m" + rendered + "\x1b[0m" + restoreSeq + after.String()
}

// lastANSISequences collects all ANSI escape sequences from the line up to the
// given visible column. After a \x1b[0m reset, previously accumulated sequences
// are discarded. This lets us restore the styling context after splicing in an
// overlay.
func lastANSISequences(line string, upToCol int) string {
	var sequences []string
	visiblePos := 0
	inEscape := false
	var currentSeq strings.Builder

	for _, r := range line {
		if visiblePos > upToCol {
			break
		}
		if r == '\x1b' {
			inEscape = true
			currentSeq.Reset()
			currentSeq.WriteRune(r)
			continue
		}
		if inEscape {
			currentSeq.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
				seq := currentSeq.String()
				if seq == "\x1b[0m" {
					sequences = nil
				} else {
					sequences = append(sequences, seq)
				}
			}
			continue
		}
		visiblePos++
	}

	return strings.Join(sequences, "")
}
