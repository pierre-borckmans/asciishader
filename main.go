package main

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type tickMsg time.Time

// FocusZone identifies what has keyboard focus.
type FocusZone int

const (
	FocusViewport FocusZone = iota
	FocusControls
	FocusEditor
)

type model struct {
	renderer   *Renderer
	gpu        *GPURenderer
	gpuMode    bool
	width      int
	height     int
	time       float64
	scene      int
	paused     bool
	camAngleY  float64
	camAngleX  float64
	camDist    float64
	autoRotate bool
	mouseLastX int
	mouseLastY int
	mouseDrag  bool
	fps        float64
	lastFrame  time.Time
	frame      string

	// Layout components
	sidebar     *Sidebar
	rightPanel  *RightPanel
	bottomPanel *BottomPanel
	controls    *ControlsTab
	editor      *EditorTab
	focus       FocusZone
}

func initialModel() model {
	r := NewRenderer(80, 24)
	r.ShapeTable = NewShapeTable()
	r.ShapeMode = true
	r.Contrast = 2.0
	r.Spread = 1.0
	r.ExtDist = 1.0
	r.Ambient = 0.15
	r.SpecPower = 32.0
	r.ShadowSteps = 32
	r.AOSteps = 5

	// Build sidebar items from scenes
	sb := NewSidebar()
	items := make([]SidebarItem, len(scenes))
	for i, s := range scenes {
		icon := string([]rune(s.Name)[0:1])
		items[i] = SidebarItem{
			ID:   fmt.Sprintf("scene-%d", i),
			Icon: icon,
			Name: s.Name,
		}
	}
	sb.SetItems(items)
	sb.SetActiveID("scene-0")

	rp := NewRightPanel()
	rp.SetExpanded(false)

	bp := NewBottomPanel()
	bp.SetTitle("GLSL Editor")

	return model{
		renderer:    r,
		camDist:     4.0,
		scene:       0,
		lastFrame:   time.Now(),
		sidebar:     sb,
		rightPanel:  rp,
		bottomPanel: bp,
		controls:    NewControlsTab(),
		editor:      NewEditorTab(),
		focus:       FocusViewport,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		tea.EnterAltScreen,
	)
}

func tick() tea.Cmd {
	return tea.Tick(time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// headerHeight returns the height of the header (3 lines: top edge + title + bottom edge).
func headerHeight() int {
	return 3
}

// footerHeight returns the height of the footer (1 line).
func footerHeight() int {
	return 1
}

// contentWidth returns the width available for the main content area.
func (m model) contentWidth() int {
	w := m.width - m.sidebar.Width() - 2 // sidebar + left gap
	if m.rightPanel.Width() > 0 {
		w -= m.rightPanel.Width() + 2 // right panel + right gap
	}
	if w < 1 {
		w = 1
	}
	return w
}

// viewportHeight returns the height available for the viewport.
func (m model) viewportHeight() int {
	middleHeight := m.height - headerHeight() - footerHeight()
	if middleHeight < 1 {
		middleHeight = 1
	}
	if m.bottomPanel.Height() > 0 {
		middleHeight -= m.bottomPanel.Height()
	}
	if middleHeight < 1 {
		middleHeight = 1
	}
	return middleHeight
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewport()
		return m, nil

	case PanelAnimTickMsg:
		switch msg.ID {
		case "sidebar":
			cmd := m.sidebar.AnimTick()
			m.resizeViewport()
			return m, cmd
		case "right-panel":
			cmd := m.rightPanel.AnimTick()
			m.resizeViewport()
			return m, cmd
		case "bottom-panel":
			cmd := m.bottomPanel.AnimTick()
			m.resizeViewport()
			return m, cmd
		}
		return m, nil

	case tickMsg:
		now := time.Now()
		dt := now.Sub(m.lastFrame).Seconds()

		// Cap at 60fps
		if dt < 1.0/63.0 {
			return m, tick()
		}

		if dt > 0 {
			m.fps = m.fps*0.9 + (1.0/dt)*0.1
		}
		m.lastFrame = now

		if !m.paused {
			m.time += dt
		}

		if m.autoRotate && !m.paused {
			m.camAngleY += dt * 0.3
		}

		// Update camera
		m.renderer.Camera.Pos = Vec3{
			math.Sin(m.camAngleY) * math.Cos(m.camAngleX) * m.camDist,
			math.Sin(m.camAngleX) * m.camDist,
			-math.Cos(m.camAngleY) * math.Cos(m.camAngleX) * m.camDist,
		}
		m.renderer.Camera.Target = V(0, 0, 0)
		m.renderer.Time = m.time
		m.renderer.Scene = scenes[m.scene].SDF
		m.renderer.ColorFunc = scenes[m.scene].Color

		// Animated light
		m.renderer.LightDir = V(
			math.Sin(m.time*0.5)*0.5,
			0.8,
			math.Cos(m.time*0.5)*0.5-0.5,
		).Normalize()

		// Resize viewport if needed (panel animation may have changed width)
		cw := m.contentWidth()
		vh := m.viewportHeight()
		if m.renderer.Width != cw || m.renderer.Height != vh {
			m.renderer.Resize(cw, vh)
		}

		// Render frame
		if m.gpuMode && m.gpu != nil {
			m.frame = m.gpu.Render(m.renderer)
		} else {
			m.frame = m.renderer.Render()
		}

		return m, tick()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	hh := headerHeight()

	// Right panel resize
	if m.rightPanel.Width() > 0 || m.rightPanel.Animating() {
		rpEdgeX := m.width - m.rightPanel.Width() - 2
		m.rightPanel.Resizer().SetEdgePos(rpEdgeX)
		if m.rightPanel.HandleResizeEvent(msg, m.width) {
			m.resizeViewport()
			return m, nil
		}
	}

	// Bottom panel resize
	if m.bottomPanel.Height() > 0 || m.bottomPanel.Animating() {
		middleHeight := m.height - hh - footerHeight()
		bpEdgeY := hh + middleHeight - m.bottomPanel.Height()
		m.bottomPanel.Resizer().SetEdgePos(bpEdgeY)
		if m.bottomPanel.HandleResizeEvent(msg, m.height) {
			m.resizeViewport()
			return m, nil
		}
	}

	// Right panel scroll
	if m.rightPanel.IsExpanded() {
		rpX := m.width - m.rightPanel.Width() + 2
		rpY := hh
		m.rightPanel.ScrollView().SetPosition(rpX, rpY)
		if m.rightPanel.HandleMouseEvent(msg) {
			return m, nil
		}
	}

	// Bottom panel (editor) scroll
	if m.bottomPanel.IsExpanded() {
		middleH := m.height - hh - footerHeight()
		bpY := hh + middleH - m.bottomPanel.Height() + 2 // +2 for separator + title row
		sidebarW := m.sidebar.Width()
		bpX := sidebarW + 2 + 1 // sidebar + left gap + left padding
		m.editor.scrollView.SetPosition(bpX, bpY)
		if m.editor.scrollView.HandleMouse(msg) {
			return m, nil
		}
	}

	// Viewport mouse (camera drag + zoom)
	// Only handle if within viewport area
	sidebarWidth := m.sidebar.Width()
	viewportLeft := sidebarWidth + 2
	viewportRight := m.width - m.rightPanel.Width()
	if m.rightPanel.Width() > 0 {
		viewportRight -= 2
	}

	middleHeight := m.height - hh - footerHeight()
	vpHeight := middleHeight
	if m.bottomPanel.Height() > 0 {
		vpHeight -= m.bottomPanel.Height()
	}

	inViewport := msg.X >= viewportLeft && msg.X < viewportRight &&
		msg.Y >= hh && msg.Y < hh+vpHeight

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft && inViewport {
			m.mouseDrag = true
			m.mouseLastX = msg.X
			m.mouseLastY = msg.Y
			m.autoRotate = false
		}
	case tea.MouseActionRelease:
		m.mouseDrag = false
	case tea.MouseActionMotion:
		if m.mouseDrag {
			dx := msg.X - m.mouseLastX
			dy := msg.Y - m.mouseLastY
			m.camAngleY += float64(dx) * 0.02
			m.camAngleX = clamp(m.camAngleX+float64(dy)*0.05, -math.Pi/2+0.1, math.Pi/2-0.1)
			m.mouseLastX = msg.X
			m.mouseLastY = msg.Y
		}
	}

	if inViewport {
		if msg.Button == tea.MouseButtonWheelUp {
			m.camDist = clamp(m.camDist*0.92, 1.5, 12)
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.camDist = clamp(m.camDist/0.92, 1.5, 12)
		}
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys
	switch key {
	case "ctrl+c":
		return m, tea.Quit
	case "s":
		if m.focus == FocusEditor {
			// Don't intercept 's' when typing in editor
			break
		}
		cmd := m.rightPanel.ToggleExpanded()
		if m.rightPanel.IsExpanded() {
			m.focus = FocusControls
		} else if m.focus == FocusControls {
			m.focus = FocusViewport
		}
		m.resizeViewport()
		return m, cmd
	case "e":
		if m.focus == FocusEditor {
			// Don't intercept 'e' when typing in editor
			break
		}
		cmd := m.bottomPanel.ToggleExpanded()
		if m.bottomPanel.IsExpanded() {
			m.focus = FocusEditor
			m.editor.Focus()
		} else if m.focus == FocusEditor {
			m.focus = FocusViewport
			m.editor.Blur()
		}
		m.resizeViewport()
		return m, cmd
	}

	// Focus-dependent routing
	switch m.focus {
	case FocusViewport:
		return m.handleViewportKey(key)
	case FocusControls:
		return m.handleControlsKey(key)
	case FocusEditor:
		return m.handleEditorKey(msg)
	}

	return m, nil
}

func (m model) handleViewportKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q", "esc":
		return m, tea.Quit
	case "left", "h":
		m.camAngleY -= 0.15
		m.autoRotate = false
	case "right", "l":
		m.camAngleY += 0.15
		m.autoRotate = false
	case "up", "k":
		m.camAngleX = clamp(m.camAngleX+0.1, -math.Pi/2+0.1, math.Pi/2-0.1)
		m.autoRotate = false
	case "down", "j":
		m.camAngleX = clamp(m.camAngleX-0.1, -math.Pi/2+0.1, math.Pi/2-0.1)
		m.autoRotate = false
	case "+", "=":
		m.camDist = clamp(m.camDist*0.92, 1.5, 12)
	case "-", "_":
		m.camDist = clamp(m.camDist/0.92, 1.5, 12)
	case "tab":
		// Cycle focus: viewport → controls (if open) → editor (if open) → viewport
		if m.rightPanel.IsExpanded() {
			m.focus = FocusControls
			return m, nil
		} else if m.bottomPanel.IsExpanded() {
			m.focus = FocusEditor
			m.editor.Focus()
			return m, nil
		}
	case "n":
		m.scene = (m.scene + 1) % len(scenes)
		m.sidebar.SetActiveID(fmt.Sprintf("scene-%d", m.scene))
		m.time = 0
		m.syncSceneGLSL()
	case "shift+tab", "N":
		m.scene = (m.scene - 1 + len(scenes)) % len(scenes)
		m.sidebar.SetActiveID(fmt.Sprintf("scene-%d", m.scene))
		m.time = 0
		m.syncSceneGLSL()
	case " ":
		m.paused = !m.paused
	case "a":
		m.autoRotate = !m.autoRotate
	case "g":
		if m.gpu != nil {
			m.gpuMode = !m.gpuMode
		}
	case "[":
		m.renderer.Contrast = clamp(m.renderer.Contrast-0.25, 0.5, 5.0)
	case "]":
		m.renderer.Contrast = clamp(m.renderer.Contrast+0.25, 0.5, 5.0)
	case "1":
		m.renderer.Spread = clamp(m.renderer.Spread+0.25, 0.25, 3.0)
	case "!":
		m.renderer.Spread = clamp(m.renderer.Spread-0.25, 0.25, 3.0)
	case "2":
		m.renderer.ExtDist = clamp(m.renderer.ExtDist+0.25, 0.25, 3.0)
	case "@":
		m.renderer.ExtDist = clamp(m.renderer.ExtDist-0.25, 0.25, 3.0)
	case "3":
		m.renderer.Ambient = clamp(m.renderer.Ambient+0.05, 0.0, 0.5)
	case "#":
		m.renderer.Ambient = clamp(m.renderer.Ambient-0.05, 0.0, 0.5)
	case "4":
		m.renderer.SpecPower = clamp(m.renderer.SpecPower*1.5, 4, 128)
	case "$":
		m.renderer.SpecPower = clamp(m.renderer.SpecPower/1.5, 4, 128)
	case "5":
		m.renderer.ShadowSteps = min(m.renderer.ShadowSteps+4, 48)
	case "%":
		m.renderer.ShadowSteps = max(m.renderer.ShadowSteps-4, 0)
	case "6":
		m.renderer.AOSteps = min(m.renderer.AOSteps+1, 10)
	case "^":
		m.renderer.AOSteps = max(m.renderer.AOSteps-1, 0)
	case "r":
		m.camAngleX = 0
		m.camAngleY = 0
		m.camDist = 4.0
		m.autoRotate = false
		m.renderer.Contrast = 2.0
		m.renderer.Spread = 1.0
		m.renderer.ExtDist = 1.0
		m.renderer.Ambient = 0.15
		m.renderer.SpecPower = 32.0
		m.renderer.ShadowSteps = 32
		m.renderer.AOSteps = 5
		m.time = 0
	}
	return m, nil
}

func (m model) handleControlsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.focus = FocusViewport
		return m, nil
	case "tab":
		if m.bottomPanel.IsExpanded() {
			m.focus = FocusEditor
			m.editor.Focus()
		} else {
			m.focus = FocusViewport
		}
		return m, nil
	case "q":
		return m, tea.Quit
	}

	if m.controls.HandleKey(key, &m) {
		return m, nil
	}
	return m, nil
}

func (m model) handleEditorKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.focus = FocusViewport
		m.editor.Blur()
		return m, nil
	case "ctrl+r":
		m.editor.Compile(m.gpu)
		return m, nil
	}

	_, cmd := m.editor.Update(msg)
	return m, cmd
}

// syncSceneGLSL populates the editor with scene GLSL if available.
func (m *model) syncSceneGLSL() {
	if m.scene >= 0 && m.scene < len(scenes) {
		glsl := scenes[m.scene].GLSL
		if glsl != "" {
			m.editor.SetCode(glsl)
			if m.gpu != nil && m.gpuMode {
				m.editor.Compile(m.gpu)
			}
		}
	}
}

// resizeViewport updates the renderer dimensions based on current layout.
func (m *model) resizeViewport() {
	cw := m.contentWidth()
	vh := m.viewportHeight()
	if cw < 1 {
		cw = 1
	}
	if vh < 1 {
		vh = 1
	}
	m.renderer.Resize(cw, vh)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// --- Header ---
	gpuStr := "CPU"
	if m.gpuMode && m.gpu != nil {
		gpuStr = "GPU"
	}
	pauseStr := ""
	if m.paused {
		pauseStr = " PAUSED"
	}
	rightInfo := fmt.Sprintf("%s | %.0f fps%s", gpuStr, m.fps, pauseStr)
	header := ComposeHeader(
		fmt.Sprintf("ASCII Shader  ·  %s", scenes[m.scene].Name),
		rightInfo,
		m.width,
		m.sidebar.Width(),
		m.rightPanel.Width(),
	)

	// --- Footer ---
	bindings := []FooterBinding{
		{"n/N", "scene"},
		{"arrows", "camera"},
		{"+/-", "zoom"},
		{"s", "controls"},
		{"e", "editor"},
		{"g", "GPU"},
		{"tab", "focus"},
		{"esc", "viewport"},
		{"space", "pause"},
		{"a", "auto-rotate"},
		{"r", "reset"},
		{"q", "quit"},
	}
	focusStr := ""
	switch m.focus {
	case FocusControls:
		focusStr = "[Controls]"
	case FocusEditor:
		focusStr = "[Editor]"
	}
	footer := RenderFooter(bindings, m.width, focusStr)

	// --- Middle section ---
	hh := headerHeight()
	fh := footerHeight()
	middleHeight := m.height - hh - fh
	if middleHeight < 1 {
		middleHeight = 1
	}

	cw := m.contentWidth()

	// Build viewport content
	vpHeight := middleHeight
	if m.bottomPanel.Height() > 0 {
		vpHeight -= m.bottomPanel.Height()
	}
	if vpHeight < 1 {
		vpHeight = 1
	}

	// Viewport content — pad/truncate each line to content width
	viewContent := m.frame
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

	viewContentBlock := strings.Join(viewLines, "\n")

	// Append bottom panel below viewport content if expanded
	if m.bottomPanel.Height() > 0 {
		// Size editor to fit bottom panel (-1 for panel's left padding space)
		editorWidth := cw - 1
		if editorWidth < 1 {
			editorWidth = 1
		}
		editorHeight := m.bottomPanel.ContentHeight()
		m.editor.SetSize(editorWidth, editorHeight)
		editorContent := m.editor.Render(editorWidth)

		bottomPanelStr := m.bottomPanel.Render(cw, editorContent)
		viewContentBlock = viewContentBlock + "\n" + bottomPanelStr
	}

	// Right panel content
	var rightPanelStr string
	if m.rightPanel.Width() > 0 {
		m.controls.SyncFromRenderer(m.renderer)
		controlsContent := m.controls.Render(m.rightPanel.InnerWidth()-1, &m)
		rightPanelStr = m.rightPanel.Render(middleHeight, controlsContent)
	}

	// Sidebar
	sidebarStr := m.sidebar.Render(middleHeight)

	// Compose middle: sidebar + gap + content + gap + rightPanel
	leftGap := strings.Repeat(" ", 2)
	var middle string
	if rightPanelStr != "" {
		rightGap := strings.Repeat(" ", 2)
		middle = lipgloss.JoinHorizontal(lipgloss.Top, sidebarStr, leftGap, viewContentBlock, rightGap, rightPanelStr)
	} else {
		middle = lipgloss.JoinHorizontal(lipgloss.Top, sidebarStr, leftGap, viewContentBlock)
	}

	return header + "\n" + middle + "\n" + footer
}

func main() {
	runtime.LockOSThread()

	f, err := os.Create("cpu.prof")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not create CPU profile: %v\n", err)
		os.Exit(1)
	}
	pprof.StartCPUProfile(f)
	defer func() {
		pprof.StopCPUProfile()
		f.Close()
	}()

	m := initialModel()

	// Try GPU init — fall back to CPU silently
	gpu, gpuErr := NewGPURenderer()
	if gpuErr == nil {
		m.gpu = gpu
		m.gpuMode = true
		defer gpu.Destroy()
	}

	p := tea.NewProgram(
		m,
		tea.WithAltScreen(),
		tea.WithMouseAllMotion(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
