package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"strings"
	"time"

	"asciishader/pkg/clip"
	"asciishader/pkg/core"
	gpupkg "asciishader/pkg/gpu"
	"asciishader/pkg/recorder"
	"asciishader/pkg/scene"
	"asciishader/tui/components"
	"asciishader/tui/controls"
	"asciishader/tui/editor"
	"asciishader/tui/layout"
	"asciishader/tui/views"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

type tickMsg time.Time

// FocusZone identifies what has keyboard focus.
type FocusZone int

const (
	FocusViewport FocusZone = iota
	FocusControls
	FocusEditor
)

// ViewMode identifies which top-level view is active.
type ViewMode int

const (
	ViewShader  ViewMode = iota // current raymarching (default)
	ViewPlayer                  // clip player
	ViewGallery                 // scene browser
	ViewHelp                    // keybindings reference
)

type model struct {
	config     *core.RenderConfig
	gpu        *gpupkg.GPURenderer
	width      int
	height     int
	time       float64
	scene      int
	paused     bool
	camAngleY  float64
	camAngleX  float64
	camDist    float64
	camTarget  core.Vec3
	autoRotate bool
	mouseLastX int
	mouseLastY int
	mouseDrag  bool
	mousePan   bool
	fps        float64
	lastFrame  time.Time
	frame      string

	// Layout components
	sidebar     *layout.Sidebar
	rightPanel  *layout.RightPanel
	bottomPanel *layout.BottomPanel
	controls    *controls.ControlsTab
	editor      *editor.EditorTab
	focus       FocusZone

	// View switching
	viewMode   ViewMode
	gallery    *views.GalleryView
	helpView   *views.HelpView
	playerView *views.PlayerView

	// Recording
	recorder       *recorder.Recorder
	recState       recorder.RecordingState
	regionSelector *recorder.RegionSelector
	recMessage     string    // transient status message (e.g. "Saved clip.asciirec")
	recMessageTime time.Time // when message was set (clears after 3s)
	compileErr     string    // persists until shader compiles successfully

	// Playback
	player   *clip.Player
	playMode bool // --play mode

	// File watching
	watchFile    string    // path to current scene's source file
	watchModTime time.Time // last known mod time
	watchCheck   time.Time // last time we checked

	// Profiling
	profiling bool
	profFile  *os.File
}

func initialModel() model {
	rc := core.NewRenderConfig(80, 24)
	rc.Contrast = 1.25
	rc.Spread = 0.75
	rc.ExtDist = 1.0
	rc.Ambient = 0.6
	rc.SpecPower = 9.0
	rc.ShadowSteps = 8
	rc.AOSteps = 2

	// Build sidebar items for view switching
	sb := layout.NewSidebar()
	sb.SetItems([]layout.SidebarItem{
		{ID: "view-shader", Icon: "◆", Name: "Shader"},
		{ID: "view-player", Icon: "▶", Name: "Player"},
		{ID: "view-gallery", Icon: "◫", Name: "Gallery"},
		{ID: "view-help", Icon: "?", Name: "Help"},
	})
	sb.SetActiveID("view-shader")

	rp := layout.NewRightPanel()
	rp.SetExpanded(false)

	bp := layout.NewBottomPanel()
	bp.SetTitle("GLSL Editor")

	return model{
		config:      rc,
		camDist:     4.0,
		scene:       0,
		lastFrame:   time.Now(),
		sidebar:     sb,
		rightPanel:  rp,
		bottomPanel: bp,
		controls:    controls.NewControlsTab(),
		editor:      editor.NewEditorTab(),
		focus:       FocusViewport,
		viewMode:    ViewShader,
		gallery:     views.NewGalleryView(),
		helpView:    views.NewHelpView(),
		playerView:  views.NewPlayerView(),
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
	if m.viewMode == ViewShader && m.rightPanel.Width() > 0 {
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
	if m.viewMode == ViewShader && m.bottomPanel.Height() > 0 {
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

	case components.PanelAnimTickMsg:
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

		// Player view: tick playback
		if m.viewMode == ViewPlayer {
			m.playerView.Tick(dt)
			return m, tick()
		}

		// Non-shader views: no rendering needed
		if m.viewMode != ViewShader {
			return m, tick()
		}

		if !m.paused {
			m.time += dt
		}

		if m.autoRotate && !m.paused {
			m.camAngleY += dt * 0.3
		}

		// Update camera (orbit around camTarget)
		m.config.Camera.Pos = core.Vec3{
		X: m.camTarget.X + math.Sin(m.camAngleY)*math.Cos(m.camAngleX)*m.camDist,
		Y: m.camTarget.Y + math.Sin(m.camAngleX)*m.camDist,
		Z: m.camTarget.Z - math.Cos(m.camAngleY)*math.Cos(m.camAngleX)*m.camDist,
		}
		m.config.Camera.Target = m.camTarget
		m.config.Time = m.time

		// Animated light
		m.config.LightDir = core.V(
			math.Sin(m.time*0.5)*0.5,
			0.8,
			math.Cos(m.time*0.5)*0.5-0.5,
		).Normalize()

		// Resize viewport if needed (panel animation may have changed width)
		cw := m.contentWidth()
		vh := m.viewportHeight()
		if m.config.Width != cw || m.config.Height != vh {
			m.config.Resize(cw, vh)
		}

		// Recording: capture keyframe during live recording
		if m.recState == recorder.RecordLive && m.recorder != nil {
			m.recorder.CaptureKeyframe(&m)
		}

		// Recording: bake step (one frame per tick)
		if m.recState == recorder.RecordBaking && m.recorder != nil {
			// Save current renderer state
			savedW, savedH := m.config.Width, m.config.Height
			savedTime := m.config.Time
			savedCam := m.config.Camera
			savedLight := m.config.LightDir
			savedContrast := m.config.Contrast
			savedAmbient := m.config.Ambient
			savedSpec := m.config.SpecPower
			savedShadow := m.config.ShadowSteps
			savedAO := m.config.AOSteps

			done := m.recorder.BakeStep(&m)

			// Restore renderer state
			m.config.Resize(savedW, savedH)
			m.config.Time = savedTime
			m.config.Camera = savedCam
			m.config.LightDir = savedLight
			m.config.Contrast = savedContrast
			m.config.Ambient = savedAmbient
			m.config.SpecPower = savedSpec
			m.config.ShadowSteps = savedShadow
			m.config.AOSteps = savedAO

			if done {
				err := m.recorder.Finalize()
				if err != nil {
					m.recMessage = fmt.Sprintf("Error: %v", err)
				} else {
					m.recMessage = fmt.Sprintf("Saved %s", m.recorder.OutputPath)
				}
				m.recMessageTime = time.Now()
				m.recState = recorder.RecordDone
			}
		}

		// Check for file changes (polls every 500ms)
		m.checkFileChanged()

		// Render frame
		m.frame = m.gpu.Render(m.config)

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

	// Sidebar mouse interaction (all views)
	sidebarWidth := m.sidebar.Width()
	sbResult := m.sidebar.HandleMouse(msg, hh)
	if sbResult.ToggleClicked {
		cmd := m.sidebar.ToggleExpanded()
		m.resizeViewport()
		return m, cmd
	}
	if sbResult.ItemClicked != "" {
		for i, item := range m.sidebar.Items() {
			if item.ID == sbResult.ItemClicked {
				m.switchView(ViewMode(i))
				return m, nil
			}
		}
	}
	if sbResult.HoverChanged {
		return m, nil
	}

	// Non-shader views: forward mouse to the active view's scrollable content
	if m.viewMode != ViewShader {
		viewportLeft := sidebarWidth + 2
		viewportTop := hh
		switch m.viewMode {
		case ViewGallery:
			sv := m.gallery.ScrollView()
			sv.SetPosition(viewportLeft, viewportTop)
			sv.HandleMouse(msg)
		case ViewHelp:
			sv := m.helpView.ScrollView()
			sv.SetPosition(viewportLeft, viewportTop)
			sv.HandleMouse(msg)
		case ViewPlayer:
			if !m.playerView.Loaded {
				sv := m.playerView.ScrollView()
				sv.SetPosition(viewportLeft, viewportTop)
				sv.HandleMouse(msg)
			}
		}
		return m, nil
	}

	// Right panel resize
	if m.rightPanel.Width() > 0 || m.rightPanel.Animating() {
		rpEdgeX := m.width - m.rightPanel.Width()
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

	// Right panel: controls mouse interaction (clicks, drags, hover)
	if m.rightPanel.IsExpanded() {
		rpX := m.width - m.rightPanel.Width() + 2
		rpY := hh
		m.rightPanel.ScrollView().SetPosition(rpX, rpY)

		// Controls zoned interaction (click/hover on sliders, scene, gpu)
		if m.controls.HandleMouse(msg, &m) {
			return m, nil
		}

		// Scrollbar interaction
		if m.rightPanel.HandleMouseEvent(msg) {
			return m, nil
		}
	}

	// Bottom panel (editor) scroll
	if m.bottomPanel.IsExpanded() {
		middleH := m.height - hh - footerHeight()
		bpY := hh + middleH - m.bottomPanel.Height() + 2 // +2 for separator + title row
		bpX := sidebarWidth + 2 + 1                      // sidebar + left gap + left padding
		m.editor.ScrollView.SetPosition(bpX, bpY)
		if m.editor.ScrollView.HandleMouse(msg) {
			return m, nil
		}
	}

	// Region selection mouse handling — only consume if interacting with the region
	if m.recState == recorder.RecordSelecting && m.regionSelector != nil {
		viewportLeft := sidebarWidth + 2
		vpX := msg.X - viewportLeft
		vpY := msg.Y - hh

		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft {
				if m.regionSelector.HandleMousePress(vpX, vpY) {
					return m, nil
				}
			}
		case tea.MouseActionMotion:
			if m.regionSelector.IsDragging() {
				m.regionSelector.HandleMouseDrag(vpX, vpY)
				return m, nil
			}
		case tea.MouseActionRelease:
			if m.regionSelector.IsDragging() {
				m.regionSelector.HandleMouseRelease()
				return m, nil
			}
		}
	}

	// Viewport mouse (camera drag + zoom)
	// Only handle if within viewport area
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
		if inViewport {
			if msg.Button == tea.MouseButtonLeft {
				m.mouseDrag = true
				m.mouseLastX = msg.X
				m.mouseLastY = msg.Y
				m.autoRotate = false
			} else if msg.Button == tea.MouseButtonRight {
				m.mousePan = true
				m.mouseLastX = msg.X
				m.mouseLastY = msg.Y
			}
		}
	case tea.MouseActionRelease:
		m.mouseDrag = false
		m.mousePan = false
	case tea.MouseActionMotion:
		if m.mouseDrag {
			dx := msg.X - m.mouseLastX
			dy := msg.Y - m.mouseLastY
			m.camAngleY += float64(dx) * 0.02
			m.camAngleX = core.Clamp(m.camAngleX+float64(dy)*0.05, -math.Pi/2+0.1, math.Pi/2-0.1)
			m.mouseLastX = msg.X
			m.mouseLastY = msg.Y
		}
		if m.mousePan {
			dx := msg.X - m.mouseLastX
			dy := msg.Y - m.mouseLastY
			// Pan in camera's right/up plane
			right := core.Vec3{X: math.Cos(m.camAngleY), Y: 0, Z: math.Sin(m.camAngleY)}
			up := core.V(0, 1, 0)
			panSpeed := m.camDist * 0.01
			m.camTarget = m.camTarget.Add(right.Mul(float64(dx) * panSpeed))
			m.camTarget = m.camTarget.Add(up.Mul(float64(dy) * panSpeed * 2.2))
			m.mouseLastX = msg.X
			m.mouseLastY = msg.Y
		}
	}

	if inViewport {
		if msg.Button == tea.MouseButtonWheelUp {
			m.camDist = core.Clamp(m.camDist*0.92, 0.5, 30)
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.camDist = core.Clamp(m.camDist/0.92, 0.5, 30)
		}
	}

	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys (all views)
	switch key {
	case "ctrl+c":
		if m.profiling {
			pprof.StopCPUProfile()
			m.profFile.Close()
		}
		return m, tea.Quit
	case "f1":
		m.switchView(ViewShader)
		return m, nil
	case "f2":
		m.switchView(ViewPlayer)
		return m, nil
	case "f3":
		m.switchView(ViewGallery)
		return m, nil
	case "f4":
		m.switchView(ViewHelp)
		return m, nil
	}

	// Non-shader views: dispatch to view-specific handler
	switch m.viewMode {
	case ViewGallery:
		switch key {
		case "q", "esc":
			return m, tea.Quit
		}
		sel := m.gallery.HandleKey(key, len(scene.Scenes))
		if sel >= 0 {
			m.scene = sel
			m.time = 0
			m.syncSceneGLSL()
			m.switchView(ViewShader)
		}
		return m, nil
	case ViewHelp:
		switch key {
		case "q", "esc":
			return m, tea.Quit
		}
		m.helpView.HandleKey(key)
		return m, nil
	case ViewPlayer:
		switch key {
		case "q":
			return m, tea.Quit
		case "esc":
			if m.playerView.Loaded {
				m.playerView.HandleKey(key)
				return m, nil
			}
			return m, tea.Quit
		}
		m.playerView.HandleKey(key)
		return m, nil
	}

	// Shader view: existing behavior
	switch key {
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

	// Focus-dependent routing (shader view only)
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
	// Region selection mode keys
	if m.recState == recorder.RecordSelecting && m.regionSelector != nil {
		switch key {
		case "enter":
			// Confirm selection, start recording
			rs := m.regionSelector
			scales := recorder.DefaultScales(rs.W, rs.H)
			m.recorder = recorder.NewRecorder(rs.X, rs.Y, rs.W, rs.H, scales)
			m.recorder.StartLive()
			m.recState = recorder.RecordLive
			rs.Recording = true
			return m, nil
		case "esc":
			// Cancel selection
			m.recState = recorder.RecordIdle
			m.regionSelector = nil
			return m, nil
		case "1":
			m.regionSelector.SetPreset(1, m.contentWidth(), m.viewportHeight())
			return m, nil
		case "2":
			m.regionSelector.SetPreset(2, m.contentWidth(), m.viewportHeight())
			return m, nil
		case "3":
			m.regionSelector.SetPreset(3, m.contentWidth(), m.viewportHeight())
			return m, nil
		case "4":
			m.regionSelector.SetPreset(4, m.contentWidth(), m.viewportHeight())
			return m, nil
		}
		// Don't fall through to other keys while selecting
		return m, nil
	}

	switch key {
	case "o":
		switch m.recState {
		case recorder.RecordIdle, recorder.RecordDone:
			// Enter region selection
			m.regionSelector = recorder.NewRegionSelector(m.contentWidth(), m.viewportHeight())
			m.recState = recorder.RecordSelecting
			m.recMessage = ""
		case recorder.RecordLive:
			// Stop recording, start bake
			m.recorder.StartBake()
			m.recState = recorder.RecordBaking
			m.regionSelector = nil
		}
		return m, nil

	case "q", "esc":
		if m.profiling {
			pprof.StopCPUProfile()
			m.profFile.Close()
		}
		return m, tea.Quit
	case "left", "h":
		m.camAngleY -= 0.15
		m.autoRotate = false
	case "right", "l":
		m.camAngleY += 0.15
		m.autoRotate = false
	case "up", "k":
		m.camAngleX = core.Clamp(m.camAngleX+0.1, -math.Pi/2+0.1, math.Pi/2-0.1)
		m.autoRotate = false
	case "down", "j":
		m.camAngleX = core.Clamp(m.camAngleX-0.1, -math.Pi/2+0.1, math.Pi/2-0.1)
		m.autoRotate = false
	case "+", "=":
		m.camDist = core.Clamp(m.camDist*0.92, 0.5, 30)
	case "-", "_":
		m.camDist = core.Clamp(m.camDist/0.92, 0.5, 30)
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
		m.scene = (m.scene + 1) % len(scene.Scenes)
		m.time = 0
		m.syncSceneGLSL()
	case "shift+tab", "N":
		m.scene = (m.scene - 1 + len(scene.Scenes)) % len(scene.Scenes)
		m.time = 0
		m.syncSceneGLSL()
	case " ":
		m.paused = !m.paused
	case "a":
		m.autoRotate = !m.autoRotate
	case "m":
		m.config.RenderMode = (m.config.RenderMode + 1) % core.RenderModeCount
	case "M":
		m.config.RenderMode = (m.config.RenderMode + core.RenderModeCount - 1) % core.RenderModeCount
	case "p":
		if !m.profiling {
			fname := fmt.Sprintf("cpu_%d.prof", time.Now().Unix())
			f, err := os.Create(fname)
			if err != nil {
				m.recMessage = fmt.Sprintf("Profile error: %v", err)
				m.recMessageTime = time.Now()
				break
			}
			pprof.StartCPUProfile(f)
			m.profiling = true
			m.profFile = f
			m.recMessage = fmt.Sprintf("Profiling started → %s", fname)
			m.recMessageTime = time.Now()
		} else {
			pprof.StopCPUProfile()
			fname := m.profFile.Name()
			m.profFile.Close()
			m.profiling = false
			m.profFile = nil
			m.recMessage = fmt.Sprintf("Saved %s", fname)
			m.recMessageTime = time.Now()
		}
	case "[":
		m.config.Contrast = core.Clamp(m.config.Contrast-0.25, 0.5, 5.0)
	case "]":
		m.config.Contrast = core.Clamp(m.config.Contrast+0.25, 0.5, 5.0)
	case "1":
		m.config.Spread = core.Clamp(m.config.Spread+0.25, 0.25, 3.0)
	case "!":
		m.config.Spread = core.Clamp(m.config.Spread-0.25, 0.25, 3.0)
	case "2":
		m.config.ExtDist = core.Clamp(m.config.ExtDist+0.25, 0.25, 3.0)
	case "@":
		m.config.ExtDist = core.Clamp(m.config.ExtDist-0.25, 0.25, 3.0)
	case "3":
		m.config.Ambient = core.Clamp(m.config.Ambient+0.05, 0.0, 1.0)
	case "#":
		m.config.Ambient = core.Clamp(m.config.Ambient-0.05, 0.0, 1.0)
	case "4":
		m.config.SpecPower = core.Clamp(m.config.SpecPower*1.5, 4, 128)
	case "$":
		m.config.SpecPower = core.Clamp(m.config.SpecPower/1.5, 4, 128)
	case "5":
		m.config.ShadowSteps = min(m.config.ShadowSteps+4, 48)
	case "%":
		m.config.ShadowSteps = max(m.config.ShadowSteps-4, 0)
	case "6":
		m.config.AOSteps = min(m.config.AOSteps+1, 10)
	case "^":
		m.config.AOSteps = max(m.config.AOSteps-1, 0)
	case "r":
		m.camAngleX = 0
		m.camAngleY = 0
		m.camDist = 4.0
		m.camTarget = core.V(0, 0, 0)
		m.autoRotate = false
		m.config.Contrast = 1.25
		m.config.Spread = 0.75
		m.config.ExtDist = 1.0
		m.config.Ambient = 0.6
		m.config.SpecPower = 9.0
		m.config.ShadowSteps = 8
		m.config.AOSteps = 2
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
		m.syncCompileErr()
		return m, nil
	}

	_, cmd := m.editor.Update(msg)
	return m, cmd
}

// switchView changes the active view mode and updates the sidebar.
func (m *model) switchView(mode ViewMode) {
	m.viewMode = mode
	ids := []string{"view-shader", "view-player", "view-gallery", "view-help"}
	if int(mode) < len(ids) {
		m.sidebar.SetActiveID(ids[mode])
	}
	if mode == ViewPlayer {
		m.playerView.ScanFiles()
	}
	if mode == ViewGallery {
		m.gallery.Selected = m.scene
	}
}

// syncSceneGLSL populates the editor with scene GLSL or Chisel code if available.
func (m *model) syncSceneGLSL() {
	if m.scene < 0 || m.scene >= len(scene.Scenes) {
		return
	}
	s := scene.Scenes[m.scene]

	// Set up file watching
	m.watchFile = s.FilePath
	m.watchModTime = time.Time{}
	if s.FilePath != "" {
		if info, err := os.Stat(s.FilePath); err == nil {
			m.watchModTime = info.ModTime()
		}
	}

	// Prefer Chisel source if available
	if s.Chisel != "" {
		m.editor.SetChiselCode(s.Chisel)
		if m.gpu != nil {
			m.editor.Compile(m.gpu)
		}
		m.syncCompileErr()
		return
	}

	if s.GLSL != "" {
		m.editor.ChiselMode = false
		m.editor.SetCode(s.GLSL)
		if m.gpu != nil {
			err := m.gpu.CompileUserCode(s.GLSL)
			if err != nil {
				m.editor.Status = fmt.Sprintf("Error: %s", err.Error())
				m.editor.StatusErr = true
			} else {
				m.editor.Status = "Compiled OK"
				m.editor.StatusErr = false
			}
		}
		m.syncCompileErr()
	}
}

// syncCompileErr updates the persistent compile error from the editor status.
func (m *model) syncCompileErr() {
	if m.editor.StatusErr {
		m.compileErr = m.editor.Status
	} else {
		m.compileErr = ""
	}
}

// checkFileChanged polls the current scene's source file for changes.
func (m *model) checkFileChanged() {
	if m.watchFile == "" {
		return
	}
	now := time.Now()
	if now.Sub(m.watchCheck) < 500*time.Millisecond {
		return
	}
	m.watchCheck = now

	info, err := os.Stat(m.watchFile)
	if err != nil {
		return
	}
	if !info.ModTime().After(m.watchModTime) {
		return
	}
	m.watchModTime = info.ModTime()

	data, err := os.ReadFile(m.watchFile)
	if err != nil {
		return
	}
	content := string(data)

	// Update scene and editor
	s := &scene.Scenes[m.scene]
	isChisel := strings.HasSuffix(m.watchFile, ".chisel")
	if isChisel {
		s.Chisel = content
		m.editor.SetChiselCode(content)
	} else {
		s.GLSL = content
		m.editor.ChiselMode = false
		m.editor.SetCode(content)
	}

	// Recompile
	if m.gpu != nil {
		m.editor.Compile(m.gpu)
	}
	m.syncCompileErr()

	m.recMessage = fmt.Sprintf("Reloaded %s", filepath.Base(m.watchFile))
	m.recMessageTime = time.Now()
}

// AppState interface methods for controls and recorder packages.
func (m *model) GetRenderConfig() *core.RenderConfig { return m.config }
func (m *model) GetGPU() *gpupkg.GPURenderer         { return m.gpu }
func (m *model) GetScene() int                       { return m.scene }
func (m *model) SetScene(v int)                      { m.scene = v }
func (m *model) NumScenes() int                      { return len(scene.Scenes) }
func (m *model) SceneName(i int) string              { return scene.Scenes[i].Name }
func (m *model) GetTime() float64                    { return m.time }
func (m *model) SetTime(v float64)                   { m.time = v }
func (m *model) SyncSceneGLSL()                      { m.syncSceneGLSL() }
func (m *model) GetCamAngleX() float64               { return m.camAngleX }
func (m *model) GetCamAngleY() float64               { return m.camAngleY }
func (m *model) GetCamDist() float64                 { return m.camDist }
func (m *model) GetCamTarget() core.Vec3             { return m.camTarget }

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
	m.config.Resize(cw, vh)
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	// --- Header ---
	var headerTitle, rightInfo string
	rpWidth := 0 // right panel width for header layout

	switch m.viewMode {
	case ViewShader:
		modeStr := "SHAPES"
		switch m.config.RenderMode {
		case core.RenderDual:
			modeStr = "DUAL"
		case core.RenderBlocks:
			modeStr = "BLOCK"
		case core.RenderHalfBlock:
			modeStr = "HALF"
		case core.RenderBraille:
			modeStr = "BRAILLE"
		case core.RenderDensity:
			modeStr = "DENSITY"
		}
		pauseStr := ""
		if m.paused {
			pauseStr = " PAUSED"
		}
		// Recording indicator
		recStr := ""
		recStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF6600")).Bold(true)
		switch m.recState {
		case recorder.RecordSelecting:
			recStr = " | " + recStyle.Render("SELECT REGION")
		case recorder.RecordLive:
			if m.recorder != nil {
				dur := m.recorder.RecordingDuration()
				recStr = " | " + recStyle.Render(fmt.Sprintf("● REC %.1fs", dur.Seconds()))
			}
		case recorder.RecordBaking:
			if m.recorder != nil {
				cur, total := m.recorder.BakeProgress()
				pct := 0
				if total > 0 {
					pct = cur * 100 / total
				}
				recStr = " | " + recStyle.Render(fmt.Sprintf("Baking %d%%", pct))
			}
		case recorder.RecordDone:
			if m.recMessage != "" && time.Since(m.recMessageTime) < 3*time.Second {
				recStr = " | " + recStyle.Render("✓ "+m.recMessage)
			}
		}
		// Show transient messages (e.g. profiling) when no recording status is displayed
		if recStr == "" && m.recMessage != "" && time.Since(m.recMessageTime) < 3*time.Second {
			recStr = " | " + recStyle.Render(m.recMessage)
		}
		// Show persistent compile error
		if recStr == "" && m.compileErr != "" {
			errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF3333")).Bold(true)
			recStr = " | " + errStyle.Render("✗ "+m.compileErr)
		}
		s := scene.Scenes[m.scene]
		fileType := ""
		if s.Chisel != "" {
			fileType = " [chisel]"
		} else if s.GLSL != "" {
			fileType = " [glsl]"
		}
		headerTitle = fmt.Sprintf("ASCII Shader  ·  %s%s", s.Name, fileType)
		rightInfo = fmt.Sprintf("%s | %.0f fps%s%s", modeStr, m.fps, pauseStr, recStr)
		rpWidth = m.rightPanel.Width()
	case ViewPlayer:
		headerTitle = "ASCII Shader  ·  Player"
	case ViewGallery:
		headerTitle = "ASCII Shader  ·  Gallery"
	case ViewHelp:
		headerTitle = "ASCII Shader  ·  Help"
	}

	header := layout.ComposeHeader(headerTitle, rightInfo, m.width, m.sidebar.Width(), rpWidth)

	// --- Footer ---
	var bindings []layout.FooterBinding
	focusStr := ""

	switch m.viewMode {
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
		switch m.focus {
		case FocusControls:
			focusStr = "[Controls]"
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
		if m.playerView.Loaded {
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
	footer := layout.RenderFooter(bindings, m.width, focusStr)

	// --- Middle section ---
	hh := headerHeight()
	fh := footerHeight()
	middleHeight := m.height - hh - fh
	if middleHeight < 1 {
		middleHeight = 1
	}

	cw := m.contentWidth()

	// Build viewport content based on view mode
	vpHeight := middleHeight
	if m.viewMode == ViewShader && m.bottomPanel.Height() > 0 {
		vpHeight -= m.bottomPanel.Height()
	}
	if vpHeight < 1 {
		vpHeight = 1
	}

	var viewContentBlock string

	switch m.viewMode {
	case ViewShader:
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

		// Region selection / recording overlay
		if m.regionSelector != nil && (m.recState == recorder.RecordSelecting || m.recState == recorder.RecordLive) {
			if m.recState == recorder.RecordLive && m.recorder != nil {
				dur := m.recorder.RecordingDuration()
				m.regionSelector.RecLabel = fmt.Sprintf("● REC %.1fs", dur.Seconds())
			}
			viewLines = m.regionSelector.RenderOverlay(viewLines)
		}

		viewContentBlock = strings.Join(viewLines, "\n")

		// Append bottom panel below viewport content if expanded
		if m.bottomPanel.Height() > 0 {
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

	case ViewGallery:
		sceneNames := make([]string, len(scene.Scenes))
		for i, s := range scene.Scenes {
			sceneNames[i] = s.Name
		}
		viewContentBlock = m.gallery.Render(cw, vpHeight, sceneNames)

	case ViewHelp:
		viewContentBlock = m.helpView.Render(cw, vpHeight)

	case ViewPlayer:
		viewContentBlock = m.playerView.Render(cw, vpHeight)
	}

	// Right panel content (shader view only)
	var rightPanelStr string
	if m.viewMode == ViewShader && m.rightPanel.Width() > 0 {
		m.controls.SyncFromRenderConfig(m.config)
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

	return zone.Scan(header + "\n" + middle + "\n" + footer)
}

func main() {
	runtime.LockOSThread()

	// Check for --play flag
	if len(os.Args) >= 3 && os.Args[1] == "--play" {
		runPlayer(os.Args[2])
		return
	}

	zone.NewGlobal()

	scene.LoadShaderFiles()

	m := initialModel()

	// Initialize GPU renderer (required)
	gpuRenderer, gpuErr := gpupkg.NewGPURenderer()
	if gpuErr != nil {
		fmt.Fprintf(os.Stderr, "GPU init failed: %v\n", gpuErr)
		os.Exit(1)
	}
	m.gpu = gpuRenderer
	m.syncSceneGLSL()
	defer gpuRenderer.Destroy()

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

// runPlayer runs the standalone clip player.
func runPlayer(path string) {
	c, err := clip.LoadClip(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading clip: %v\n", err)
		os.Exit(1)
	}

	player := clip.NewPlayer(c)
	player.SetLoop(true)

	pm := playerModel{
		player:    player,
		clipPath:  path,
		lastFrame: time.Now(),
	}

	p := tea.NewProgram(
		pm,
		tea.WithAltScreen(),
	)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// playerModel is the Bubble Tea model for standalone playback.
type playerModel struct {
	player    *clip.Player
	clipPath  string
	width     int
	height    int
	lastFrame time.Time
}

func (pm playerModel) Init() tea.Cmd {
	return tea.Batch(
		tick(),
		tea.EnterAltScreen,
	)
}

func (pm playerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		pm.width = msg.Width
		pm.height = msg.Height
		pm.player.SetSize(pm.width, pm.height-1) // -1 for status line
		return pm, nil

	case tickMsg:
		now := time.Now()
		dt := now.Sub(pm.lastFrame).Seconds()
		if dt < 1.0/63.0 {
			return pm, tick()
		}
		pm.lastFrame = now
		pm.player.Tick(dt)
		return pm, tick()

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			return pm, tea.Quit
		case " ":
			pm.player.Paused = !pm.player.Paused
		case "l":
			pm.player.SetLoop(!pm.player.Loop)
		}
		return pm, nil
	}
	return pm, nil
}

func (pm playerModel) View() string {
	if pm.width == 0 || pm.height == 0 {
		return "Loading..."
	}

	frame := pm.player.Render()

	// Status line
	status := fmt.Sprintf(" Playing: %s | Frame %d/%d | q: quit | space: pause | l: loop",
		pm.clipPath, pm.player.CurrentFrame+1, len(pm.player.Clip().Tracks[pm.player.ScaleIdx].Frames))

	statusStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#333333")).
		Foreground(lipgloss.Color("#CCCCCC")).
		Width(pm.width)

	return frame + "\n" + statusStyle.Render(status)
}
