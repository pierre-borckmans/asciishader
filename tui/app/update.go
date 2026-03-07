package app

import (
	"fmt"
	"log"
	"math"
	"os"
	"runtime/pprof"
	"time"

	"asciishader/pkg/core"
	"asciishader/pkg/recorder"
	"asciishader/pkg/scene"
	"asciishader/tui/components"

	tea "charm.land/bubbletea/v2"
)

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.ResizeViewport()
		return m, nil

	case components.PanelAnimTickMsg:
		switch msg.ID {
		case "sidebar":
			cmd := m.Sidebar.AnimTick()
			m.ResizeViewport()
			return m, cmd
		case "right-panel":
			cmd := m.RightPanel.AnimTick()
			m.ResizeViewport()
			return m, cmd
		case "bottom-panel":
			cmd := m.BottomPanel.AnimTick()
			m.ResizeViewport()
			return m, cmd
		}
		return m, nil

	case TickMsg:
		now := time.Now()
		dt := now.Sub(m.LastFrame).Seconds()

		// Cap at 60fps
		if dt < 1.0/63.0 {
			return m, Tick()
		}

		if dt > 0 {
			m.FPS = m.FPS*0.9 + (1.0/dt)*0.1
		}
		m.LastFrame = now

		// Player view: tick playback
		if m.Mode == ViewPlayer {
			m.PlayerView.Tick(dt)
			return m, Tick()
		}

		// Non-shader views: no rendering needed
		if m.Mode != ViewShader {
			return m, Tick()
		}

		if !m.Paused {
			m.Time += dt
		}

		if m.AutoRotate && !m.Paused {
			m.CamAngleY += dt * 0.3
		}

		// Update camera (orbit around CamTarget)
		m.Config.Camera.Pos = core.Vec3{
			X: m.CamTarget.X + math.Sin(m.CamAngleY)*math.Cos(m.CamAngleX)*m.CamDist,
			Y: m.CamTarget.Y + math.Sin(m.CamAngleX)*m.CamDist,
			Z: m.CamTarget.Z - math.Cos(m.CamAngleY)*math.Cos(m.CamAngleX)*m.CamDist,
		}
		m.Config.Camera.Target = m.CamTarget
		m.Config.Time = m.Time
		m.Config.OrthoScale = m.CamDist * 0.75 // ortho scale tracks zoom
		switch m.Config.RenderMode {
		case core.RenderSlice:
			m.Config.SliceMode = 1
		case core.RenderCost:
			m.Config.SliceMode = 2
		default:
			m.Config.SliceMode = 0
		}

		// Animated light
		m.Config.LightDir = core.V(
			math.Sin(m.Time*0.5)*0.5,
			0.8,
			math.Cos(m.Time*0.5)*0.5-0.5,
		).Normalize()

		// Resize viewport if needed (panel animation may have changed width)
		cw := m.ContentWidth()
		vh := m.ViewportHeight()
		if m.Config.Width != cw || m.Config.Height != vh {
			m.Config.Resize(cw, vh)
		}

		// Recording: capture keyframe during live recording
		if m.RecState == recorder.RecordLive && m.Recorder != nil {
			m.Recorder.CaptureKeyframe(&m)
		}

		// Recording: bake step (one frame per tick)
		if m.RecState == recorder.RecordBaking && m.Recorder != nil {
			// Save current renderer state
			savedW, savedH := m.Config.Width, m.Config.Height
			savedTime := m.Config.Time
			savedCam := m.Config.Camera
			savedLight := m.Config.LightDir
			savedContrast := m.Config.Contrast
			savedAmbient := m.Config.Ambient
			savedSpec := m.Config.SpecPower
			savedShadow := m.Config.ShadowSteps
			savedAO := m.Config.AOSteps

			done := m.Recorder.BakeStep(&m)

			// Restore renderer state
			m.Config.Resize(savedW, savedH)
			m.Config.Time = savedTime
			m.Config.Camera = savedCam
			m.Config.LightDir = savedLight
			m.Config.Contrast = savedContrast
			m.Config.Ambient = savedAmbient
			m.Config.SpecPower = savedSpec
			m.Config.ShadowSteps = savedShadow
			m.Config.AOSteps = savedAO

			if done {
				err := m.Recorder.Finalize()
				if err != nil {
					m.RecMessage = fmt.Sprintf("Error: %v", err)
				} else {
					m.RecMessage = fmt.Sprintf("Saved %s", m.Recorder.OutputPath)
				}
				m.RecMessageTime = time.Now()
				m.RecState = recorder.RecordDone
			}
		}

		// Check for file changes (polls every 500ms)
		checkFileChanged(&m)

		// Render frame
		m.Frame = m.GPU.Render(m.Config)

		return m, Tick()

	case tea.MouseMsg:
		return m.handleMouse(msg)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	hh := HeaderHeight()
	mouse := msg.Mouse()

	// Sidebar mouse interaction (all views)
	sidebarWidth := m.Sidebar.Width()
	sbResult := m.Sidebar.HandleMouse(msg, hh)
	if sbResult.ToggleClicked {
		cmd := m.Sidebar.ToggleExpanded()
		m.ResizeViewport()
		return m, cmd
	}
	if sbResult.ItemClicked != "" {
		for i, item := range m.Sidebar.Items() {
			if item.ID == sbResult.ItemClicked {
				switchView(&m, ViewMode(i))
				return m, nil
			}
		}
	}
	if sbResult.HoverChanged {
		return m, nil
	}

	// Non-shader views: forward mouse to the active view's scrollable content
	if m.Mode != ViewShader {
		viewportLeft := sidebarWidth + 2
		viewportTop := hh
		switch m.Mode {
		case ViewGallery:
			sv := m.Gallery.ScrollView()
			sv.SetPosition(viewportLeft, viewportTop)
			sv.HandleMouse(msg)
		case ViewHelp:
			sv := m.HelpView.ScrollView()
			sv.SetPosition(viewportLeft, viewportTop)
			sv.HandleMouse(msg)
		case ViewPlayer:
			if !m.PlayerView.Loaded {
				sv := m.PlayerView.ScrollView()
				sv.SetPosition(viewportLeft, viewportTop)
				sv.HandleMouse(msg)
			}
		}
		return m, nil
	}

	// Right panel resize
	if m.RightPanel.Width() > 0 || m.RightPanel.Animating() {
		rpEdgeX := m.Width - m.RightPanel.Width()
		m.RightPanel.Resizer().SetEdgePos(rpEdgeX)
		if m.RightPanel.HandleResizeEvent(msg, m.Width) {
			m.ResizeViewport()
			return m, nil
		}
	}

	// Bottom panel resize
	if m.BottomPanel.Height() > 0 || m.BottomPanel.Animating() {
		middleHeight := m.Height - hh - FooterHeight()
		bpEdgeY := hh + middleHeight - m.BottomPanel.Height()
		m.BottomPanel.Resizer().SetEdgePos(bpEdgeY)
		if m.BottomPanel.HandleResizeEvent(msg, m.Height) {
			m.ResizeViewport()
			return m, nil
		}
	}

	// Right panel: controls mouse interaction (clicks, drags, hover)
	if m.RightPanel.IsExpanded() {
		rpX := m.Width - m.RightPanel.Width() + 2
		rpY := hh
		m.RightPanel.ScrollView().SetPosition(rpX, rpY)

		// Controls zoned interaction (click/hover on sliders, scene, gpu)
		if m.Controls.HandleMouse(msg, &m) {
			return m, nil
		}

		// Scrollbar interaction
		if m.RightPanel.HandleMouseEvent(msg) {
			return m, nil
		}
	}

	// Bottom panel (editor) scroll
	if m.BottomPanel.IsExpanded() {
		middleH := m.Height - hh - FooterHeight()
		bpY := hh + middleH - m.BottomPanel.Height() + 2 // +2 for separator + title row
		bpX := sidebarWidth + 2 + 1                      // sidebar + left gap + left padding
		m.Editor.ScrollView.SetPosition(bpX, bpY)
		if m.Editor.ScrollView.HandleMouse(msg) {
			return m, nil
		}
	}

	// Region selection mouse handling — only consume if interacting with the region
	if m.RecState == recorder.RecordSelecting && m.RegionSelector != nil {
		viewportLeft := sidebarWidth + 2
		vpX := mouse.X - viewportLeft
		vpY := mouse.Y - hh

		switch msg.(type) {
		case tea.MouseClickMsg:
			if mouse.Button == tea.MouseLeft {
				if m.RegionSelector.HandleMousePress(vpX, vpY) {
					return m, nil
				}
			}
		case tea.MouseMotionMsg:
			if m.RegionSelector.IsDragging() {
				m.RegionSelector.HandleMouseDrag(vpX, vpY)
				return m, nil
			}
		case tea.MouseReleaseMsg:
			if m.RegionSelector.IsDragging() {
				m.RegionSelector.HandleMouseRelease()
				return m, nil
			}
		}
	}

	// Viewport mouse (camera drag + zoom)
	// Only handle if within viewport area
	viewportLeft := sidebarWidth + 2
	viewportRight := m.Width - m.RightPanel.Width()
	if m.RightPanel.Width() > 0 {
		viewportRight -= 2
	}

	middleHeight := m.Height - hh - FooterHeight()
	vpHeight := middleHeight
	if m.BottomPanel.Height() > 0 {
		vpHeight -= m.BottomPanel.Height()
	}

	inViewport := mouse.X >= viewportLeft && mouse.X < viewportRight &&
		mouse.Y >= hh && mouse.Y < hh+vpHeight

	switch msg.(type) {
	case tea.MouseClickMsg:
		if inViewport {
			if mouse.Button == tea.MouseLeft {
				// Double-click detection: reset camera
				now := time.Now()
				dx, dy := mouse.X-m.LastClickX, mouse.Y-m.LastClickY
				if dx < 0 {
					dx = -dx
				}
				if dy < 0 {
					dy = -dy
				}
				if now.Sub(m.LastClickTime) < 300*time.Millisecond && dx < 3 && dy < 3 {
					m.CamDist = 4.0
					m.CamTarget = core.V(0, 0, 0)
					m.AutoRotate = false
					if m.Config.Projection == core.ProjectionIsometric {
						m.CamAngleY = 0.785 // 45°
						m.CamAngleX = 0.615 // ~35.26°
					} else {
						m.CamAngleX = 0
						m.CamAngleY = 0
					}
					m.LastClickTime = time.Time{}
					return m, nil
				}
				m.LastClickTime = now
				m.LastClickX = mouse.X
				m.LastClickY = mouse.Y

				m.MouseDrag = true
				m.MouseLastX = mouse.X
				m.MouseLastY = mouse.Y
				m.AutoRotate = false
			} else if mouse.Button == tea.MouseRight {
				m.MousePan = true
				m.MouseLastX = mouse.X
				m.MouseLastY = mouse.Y
			}
		}
	case tea.MouseReleaseMsg:
		m.MouseDrag = false
		m.MousePan = false
	case tea.MouseMotionMsg:
		if m.MouseDrag {
			dx := mouse.X - m.MouseLastX
			dy := mouse.Y - m.MouseLastY
			m.CamAngleY += float64(dx) * 0.02
			m.CamAngleX = core.Clamp(m.CamAngleX+float64(dy)*0.05, -math.Pi/2+0.1, math.Pi/2-0.1)
			m.MouseLastX = mouse.X
			m.MouseLastY = mouse.Y
		}
		if m.MousePan {
			dx := mouse.X - m.MouseLastX
			dy := mouse.Y - m.MouseLastY
			// Pan in camera's right/up plane
			right := core.Vec3{X: math.Cos(m.CamAngleY), Y: 0, Z: math.Sin(m.CamAngleY)}
			up := core.V(0, 1, 0)
			panSpeed := m.CamDist * 0.01
			m.CamTarget = m.CamTarget.Add(right.Mul(float64(dx) * panSpeed))
			m.CamTarget = m.CamTarget.Add(up.Mul(float64(dy) * panSpeed * 2.2))
			m.MouseLastX = mouse.X
			m.MouseLastY = mouse.Y
		}
	case tea.MouseWheelMsg:
		if inViewport {
			if mouse.Mod.Contains(tea.ModShift) && m.Config.RenderMode == core.RenderSlice {
				// Shift+scroll moves slice depth along view direction
				fwd := m.Config.Camera.Target.Sub(m.Config.Camera.Pos).Normalize()
				step := m.CamDist * 0.05
				if mouse.Button == tea.MouseWheelUp {
					m.CamTarget = m.CamTarget.Add(fwd.Mul(step))
				} else if mouse.Button == tea.MouseWheelDown {
					m.CamTarget = m.CamTarget.Sub(fwd.Mul(step))
				}
			} else {
				// Normal scroll = zoom
				if mouse.Button == tea.MouseWheelUp {
					m.CamDist = core.Clamp(m.CamDist*0.92, 0.5, 30)
				} else if mouse.Button == tea.MouseWheelDown {
					m.CamDist = core.Clamp(m.CamDist/0.92, 0.5, 30)
				}
			}
		}
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global keys (all views)
	switch key {
	case "ctrl+c":
		if m.Profiling {
			pprof.StopCPUProfile()
			_ = m.ProfFile.Close()
		}
		return m, tea.Quit
	case "f1":
		switchView(&m, ViewShader)
		return m, nil
	case "f2":
		switchView(&m, ViewPlayer)
		return m, nil
	case "f3":
		switchView(&m, ViewGallery)
		return m, nil
	case "f4":
		switchView(&m, ViewHelp)
		return m, nil
	}

	// Non-shader views: dispatch to view-specific handler
	switch m.Mode {
	case ViewGallery:
		switch key {
		case "q", "esc":
			return m, tea.Quit
		}
		sel := m.Gallery.HandleKey(key, len(scene.Scenes))
		if sel >= 0 {
			m.Scene = sel
			m.Time = 0
			syncSceneGLSL(&m)
			switchView(&m, ViewShader)
		}
		return m, nil
	case ViewHelp:
		switch key {
		case "q", "esc":
			return m, tea.Quit
		}
		m.HelpView.HandleKey(key)
		return m, nil
	case ViewPlayer:
		switch key {
		case "q":
			return m, tea.Quit
		case "esc":
			if m.PlayerView.Loaded {
				m.PlayerView.HandleKey(key)
				return m, nil
			}
			return m, tea.Quit
		}
		m.PlayerView.HandleKey(key)
		return m, nil
	}

	// Shader view: existing behavior
	switch key {
	case "s":
		if m.Focus == FocusEditor {
			// Don't intercept 's' when typing in editor
			break
		}
		cmd := m.RightPanel.ToggleExpanded()
		if m.RightPanel.IsExpanded() {
			m.Focus = FocusControls
		} else if m.Focus == FocusControls {
			m.Focus = FocusViewport
		}
		m.ResizeViewport()
		return m, cmd
	case "e":
		if m.Focus == FocusEditor {
			// Don't intercept 'e' when typing in editor
			break
		}
		cmd := m.BottomPanel.ToggleExpanded()
		if m.BottomPanel.IsExpanded() {
			m.Focus = FocusEditor
			m.Editor.Focus()
		} else if m.Focus == FocusEditor {
			m.Focus = FocusViewport
			m.Editor.Blur()
		}
		m.ResizeViewport()
		return m, cmd
	}

	// Focus-dependent routing (shader view only)
	switch m.Focus {
	case FocusViewport:
		return m.handleViewportKey(key)
	case FocusControls:
		return m.handleControlsKey(key)
	case FocusEditor:
		return m.handleEditorKey(msg)
	}

	return m, nil
}

func (m Model) handleViewportKey(key string) (tea.Model, tea.Cmd) {
	// Region selection mode keys
	if m.RecState == recorder.RecordSelecting && m.RegionSelector != nil {
		switch key {
		case "enter":
			// Confirm selection, start recording
			rs := m.RegionSelector
			m.Recorder = recorder.NewRecorder(rs.X, rs.Y, rs.W, rs.H)
			m.Recorder.StartLive()
			m.RecState = recorder.RecordLive
			rs.Recording = true
			return m, nil
		case "esc":
			// Cancel selection
			m.RecState = recorder.RecordIdle
			m.RegionSelector = nil
			return m, nil
		case "1":
			m.RegionSelector.SetPreset(1, m.ContentWidth(), m.ViewportHeight())
			return m, nil
		case "2":
			m.RegionSelector.SetPreset(2, m.ContentWidth(), m.ViewportHeight())
			return m, nil
		case "3":
			m.RegionSelector.SetPreset(3, m.ContentWidth(), m.ViewportHeight())
			return m, nil
		case "4":
			m.RegionSelector.SetPreset(4, m.ContentWidth(), m.ViewportHeight())
			return m, nil
		}
		// Don't fall through to other keys while selecting
		return m, nil
	}

	switch key {
	case "o":
		switch m.RecState {
		case recorder.RecordIdle, recorder.RecordDone:
			// Enter region selection
			m.RegionSelector = recorder.NewRegionSelector(m.ContentWidth(), m.ViewportHeight())
			m.RecState = recorder.RecordSelecting
			m.RecMessage = ""
		case recorder.RecordLive:
			// Stop recording, start bake
			m.Recorder.StartBake()
			m.RecState = recorder.RecordBaking
			m.RegionSelector = nil
		}
		return m, nil

	case "q", "esc":
		if m.Profiling {
			pprof.StopCPUProfile()
			_ = m.ProfFile.Close()
		}
		return m, tea.Quit
	case "left", "h":
		m.CamAngleY -= 0.15
		m.AutoRotate = false
	case "right", "l":
		m.CamAngleY += 0.15
		m.AutoRotate = false
	case "up", "k":
		m.CamAngleX = core.Clamp(m.CamAngleX+0.1, -math.Pi/2+0.1, math.Pi/2-0.1)
		m.AutoRotate = false
	case "down", "j":
		m.CamAngleX = core.Clamp(m.CamAngleX-0.1, -math.Pi/2+0.1, math.Pi/2-0.1)
		m.AutoRotate = false
	case "+", "=":
		m.CamDist = core.Clamp(m.CamDist*0.92, 0.5, 30)
	case "-", "_":
		m.CamDist = core.Clamp(m.CamDist/0.92, 0.5, 30)
	case "tab":
		// Cycle focus: viewport → controls (if open) → editor (if open) → viewport
		if m.RightPanel.IsExpanded() {
			m.Focus = FocusControls
			return m, nil
		} else if m.BottomPanel.IsExpanded() {
			m.Focus = FocusEditor
			m.Editor.Focus()
			return m, nil
		}
	case "n":
		m.Scene = (m.Scene + 1) % len(scene.Scenes)
		m.Time = 0
		syncSceneGLSL(&m)
	case "shift+tab", "N":
		m.Scene = (m.Scene - 1 + len(scene.Scenes)) % len(scene.Scenes)
		m.Time = 0
		syncSceneGLSL(&m)
	case "space":
		m.Paused = !m.Paused
	case "a":
		m.AutoRotate = !m.AutoRotate
	case "m":
		m.Config.RenderMode = (m.Config.RenderMode + 1) % core.RenderModeCount
	case "M":
		m.Config.RenderMode = (m.Config.RenderMode + core.RenderModeCount - 1) % core.RenderModeCount
	case "v":
		m.Config.Projection = (m.Config.Projection + 1) % core.ProjectionCount
		// For isometric, set camera to fixed angle
		if m.Config.Projection == core.ProjectionIsometric {
			m.CamAngleY = 0.785 // 45°
			m.CamAngleX = 0.615 // ~35.26° (arctan(1/√2))
		}
	case "p":
		if !m.Profiling {
			fname := fmt.Sprintf("cpu_%d.prof", time.Now().Unix())
			f, err := os.Create(fname)
			if err != nil {
				m.RecMessage = fmt.Sprintf("Profile error: %v", err)
				m.RecMessageTime = time.Now()
				break
			}
			if err := pprof.StartCPUProfile(f); err != nil {
				log.Printf("pprof.StartCPUProfile: %v", err)
			}
			m.Profiling = true
			m.ProfFile = f
			m.RecMessage = fmt.Sprintf("Profiling started → %s", fname)
			m.RecMessageTime = time.Now()
		} else {
			pprof.StopCPUProfile()
			fname := m.ProfFile.Name()
			_ = m.ProfFile.Close()
			m.Profiling = false
			m.ProfFile = nil
			m.RecMessage = fmt.Sprintf("Saved %s", fname)
			m.RecMessageTime = time.Now()
		}
	case "[":
		m.Config.Contrast = core.Clamp(m.Config.Contrast-0.25, 0.5, 5.0)
	case "]":
		m.Config.Contrast = core.Clamp(m.Config.Contrast+0.25, 0.5, 5.0)
	case "1":
		m.Config.Spread = core.Clamp(m.Config.Spread+0.25, 0.25, 3.0)
	case "!":
		m.Config.Spread = core.Clamp(m.Config.Spread-0.25, 0.25, 3.0)
	case "2":
		m.Config.ExtDist = core.Clamp(m.Config.ExtDist+0.25, 0.25, 3.0)
	case "@":
		m.Config.ExtDist = core.Clamp(m.Config.ExtDist-0.25, 0.25, 3.0)
	case "3":
		m.Config.Ambient = core.Clamp(m.Config.Ambient+0.05, 0.0, 1.0)
	case "#":
		m.Config.Ambient = core.Clamp(m.Config.Ambient-0.05, 0.0, 1.0)
	case "4":
		m.Config.SpecPower = core.Clamp(m.Config.SpecPower*1.5, 4, 128)
	case "$":
		m.Config.SpecPower = core.Clamp(m.Config.SpecPower/1.5, 4, 128)
	case "5":
		m.Config.ShadowSteps = min(m.Config.ShadowSteps+4, 48)
	case "%":
		m.Config.ShadowSteps = max(m.Config.ShadowSteps-4, 0)
	case "6":
		m.Config.AOSteps = min(m.Config.AOSteps+1, 10)
	case "^":
		m.Config.AOSteps = max(m.Config.AOSteps-1, 0)
	case "r":
		m.CamDist = 4.0
		m.CamTarget = core.V(0, 0, 0)
		m.AutoRotate = false
		if m.Config.Projection == core.ProjectionIsometric {
			m.CamAngleY = 0.785
			m.CamAngleX = 0.615
		} else {
			m.CamAngleX = 0
			m.CamAngleY = 0
		}
		m.Config.Contrast = 1.25
		m.Config.Spread = 0.75
		m.Config.ExtDist = 1.0
		m.Config.Ambient = 0.6
		m.Config.SpecPower = 9.0
		m.Config.ShadowSteps = 8
		m.Config.AOSteps = 2
		m.Time = 0
	}
	return m, nil
}

func (m Model) handleControlsKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.Focus = FocusViewport
		return m, nil
	case "tab":
		if m.BottomPanel.IsExpanded() {
			m.Focus = FocusEditor
			m.Editor.Focus()
		} else {
			m.Focus = FocusViewport
		}
		return m, nil
	case "q":
		return m, tea.Quit
	}

	if m.Controls.HandleKey(key, &m) {
		return m, nil
	}
	return m, nil
}

func (m Model) handleEditorKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.Focus = FocusViewport
		m.Editor.Blur()
		return m, nil
	case "ctrl+r":
		m.Editor.Compile(m.GPU)
		syncCompileErr(&m)
		return m, nil
	}

	_, cmd := m.Editor.Update(msg)
	return m, cmd
}
