package app

import (
	"fmt"
	"strings"
	"time"

	"asciishader/pkg/core"
	"asciishader/pkg/recorder"
	"asciishader/pkg/scene"
	"asciishader/tui/layout"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	zone "github.com/lrstanley/bubblezone/v2"
)

func (m Model) View() tea.View {
	if m.Width == 0 || m.Height == 0 {
		v := tea.NewView("Initializing...")
		v.AltScreen = true
		v.MouseMode = tea.MouseModeAllMotion
		return v
	}

	// --- Header ---
	var headerTitle, rightInfo string
	rpWidth := 0 // right panel width for header layout

	switch m.Mode {
	case ViewShader:
		modeStr := "SHAPES"
		switch m.Config.RenderMode {
		case core.RenderBlocks:
			modeStr = "BLOCK"
		case core.RenderBraille:
			modeStr = "BRAILLE"
		case core.RenderSlice:
			modeStr = "SLICE"
		case core.RenderCost:
			modeStr = "COST"
		}
		// Projection mode
		switch m.Config.Projection {
		case core.ProjectionOrthographic:
			modeStr += " ORTHO"
		case core.ProjectionIsometric:
			modeStr += " ISO"
		}
		pauseStr := ""
		if m.Paused {
			pauseStr = " PAUSED"
		}
		// Recording indicator
		recStr := ""
		recStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6600")).Bold(true)
		switch m.RecState {
		case recorder.RecordSelecting:
			recStr = " | " + recStyle.Render("SELECT REGION")
		case recorder.RecordLive:
			if m.Recorder != nil {
				dur := m.Recorder.RecordingDuration()
				recStr = " | " + recStyle.Render(fmt.Sprintf("● REC %.1fs", dur.Seconds()))
			}
		case recorder.RecordBaking:
			if m.Recorder != nil {
				cur, total := m.Recorder.BakeProgress()
				pct := 0
				if total > 0 {
					pct = cur * 100 / total
				}
				recStr = " | " + recStyle.Render(fmt.Sprintf("Baking %d%%", pct))
			}
		case recorder.RecordDone:
			if m.RecMessage != "" && time.Since(m.RecMessageTime) < 3*time.Second {
				recStr = " | " + recStyle.Render("✓ "+m.RecMessage)
			}
		}
		// Show transient messages (e.g. profiling) when no recording status is displayed
		if recStr == "" && m.RecMessage != "" && time.Since(m.RecMessageTime) < 3*time.Second {
			recStr = " | " + recStyle.Render(m.RecMessage)
		}
		// Show persistent compile error (truncated to fit)
		if recStr == "" && m.CompileErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3333")).Bold(true)
			errMsg := m.CompileErr
			maxLen := m.Width / 3
			if maxLen < 20 {
				maxLen = 20
			}
			if len(errMsg) > maxLen {
				errMsg = errMsg[:maxLen-3] + "..."
			}
			recStr = " | " + errStyle.Render("✗ "+errMsg)
		}
		s := scene.Scenes[m.Scene]
		fileType := ""
		if s.Chisel != "" {
			fileType = " [chisel]"
		} else if s.GLSL != "" {
			fileType = " [glsl]"
		}
		headerTitle = fmt.Sprintf("ASCII Shader  ·  %s%s", s.Name, fileType)
		rightInfo = fmt.Sprintf("%s | %.0f fps%s%s", modeStr, m.FPS, pauseStr, recStr)
		rpWidth = m.RightPanel.Width()
	case ViewPlayer:
		headerTitle = "ASCII Shader  ·  Player"
	case ViewGallery:
		headerTitle = "ASCII Shader  ·  Gallery"
	case ViewHelp:
		headerTitle = "ASCII Shader  ·  Help"
	}

	header := layout.ComposeHeader(headerTitle, rightInfo, m.Width, m.Sidebar.Width(), rpWidth)

	// --- Footer ---
	var bindings []layout.FooterBinding
	focusStr := ""

	switch m.Mode {
	case ViewShader:
		bindings = []layout.FooterBinding{
			{Key: "F1-F4", Desc: "views"},
			{Key: "n/N", Desc: "scene"},
			{Key: "arrows", Desc: "camera"},
			{Key: "+/-", Desc: "zoom"},
			{Key: "o", Desc: "record"},
			{Key: "s", Desc: "controls"},
			{Key: "e", Desc: "editor"},
			{Key: "m", Desc: "mode"},
			{Key: "tab", Desc: "focus"},
			{Key: "space", Desc: "pause"},
			{Key: "q", Desc: "quit"},
		}
		switch m.Focus {
		case FocusControls:
			focusStr = "[Controls]"
		case FocusTree:
			focusStr = "[Tree]"
		case FocusEditor:
			focusStr = "[Editor]"
		}
	case ViewGallery:
		bindings = []layout.FooterBinding{
			{Key: "F1-F4", Desc: "views"},
			{Key: "↑/↓", Desc: "navigate"},
			{Key: "enter", Desc: "select"},
			{Key: "q", Desc: "quit"},
		}
	case ViewHelp:
		bindings = []layout.FooterBinding{
			{Key: "F1-F4", Desc: "views"},
			{Key: "↑/↓", Desc: "scroll"},
			{Key: "q", Desc: "quit"},
		}
	case ViewPlayer:
		if m.PlayerView.Loaded {
			bindings = []layout.FooterBinding{
				{Key: "F1-F4", Desc: "views"},
				{Key: "space", Desc: "pause"},
				{Key: "l", Desc: "loop"},
				{Key: "esc", Desc: "back"},
				{Key: "q", Desc: "quit"},
			}
		} else {
			bindings = []layout.FooterBinding{
				{Key: "F1-F4", Desc: "views"},
				{Key: "↑/↓", Desc: "navigate"},
				{Key: "enter", Desc: "load"},
				{Key: "q", Desc: "quit"},
			}
		}
	}
	footer := layout.RenderFooter(bindings, m.Width, focusStr)

	// --- Middle section ---
	hh := HeaderHeight()
	fh := FooterHeight()
	middleHeight := m.Height - hh - fh
	if middleHeight < 1 {
		middleHeight = 1
	}

	cw := m.ContentWidth()

	// Build viewport content based on view mode
	vpHeight := middleHeight
	if m.Mode == ViewShader && m.BottomPanel.Height() > 0 {
		vpHeight -= m.BottomPanel.Height()
	}
	if vpHeight < 1 {
		vpHeight = 1
	}

	var viewContentBlock string

	switch m.Mode {
	case ViewShader:
		// Viewport content — pad/truncate each line to content width
		viewContent := m.Frame
		viewLines := strings.Split(viewContent, "\n")
		for i, line := range viewLines {
			lineWidth := lipgloss.Width(line)
			if lineWidth < cw {
				viewLines[i] = line + strings.Repeat(" ", cw-lineWidth)
			}
		}
		// Pad to fill viewport height
		for len(viewLines) < vpHeight {
			viewLines = append(viewLines, strings.Repeat(" ", cw))
		}
		if len(viewLines) > vpHeight {
			viewLines = viewLines[:vpHeight]
		}

		// Region selection / recording overlay
		if m.RegionSelector != nil && (m.RecState == recorder.RecordSelecting || m.RecState == recorder.RecordLive) {
			if m.RecState == recorder.RecordLive && m.Recorder != nil {
				dur := m.Recorder.RecordingDuration()
				m.RegionSelector.RecLabel = fmt.Sprintf("● REC %.1fs", dur.Seconds())
			}
			viewLines = m.RegionSelector.RenderOverlay(viewLines)
		}

		viewContentBlock = strings.Join(viewLines, "\n")

		// Append bottom panel below viewport content if expanded
		if m.BottomPanel.Height() > 0 {
			editorWidth := cw - 1
			if editorWidth < 1 {
				editorWidth = 1
			}
			editorHeight := m.BottomPanel.ContentHeight()
			m.Editor.SetSize(editorWidth, editorHeight)
			editorContent := m.Editor.Render(editorWidth)
			bottomPanelStr := m.BottomPanel.Render(cw, editorContent)
			viewContentBlock = viewContentBlock + "\n" + bottomPanelStr
		}

	case ViewGallery:
		sceneNames := make([]string, len(scene.Scenes))
		for i, s := range scene.Scenes {
			sceneNames[i] = s.Name
		}
		viewContentBlock = m.Gallery.Render(cw, vpHeight, sceneNames)

	case ViewHelp:
		viewContentBlock = m.HelpView.Render(cw, vpHeight)

	case ViewPlayer:
		viewContentBlock = m.PlayerView.Render(cw, vpHeight)
	}

	// Right panel content (shader view only)
	var rightPanelStr string
	if m.Mode == ViewShader && m.RightPanel.Width() > 0 {
		m.Controls.SyncFromRenderConfig(m.Config)
		rpInner := m.RightPanel.InnerWidth() - 1
		controlsContent := m.Controls.Render(rpInner, &m)

		// Scene tree below controls (chisel scenes only)
		if m.SceneTree.Len() > 0 {
			controlsLines := strings.Count(controlsContent, "\n") + 1
			treeHeaderLines := 3 // blank + header + separator
			treeHeight := middleHeight - controlsLines - treeHeaderLines
			if treeHeight < 3 {
				treeHeight = 3
			}
			m.SceneTree.SetSize(rpInner, treeHeight)

			// Set tree screen position for mouse handling
			rpX := m.Width - m.RightPanel.Width() + 2
			treeY := hh + controlsLines + treeHeaderLines - m.RightPanel.ScrollView().ScrollOffset()
			m.SceneTree.SetPosition(rpX, treeY)

			headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
			dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("243"))
			sep := " ───────────────────────────"
			if rpInner > len(sep) {
				sep += strings.Repeat(" ", rpInner-len(sep))
			}
			treeHeader := "\n" + headerStyle.Render(fmt.Sprintf(" %-*s", rpInner-1, "Scene Tree")) + "\n" +
				dimStyle.Render(sep) + "\n"

			controlsContent += treeHeader + m.SceneTree.Render()
		}

		rightPanelStr = m.RightPanel.Render(middleHeight, controlsContent)
	}

	// Sidebar
	sidebarStr := m.Sidebar.Render(middleHeight)

	// Compose middle: sidebar + gap + content + gap + rightPanel
	leftGap := strings.Repeat(" ", 2)
	var middle string
	if rightPanelStr != "" {
		rightGap := strings.Repeat(" ", 2)
		middle = lipgloss.JoinHorizontal(lipgloss.Top, sidebarStr, leftGap, viewContentBlock, rightGap, rightPanelStr)
	} else {
		middle = lipgloss.JoinHorizontal(lipgloss.Top, sidebarStr, leftGap, viewContentBlock)
	}

	v := tea.NewView(zone.Scan(header + "\n" + middle + "\n" + footer))
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}
