package editor

import (
	"fmt"
	"strings"

	"asciishader/pkg/chisel"
	"asciishader/pkg/chisel/compiler/diagnostic"
	gpupkg "asciishader/pkg/gpu"
	"asciishader/pkg/shader"
	"asciishader/tui/components"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

// EditorTab wraps a textarea for editing GLSL code with compile support.
// The textarea is set to a very large internal width (no wrapping) and its
// height matches the total line count (no internal scrolling). The
// ScrollableView handles both vertical scrolling + scrollbar and horizontal
// truncation.
type EditorTab struct {
	textarea   textarea.Model
	ScrollView *components.ScrollableView
	Status     string // status bar text
	StatusErr  bool   // true if status is an error
	ChiselMode bool   // true when editing .chisel code
	width      int
	height     int
}

// NewEditorTab creates a new GLSL code editor.
func NewEditorTab() *EditorTab {
	ta := textarea.New()
	ta.SetValue(shader.DefaultUserCode)
	ta.ShowLineNumbers = true
	ta.CharLimit = 0
	// Very large internal width so the textarea never wraps lines.
	ta.SetWidth(10000)
	// Set height to total lines so the textarea never scrolls internally.
	ta.SetHeight(len(strings.Split(shader.DefaultUserCode, "\n")) + 1)
	ta.Focus()

	sv := components.NewScrollableView()
	sv.SetTruncate(true)

	return &EditorTab{
		textarea:   ta,
		ScrollView: sv,
		Status:     "Ctrl+R: compile",
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
	et.ScrollView.SetSize(width, visibleHeight)
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
	et.Status = "Ctrl+R: compile"
	et.StatusErr = false
}

// SetChiselCode replaces the editor content with Chisel code.
func (et *EditorTab) SetChiselCode(code string) {
	et.ChiselMode = true
	et.SetCode(code)
	et.Status = "Ctrl+R: compile (Chisel)"
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
// If ChiselMode is true, the code is first compiled from Chisel to GLSL.
func (et *EditorTab) Compile(gpu *gpupkg.GPURenderer) {
	if gpu == nil {
		et.Status = "No GPU renderer"
		et.StatusErr = true
		return
	}

	code := et.textarea.Value()

	if et.ChiselMode {
		et.compileChisel(gpu, code)
	} else {
		et.compileGLSL(gpu, code)
	}
}

func (et *EditorTab) compileChisel(gpu *gpupkg.GPURenderer, code string) {
	glsl, diags := chisel.Compile(code)

	// Check for Chisel compilation errors
	var errors []diagnostic.Diagnostic
	for _, d := range diags {
		if d.Severity == diagnostic.Error {
			errors = append(errors, d)
		}
	}
	if len(errors) > 0 {
		// Show the first error in the status bar
		d := errors[0]
		loc := ""
		if d.Span.Start.Line > 0 {
			loc = fmt.Sprintf(":%d:%d", d.Span.Start.Line, d.Span.Start.Col)
		}
		et.Status = fmt.Sprintf("Chisel%s: %s", loc, d.Message)
		et.StatusErr = true
		return
	}

	// Chisel compiled OK — now compile the GLSL
	err := gpu.CompileUserCode(glsl)
	if err != nil {
		et.Status = fmt.Sprintf("GLSL: %s", err.Error())
		et.StatusErr = true
	} else {
		et.Status = "Chisel → GLSL ✓"
		et.StatusErr = false
	}
}

func (et *EditorTab) compileGLSL(gpu *gpupkg.GPURenderer, code string) {
	// Standalone GLSL files use minimal prefix (no SDF library)
	err := gpu.CompileGLSLCode(code)
	if err != nil {
		errMsg := err.Error()
		et.Status = fmt.Sprintf("Error: %s", errMsg)
		et.StatusErr = true
	} else {
		et.Status = "Compiled OK"
		et.StatusErr = false
	}
}

// Update processes a bubbletea message. Returns (handled, cmd).
func (et *EditorTab) Update(msg tea.Msg) (bool, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyPressMsg); ok {
		if keyMsg.String() == "ctrl+r" {
			return true, nil // caller handles compile
		}
	}
	var cmd tea.Cmd
	et.textarea, cmd = et.textarea.Update(msg)
	// Keep textarea height in sync with content (lines may have been added/removed)
	et.syncTextareaHeight()
	// Keep cursor visible in the scroll view (vertical + horizontal)
	et.ScrollView.EnsureLineVisible(et.textarea.Line())
	lineInfo := et.textarea.LineInfo()
	// Cursor visual column = line number gutter width + column offset
	lineCount := len(strings.Split(et.textarea.Value(), "\n"))
	gutterWidth := len(fmt.Sprintf(" %d ", lineCount)) + 1 // matches textarea's " N│ " format
	et.ScrollView.EnsureColumnVisible(gutterWidth + lineInfo.ColumnOffset)
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
	scrolled := et.ScrollView.RenderContent(trimmedView)
	sb.WriteString(scrolled)
	sb.WriteString("\n")

	// Status bar
	statusStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("252")).
		Background(lipgloss.Color("236"))
	if et.StatusErr {
		statusStyle = statusStyle.Foreground(lipgloss.Color("196"))
	}

	statusLine := " " + et.Status
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
