package main

import (
	"fmt"
	"strings"

	"asciishader/components"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// EditorTab wraps a textarea for editing GLSL code with compile support.
// The textarea is set to a very large internal width (no wrapping) and its
// height matches the total line count (no internal scrolling). The
// ScrollableView handles both vertical scrolling + scrollbar and horizontal
// truncation.
type EditorTab struct {
	textarea   textarea.Model
	scrollView *components.ScrollableView
	status     string // status bar text
	statusErr  bool   // true if status is an error
	width      int
	height     int
}

// NewEditorTab creates a new GLSL code editor.
func NewEditorTab() *EditorTab {
	ta := textarea.New()
	ta.SetValue(defaultUserCode)
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	// Very large internal width so the textarea never wraps lines.
	ta.SetWidth(10000)
	// Set height to total lines so the textarea never scrolls internally.
	ta.SetHeight(len(strings.Split(defaultUserCode, "\n")) + 1)
	ta.Focus()

	sv := components.NewScrollableView()
	sv.SetTruncate(true)

	return &EditorTab{
		textarea:   ta,
		scrollView: sv,
		status:     "Ctrl+R: compile",
	}
}

// SetSize updates the editor dimensions.
func (et *EditorTab) SetSize(width, height int) {
	et.width = width
	et.height = height
	// Reserve 1 line for status bar
	visibleHeight := height - 1
	if visibleHeight < 3 {
		visibleHeight = 3
	}
	// Textarea gets full content height (no internal scrolling)
	et.syncTextareaHeight()
	// ScrollableView clips to visible area
	et.scrollView.SetSize(width, visibleHeight)
}

// syncTextareaHeight sets the textarea height to match its content.
func (et *EditorTab) syncTextareaHeight() {
	totalLines := len(strings.Split(et.textarea.Value(), "\n")) + 1
	if et.textarea.Height() != totalLines {
		et.textarea.SetHeight(totalLines)
	}
}

// SetCode replaces the editor content with new code.
func (et *EditorTab) SetCode(code string) {
	et.textarea.SetValue(code)
	et.syncTextareaHeight()
	et.status = "Ctrl+R: compile"
	et.statusErr = false
}

// Code returns the current editor content.
func (et *EditorTab) Code() string {
	return et.textarea.Value()
}

// Focus gives keyboard focus to the textarea.
func (et *EditorTab) Focus() {
	et.textarea.Focus()
}

// Blur removes keyboard focus from the textarea.
func (et *EditorTab) Blur() {
	et.textarea.Blur()
}

// Compile attempts to compile the current code using the GPU renderer.
func (et *EditorTab) Compile(gpu *GPURenderer) {
	if gpu == nil {
		et.status = "No GPU renderer"
		et.statusErr = true
		return
	}

	code := et.textarea.Value()
	err := gpu.CompileUserCode(code)
	if err != nil {
		errMsg := err.Error()
		prefixLines := PrefixLineCount(code)
		errMsg = adjustErrorLineNumbers(errMsg, prefixLines)
		et.status = fmt.Sprintf("Error: %s", errMsg)
		et.statusErr = true
	} else {
		et.status = "Compiled OK"
		et.statusErr = false
	}
}

// Update processes a bubbletea message. Returns (handled, cmd).
func (et *EditorTab) Update(msg tea.Msg) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		if keyMsg.String() == "ctrl+r" {
			return true, nil // caller handles compile
		}
	}
	var cmd tea.Cmd
	et.textarea, cmd = et.textarea.Update(msg)
	// Keep textarea height in sync with content (lines may have been added/removed)
	et.syncTextareaHeight()
	// Keep cursor visible in the scroll view (vertical + horizontal)
	et.scrollView.EnsureLineVisible(et.textarea.Line())
	lineInfo := et.textarea.LineInfo()
	// Cursor visual column = line number gutter width + column offset
	lineCount := len(strings.Split(et.textarea.Value(), "\n"))
	gutterWidth := len(fmt.Sprintf(" %d ", lineCount)) + 1 // matches textarea's " N│ " format
	et.scrollView.EnsureColumnVisible(gutterWidth + lineInfo.ColumnOffset)
	return true, cmd
}

// Render returns the editor view.
func (et *EditorTab) Render(width int) string {
	var sb strings.Builder

	// The textarea renders ALL lines (no internal scrolling).
	// Strip trailing padding from the 10000-width hack so ScrollableView
	// sees real content widths. On the cursor line, keep one extra char
	// so the cursor block (a styled trailing space) isn't stripped.
	rawView := et.textarea.View()
	rawLines := strings.Split(rawView, "\n")
	cursorLine := et.textarea.Line()
	// Figure out the cursor's byte position within the rendered line so we
	// can trim padding without losing the cursor character.  The textarea
	// renders: gutter (prompt+line number) + content + cursor + padding.
	// After stripANSI + TrimRight, we get gutter+content. We need 1 more
	// byte-position for the cursor block; on empty lines, TrimRight also
	// removes the separator space, so we need 2.
	var cursorKeepExtra int
	if cursorLine < len(rawLines) {
		contentLines := strings.Split(et.textarea.Value(), "\n")
		if cursorLine < len(contentLines) && len(contentLines[cursorLine]) == 0 {
			cursorKeepExtra = 2 // separator space + cursor
		} else {
			cursorKeepExtra = 1 // just cursor
		}
	}
	for i, line := range rawLines {
		stripped := components.StripANSI(line)
		trimmed := strings.TrimRight(stripped, " ")
		keepWidth := len(trimmed)
		if i == cursorLine {
			keepWidth += cursorKeepExtra
		}
		if keepWidth < len(stripped) {
			// Reset ANSI at end so CursorLine background doesn't bleed
			rawLines[i] = truncateLineAt(line, keepWidth) + "\x1b[0m"
		}
	}
	trimmedView := strings.Join(rawLines, "\n")

	// ScrollableView handles vertical scrolling + scrollbar + horizontal truncation.
	scrolled := et.scrollView.RenderContent(trimmedView)
	sb.WriteString(scrolled)
	sb.WriteString("\n")

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236"))
	if et.statusErr {
		statusStyle = statusStyle.Foreground(lipgloss.Color("196"))
	}

	statusLine := " " + et.status
	for len(statusLine) < width {
		statusLine += " "
	}
	if len(statusLine) > width {
		statusLine = statusLine[:width]
	}
	sb.WriteString(statusStyle.Render(statusLine))

	return sb.String()
}

// truncateLineAt truncates a styled string to show only the first n visible
// characters, preserving ANSI escape sequences.
func truncateLineAt(s string, n int) string {
	if n <= 0 {
		return ""
	}
	var result strings.Builder
	visible := 0
	i := 0
	for i < len(s) && visible < n {
		if s[i] == '\x1b' {
			start := i
			i++
			for i < len(s) && !((s[i] >= 'a' && s[i] <= 'z') || (s[i] >= 'A' && s[i] <= 'Z')) {
				i++
			}
			if i < len(s) {
				i++
			}
			result.WriteString(s[start:i])
		} else {
			result.WriteByte(s[i])
			visible++
			i++
		}
	}
	return result.String()
}

// adjustErrorLineNumbers subtracts prefixLines from line numbers in GLSL error messages.
func adjustErrorLineNumbers(errMsg string, prefixLines int) string {
	lines := strings.Split(errMsg, "\n")
	for i, line := range lines {
		lines[i] = adjustLineInError(line, prefixLines)
	}
	return strings.Join(lines, "\n")
}

func adjustLineInError(line string, prefixLines int) string {
	for _, prefix := range []string{"0:", "ERROR: 0:"} {
		idx := strings.Index(line, prefix)
		if idx < 0 {
			continue
		}
		rest := line[idx+len(prefix):]
		numEnd := 0
		for numEnd < len(rest) && rest[numEnd] >= '0' && rest[numEnd] <= '9' {
			numEnd++
		}
		if numEnd == 0 {
			continue
		}
		num := 0
		for _, c := range rest[:numEnd] {
			num = num*10 + int(c-'0')
		}
		adjusted := num - prefixLines
		if adjusted < 1 {
			adjusted = 1
		}
		return line[:idx+len(prefix)] + fmt.Sprintf("%d", adjusted) + rest[numEnd:]
	}
	return line
}
