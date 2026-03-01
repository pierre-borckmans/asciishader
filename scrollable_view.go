package main

import (
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"
)

// ansiRegex matches ANSI escape codes
var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;:]*[a-zA-Z]|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[PX^_][^\x1b]*\x1b\\|\x1b[@-Z\\-_]|\x1b\[[\?]?[0-9;]*[a-zA-Z]`)

// stripANSI removes ANSI escape codes from a string
func stripANSI(s string) string {
	return ansiRegex.ReplaceAllString(s, "")
}

// ScrollableView is a reusable component for scrolling any content
type ScrollableView struct {
	scrollOffset  int // Lines scrolled from top (vertical)
	hScrollOffset int // Characters scrolled from left (horizontal)
	totalLines    int // Total lines in content
	maxLineWidth  int // Width of the widest line
	height        int // Visible height in lines
	width         int // Visible width in characters

	// Screen position (set by parent for mouse handling)
	screenX int
	screenY int

	// Drag state
	draggingV bool // Currently dragging vertical scrollbar
	draggingH bool // Currently dragging horizontal scrollbar

	// Hover state
	hoverVThumb bool // Mouse is hovering over vertical scrollbar thumb
	hoverHThumb bool // Mouse is hovering over horizontal scrollbar thumb

	// Truncation mode
	truncateLines bool

	// Auto-follow mode
	autoFollow bool
}

// NewScrollableView creates a new scrollable view
func NewScrollableView() *ScrollableView {
	return &ScrollableView{
		height: 20,
		width:  80,
	}
}

// SetHeight sets the visible height in lines
func (sv *ScrollableView) SetHeight(height int) {
	sv.height = height
	sv.clampScroll()
}

// SetWidth sets the visible width in characters
func (sv *ScrollableView) SetWidth(width int) {
	sv.width = width
}

// SetSize sets both width and height
func (sv *ScrollableView) SetSize(width, height int) {
	sv.width = width
	sv.height = height
	sv.clampScroll()
}

// SetTruncate enables or disables line truncation in RenderContent.
func (sv *ScrollableView) SetTruncate(truncate bool) {
	sv.truncateLines = truncate
}

// SetAutoFollow enables or disables auto-follow mode.
func (sv *ScrollableView) SetAutoFollow(follow bool) {
	sv.autoFollow = follow
}

// SetPosition sets the screen position of this view (for mouse handling)
func (sv *ScrollableView) SetPosition(x, y int) {
	sv.screenX = x
	sv.screenY = y
}

// ScreenX returns the screen X position set by the parent
func (sv *ScrollableView) ScreenX() int {
	return sv.screenX
}

// ScreenY returns the screen Y position set by the parent
func (sv *ScrollableView) ScreenY() int {
	return sv.screenY
}

// ScrollOffset returns current scroll offset
func (sv *ScrollableView) ScrollOffset() int {
	return sv.scrollOffset
}

// SetScrollOffset sets the scroll offset directly
func (sv *ScrollableView) SetScrollOffset(offset int) {
	sv.scrollOffset = offset
	sv.clampScroll()
}


// ScrollUp scrolls up by n lines
func (sv *ScrollableView) ScrollUp(n int) {
	sv.scrollOffset -= n
	sv.clampScroll()
}

// ScrollDown scrolls down by n lines
func (sv *ScrollableView) ScrollDown(n int) {
	sv.scrollOffset += n
	sv.clampScroll()
}

// ScrollLeft scrolls left by n characters
func (sv *ScrollableView) ScrollLeft(n int) {
	sv.hScrollOffset -= n
	sv.clampHScroll()
}

// ScrollRight scrolls right by n characters
func (sv *ScrollableView) ScrollRight(n int) {
	sv.hScrollOffset += n
	sv.clampHScroll()
}

// ScrollToTop scrolls to the top
func (sv *ScrollableView) ScrollToTop() {
	sv.scrollOffset = 0
}

// ScrollToBottom scrolls to the bottom
func (sv *ScrollableView) ScrollToBottom() {
	sv.scrollOffset = sv.maxScroll()
}

// EnsureColumnVisible scrolls horizontally to make a specific column visible.
func (sv *ScrollableView) EnsureColumnVisible(col int) {
	contentWidth := sv.width
	if sv.CanScroll() {
		contentWidth -= 2 // scrollbar column + gap
	}
	if contentWidth < 1 {
		contentWidth = 1
	}
	if col < sv.hScrollOffset {
		sv.hScrollOffset = col
	} else if col >= sv.hScrollOffset+contentWidth {
		sv.hScrollOffset = col - contentWidth + 1
	}
	sv.clampHScroll()
}

// EnsureLineVisible scrolls to make a specific line visible
func (sv *ScrollableView) EnsureLineVisible(lineIdx int) {
	visibleHeight := sv.height
	if sv.CanScrollHorizontally() {
		visibleHeight--
	}
	if visibleHeight < 1 {
		visibleHeight = 1
	}

	if lineIdx < sv.scrollOffset {
		sv.scrollOffset = lineIdx
	} else if lineIdx >= sv.scrollOffset+visibleHeight {
		sv.scrollOffset = lineIdx - visibleHeight + 1
	}
	sv.clampScroll()
}

// HandleMouseWheel handles mouse wheel events, returns true if handled
func (sv *ScrollableView) HandleMouseWheel(msg tea.MouseMsg) bool {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		if msg.Shift {
			if sv.hScrollOffset > 0 {
				sv.ScrollLeft(1)
			}
		} else {
			if sv.scrollOffset > 0 {
				sv.ScrollUp(1)
			}
		}
		return true
	case tea.MouseButtonWheelDown:
		if msg.Shift {
			if sv.hScrollOffset < sv.maxHScroll() {
				sv.ScrollRight(1)
			}
		} else {
			if sv.scrollOffset < sv.maxScroll() {
				sv.ScrollDown(1)
			}
		}
		return true
	case tea.MouseButtonWheelLeft:
		if sv.hScrollOffset > 0 {
			sv.ScrollLeft(1)
		}
		return true
	case tea.MouseButtonWheelRight:
		if sv.hScrollOffset < sv.maxHScroll() {
			sv.ScrollRight(1)
		}
		return true
	default:
		return false
	}
}

// HandleMouse handles all mouse events including clicks and drags on scrollbars.
func (sv *ScrollableView) HandleMouse(msg tea.MouseMsg) bool {
	if msg.Button == tea.MouseButtonWheelUp || msg.Button == tea.MouseButtonWheelDown ||
		msg.Button == tea.MouseButtonWheelLeft || msg.Button == tea.MouseButtonWheelRight {
		return sv.HandleMouseWheel(msg)
	}

	relX := msg.X - sv.screenX
	relY := msg.Y - sv.screenY

	hasVScroll := sv.CanScroll()
	hasHScroll := sv.CanScrollHorizontally()

	visibleContentLines := sv.totalLines - sv.scrollOffset
	if visibleContentLines > sv.height {
		visibleContentLines = sv.height
	}
	if hasHScroll && visibleContentLines > sv.height-1 {
		visibleContentLines = sv.height - 1
	}

	vTrackHeight := sv.height
	if hasHScroll {
		vTrackHeight--
	}

	hScrollRow := visibleContentLines
	if hasVScroll {
		hScrollRow = vTrackHeight
	}

	hTrackWidth := sv.width
	if hasVScroll {
		hTrackWidth -= 2
	}

	onVScrollbar := hasVScroll && relX == sv.width-1 && relY >= 0 && relY < vTrackHeight
	onHScrollbar := hasHScroll && relY == hScrollRow && relX >= 0 && relX < hTrackWidth

	oldVHover := sv.hoverVThumb
	oldHHover := sv.hoverHThumb
	sv.hoverVThumb = onVScrollbar && relY == sv.vThumbPos(vTrackHeight)
	sv.hoverHThumb = onHScrollbar && relX == sv.hThumbPos(hTrackWidth)
	hoverChanged := sv.hoverVThumb != oldVHover || sv.hoverHThumb != oldHHover

	if !onVScrollbar && !onHScrollbar && !sv.draggingV && !sv.draggingH {
		if hoverChanged && msg.Action == tea.MouseActionMotion {
			return true
		}
		return false
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft {
			if onVScrollbar {
				sv.draggingV = true
				sv.updateVScrollFromMouse(relY, vTrackHeight)
				return true
			}
			if onHScrollbar {
				sv.draggingH = true
				sv.updateHScrollFromMouse(relX, hTrackWidth)
				return true
			}
		}

	case tea.MouseActionMotion:
		if sv.draggingV {
			sv.updateVScrollFromMouse(relY, vTrackHeight)
			return true
		}
		if sv.draggingH {
			sv.updateHScrollFromMouse(relX, hTrackWidth)
			return true
		}
		if hoverChanged {
			return true
		}

	case tea.MouseActionRelease:
		if sv.draggingV || sv.draggingH {
			sv.draggingV = false
			sv.draggingH = false
			return true
		}
	}

	return false
}

func (sv *ScrollableView) vThumbPos(trackHeight int) int {
	maxScroll := sv.maxScroll()
	if maxScroll <= 0 || trackHeight <= 1 {
		return 0
	}
	return (sv.scrollOffset * (trackHeight - 1)) / maxScroll
}

func (sv *ScrollableView) hThumbPos(trackWidth int) int {
	maxScroll := sv.maxHScroll()
	if maxScroll <= 0 || trackWidth <= 1 {
		return 0
	}
	pos := (sv.hScrollOffset * (trackWidth - 1)) / maxScroll
	if pos >= trackWidth {
		pos = trackWidth - 1
	}
	return pos
}

func (sv *ScrollableView) updateVScrollFromMouse(relY, trackHeight int) {
	maxScroll := sv.maxScroll()
	if maxScroll <= 0 || trackHeight <= 1 {
		return
	}
	if relY < 0 {
		relY = 0
	}
	if relY >= trackHeight {
		relY = trackHeight - 1
	}
	sv.scrollOffset = (relY * maxScroll) / (trackHeight - 1)
	sv.clampScroll()
}

func (sv *ScrollableView) updateHScrollFromMouse(relX, trackWidth int) {
	maxScroll := sv.maxHScroll()
	if maxScroll <= 0 || trackWidth <= 1 {
		return
	}
	if relX < 0 {
		relX = 0
	}
	if relX >= trackWidth {
		relX = trackWidth - 1
	}
	sv.hScrollOffset = (relX * maxScroll) / (trackWidth - 1)
	sv.clampHScroll()
}

// IsDragging returns true if currently dragging a scrollbar
func (sv *ScrollableView) IsDragging() bool {
	return sv.draggingV || sv.draggingH
}

// RenderContent takes full content and returns only the visible portion with scrollbar
func (sv *ScrollableView) RenderContent(content string) string {
	lines := strings.Split(content, "\n")
	sv.totalLines = len(lines)

	// Calculate max line width for horizontal scrolling
	sv.maxLineWidth = 0
	for _, line := range lines {
		w := runewidth.StringWidth(stripANSI(line))
		if w > sv.maxLineWidth {
			sv.maxLineWidth = w
		}
	}

	// Auto-follow: pin to bottom
	if sv.autoFollow && sv.totalLines > sv.height {
		sv.scrollOffset = sv.totalLines - sv.height
	}

	sv.clampScroll()
	sv.clampHScroll()

	hasVScroll := sv.CanScroll()
	hasHScroll := sv.CanScrollHorizontally()

	contentHeight := sv.height
	if hasHScroll {
		contentHeight = sv.height - 1
	}
	if contentHeight < 0 {
		contentHeight = 0
	}

	start := sv.scrollOffset
	end := start + contentHeight

	if start >= len(lines) {
		start = 0
		end = contentHeight
	}
	if end > len(lines) {
		end = len(lines)
	}

	visibleLines := make([]string, 0, contentHeight+1)
	visibleLines = append(visibleLines, lines[start:end]...)

	// Apply horizontal scrolling
	if sv.hScrollOffset > 0 {
		for i := range visibleLines {
			visibleLines[i] = horizontalSlice(visibleLines[i], sv.hScrollOffset)
		}
	}

	// Calculate content width
	contentWidth := sv.width
	if hasVScroll {
		contentWidth = sv.width - 2
	}

	// Truncate lines if enabled
	if sv.truncateLines && !hasVScroll {
		for i := range visibleLines {
			visibleLines[i] = truncateOrPadLine(visibleLines[i], contentWidth)
		}
	}

	// Add vertical scrollbar
	if hasVScroll && sv.width > 0 {
		scrollbar := sv.renderScrollbar(contentHeight)

		for len(visibleLines) < contentHeight {
			visibleLines = append(visibleLines, "")
		}

		for i := 0; i < contentHeight; i++ {
			visibleLines[i] = truncateOrPadLine(visibleLines[i], contentWidth) + "\x1b[0m " + scrollbar[i]
		}
	}

	// Add horizontal scrollbar
	if hasHScroll && sv.width > 0 {
		hScrollbar := sv.renderHScrollbar(contentWidth)
		visibleLines = append(visibleLines, hScrollbar)
	}

	return strings.Join(visibleLines, "\n")
}

func (sv *ScrollableView) renderHScrollbar(width int) string {
	if width <= 0 || sv.maxLineWidth == 0 {
		return ""
	}

	maxScroll := sv.maxHScroll()
	var thumbPos int
	if maxScroll > 0 && width > 1 {
		thumbPos = (sv.hScrollOffset * (width - 1)) / maxScroll
	}
	if thumbPos < 0 {
		thumbPos = 0
	}
	if thumbPos >= width {
		thumbPos = width - 1
	}

	leftWidth := thumbPos
	rightWidth := width - thumbPos - 1
	if rightWidth < 0 {
		rightWidth = 0
	}

	thumbColor := "39" // Cyan
	if sv.hoverHThumb || sv.draggingH {
		thumbColor = "226" // Yellow on hover/drag
	}
	result := strings.Builder{}
	result.WriteString("\x1b[38;5;39m")
	result.WriteString(strings.Repeat("━", leftWidth))
	result.WriteString("\x1b[38;5;" + thumbColor + "m●\x1b[38;5;39m")
	result.WriteString(strings.Repeat("─", rightWidth))
	result.WriteString("\x1b[0m")

	return result.String()
}

// horizontalSlice removes the first n visible characters from a line, preserving ANSI codes
func horizontalSlice(line string, offset int) string {
	if offset <= 0 {
		return line
	}

	result := strings.Builder{}
	skipped := 0
	inEscape := false
	escapeSeq := strings.Builder{}

	for _, r := range line {
		if inEscape {
			escapeSeq.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				result.WriteString(escapeSeq.String())
				escapeSeq.Reset()
				inEscape = false
			}
			continue
		}

		if r == '\x1b' {
			inEscape = true
			escapeSeq.WriteRune(r)
			continue
		}

		charWidth := runewidth.RuneWidth(r)
		if skipped < offset {
			skipped += charWidth
			continue
		}

		result.WriteRune(r)
	}

	return result.String()
}

// truncateOrPadLine ensures line is exactly targetWidth visible characters
func truncateOrPadLine(line string, targetWidth int) string {
	if targetWidth <= 0 {
		return ""
	}

	// Fast path: pure ASCII with no ANSI codes
	hasANSI := false
	isASCII := true
	for i := 0; i < len(line); i++ {
		b := line[i]
		if b == '\x1b' {
			hasANSI = true
		}
		if b > 127 {
			isASCII = false
			break
		}
	}

	if isASCII && !hasANSI {
		if len(line) <= targetWidth {
			return line + strings.Repeat(" ", targetWidth-len(line))
		}
		return line[:targetWidth]
	}

	if isASCII && hasANSI {
		stripped := stripANSI(line)
		currentWidth := len(stripped)

		if currentWidth <= targetWidth {
			return line + strings.Repeat(" ", targetWidth-currentWidth)
		}

		return truncateWithANSIASCII(line, targetWidth)
	}

	return truncateWithANSIUnicode(line, targetWidth)
}

func truncateWithANSIASCII(line string, targetWidth int) string {
	result := strings.Builder{}
	width := 0
	inEscape := false
	escapeSeq := strings.Builder{}

	for i := 0; i < len(line); i++ {
		b := line[i]

		if inEscape {
			escapeSeq.WriteByte(b)
			if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') {
				result.WriteString(escapeSeq.String())
				escapeSeq.Reset()
				inEscape = false
			}
			continue
		}

		if b == '\x1b' {
			inEscape = true
			escapeSeq.WriteByte(b)
			continue
		}

		if width >= targetWidth {
			break
		}
		result.WriteByte(b)
		width++
	}

	if width < targetWidth {
		result.WriteString(strings.Repeat(" ", targetWidth-width))
	}

	return result.String()
}

func truncateWithANSIUnicode(line string, targetWidth int) string {
	stripped := stripANSI(line)
	currentWidth := runewidth.StringWidth(stripped)

	if currentWidth <= targetWidth {
		return line + strings.Repeat(" ", targetWidth-currentWidth)
	}

	result := strings.Builder{}
	width := 0
	inEscape := false
	escapeSeq := strings.Builder{}

	for _, r := range line {
		if inEscape {
			escapeSeq.WriteRune(r)
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				result.WriteString(escapeSeq.String())
				escapeSeq.Reset()
				inEscape = false
			}
			continue
		}

		if r == '\x1b' {
			inEscape = true
			escapeSeq.WriteRune(r)
			continue
		}

		charWidth := runewidth.RuneWidth(r)
		if width+charWidth > targetWidth {
			break
		}
		result.WriteRune(r)
		width += charWidth
	}

	if width < targetWidth {
		result.WriteString(strings.Repeat(" ", targetWidth-width))
	}

	return result.String()
}

// renderScrollbar returns a slice of scrollbar characters for each visible line
func (sv *ScrollableView) renderScrollbar(visibleCount int) []string {
	if visibleCount == 0 || sv.totalLines == 0 {
		return nil
	}

	result := make([]string, visibleCount)

	maxScroll := sv.maxScroll()
	var thumbPos int
	if maxScroll > 0 {
		thumbPos = (sv.scrollOffset * (visibleCount - 1)) / maxScroll
	}

	thumbColor := "39" // Cyan
	if sv.hoverVThumb || sv.draggingV {
		thumbColor = "226" // Yellow on hover/drag
	}
	for i := 0; i < visibleCount; i++ {
		if i < thumbPos {
			result[i] = "\x1b[38;5;39m┃\x1b[0m"
		} else if i == thumbPos {
			result[i] = "\x1b[38;5;" + thumbColor + "m●\x1b[0m"
		} else {
			result[i] = "\x1b[38;5;39m│\x1b[0m"
		}
	}

	return result
}

// CanScroll returns true if content is taller than visible height
func (sv *ScrollableView) CanScroll() bool {
	return sv.totalLines > sv.height
}

// ScrollInfo returns scroll position info (e.g., "1-20/50")
func (sv *ScrollableView) ScrollInfo() string {
	if !sv.CanScroll() {
		return ""
	}
	endLine := sv.scrollOffset + sv.height
	if endLine > sv.totalLines {
		endLine = sv.totalLines
	}
	return fmt.Sprintf("%d-%d/%d", sv.scrollOffset+1, endLine, sv.totalLines)
}

func (sv *ScrollableView) maxScroll() int {
	m := sv.totalLines - sv.height
	if m < 0 {
		return 0
	}
	return m
}

func (sv *ScrollableView) maxHScroll() int {
	visibleWidth := sv.width - 1
	if !sv.CanScroll() {
		visibleWidth = sv.width
	}
	m := sv.maxLineWidth - visibleWidth
	if m < 0 {
		return 0
	}
	return m
}

func (sv *ScrollableView) clampScroll() {
	if sv.scrollOffset < 0 {
		sv.scrollOffset = 0
	}
	m := sv.maxScroll()
	if sv.scrollOffset > m {
		sv.scrollOffset = m
	}
}

func (sv *ScrollableView) clampHScroll() {
	if sv.hScrollOffset < 0 {
		sv.hScrollOffset = 0
	}
	m := sv.maxHScroll()
	if sv.hScrollOffset > m {
		sv.hScrollOffset = m
	}
}

// CanScrollHorizontally returns true if content is wider than visible width
func (sv *ScrollableView) CanScrollHorizontally() bool {
	visibleWidth := sv.width - 1
	if !sv.CanScroll() {
		visibleWidth = sv.width
	}
	return sv.maxLineWidth > visibleWidth
}
