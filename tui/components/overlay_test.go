package components

import (
	"strings"
	"testing"
)

func TestVisibleWidth(t *testing.T) {
	tests := []struct {
		input    string
		expected int
	}{
		{"hello", 5},
		{"", 0},
		{"\x1b[31mred\x1b[0m", 3},
		{"\x1b[38;2;255;0;0m█\x1b[0m", 1},
		{"no ansi", 7},
	}
	for _, tc := range tests {
		got := visibleWidth(tc.input)
		if got != tc.expected {
			t.Errorf("visibleWidth(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}
}

func TestSpliceLine(t *testing.T) {
	bg := "ABCDEFGHIJ"
	overlay := "XY"
	result := spliceLine(bg, 3, overlay, 2)
	stripped := StripANSI(result)
	if stripped != "ABCXYFGHIJ" {
		t.Errorf("expected 'ABCXYFGHIJ', got %q", stripped)
	}
}

func TestSpliceLineWithANSI(t *testing.T) {
	bg := "\x1b[31mABCDE\x1b[0m     "
	overlay := "XY"
	result := spliceLine(bg, 3, overlay, 2)
	stripped := StripANSI(result)
	if stripped != "ABCXY     " {
		t.Errorf("expected 'ABCXY     ', got %q", stripped)
	}
}

func TestOverlayPanel(t *testing.T) {
	bg := strings.Join([]string{
		"AAAAAAAAAA",
		"BBBBBBBBBB",
		"CCCCCCCCCC",
		"DDDDDDDDDD",
		"EEEEEEEEEE",
	}, "\n")

	panel := "XX\nYY"

	result := OverlayPanel(bg, panel, 1, 4)
	lines := strings.Split(result, "\n")

	s1 := StripANSI(lines[1])
	if s1 != "BBBBXXBBBB" {
		t.Errorf("line 1: expected 'BBBBXXBBBB', got %q", s1)
	}
	s2 := StripANSI(lines[2])
	if s2 != "CCCCYYCCCC" {
		t.Errorf("line 2: expected 'CCCCYYCCCC', got %q", s2)
	}
	// Untouched lines
	if StripANSI(lines[0]) != "AAAAAAAAAA" {
		t.Errorf("line 0 should be untouched, got %q", StripANSI(lines[0]))
	}
}

func TestOverlayCentered(t *testing.T) {
	bg := strings.Repeat("          \n", 5)
	bg = strings.TrimSuffix(bg, "\n")

	panel := "XX\nYY"
	result := OverlayCentered(bg, panel, 10, 5)
	lines := strings.Split(result, "\n")

	// Panel is 2x2, bg is 10x5, so it should be around row 1-2, col 4
	// Just verify the panel content appears
	found := false
	for _, l := range lines {
		if strings.Contains(StripANSI(l), "XX") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected panel content 'XX' in overlay result")
	}
}
