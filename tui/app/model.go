package app

import (
	"os"
	"time"

	"asciishader/pkg/core"
	gpupkg "asciishader/pkg/gpu"
	"asciishader/pkg/recorder"
	"asciishader/pkg/scene"
	"asciishader/tui/components"
	"asciishader/tui/controls"
	"asciishader/tui/editor"
	"asciishader/tui/layout"
	"asciishader/tui/views"

	tea "charm.land/bubbletea/v2"
)

// Model is the top-level BubbleTea model for the AsciiShader TUI.
type Model struct {
	Config          *core.RenderConfig
	GPU             *gpupkg.GPURenderer
	Width           int
	Height          int
	Time            float64
	Scene           int
	Paused          bool
	CamAngleY       float64
	CamAngleX       float64
	CamAngleYTarget float64
	CamAngleXTarget float64
	CamDist         float64
	CamDistTarget   float64
	CamTarget       core.Vec3
	CamTargetTarget core.Vec3
	AutoRotate      bool
	MouseLastX      int
	MouseLastY      int
	MouseDrag       bool
	MousePan        bool
	LastClickTime   time.Time
	LastClickX      int
	LastClickY      int
	FPS             float64
	LastFrame       time.Time
	Frame           string

	// Layout components
	Sidebar     *layout.Sidebar
	RightPanel  *layout.RightPanel
	BottomPanel *layout.BottomPanel
	Controls    *controls.ControlsTab
	SceneTree   *components.TreeView
	Editor      *editor.EditorTab
	Focus       FocusZone

	// View switching
	Mode       ViewMode
	Gallery    *views.GalleryView
	HelpView   *views.HelpView
	PlayerView *views.PlayerView

	// Recording
	Recorder       *recorder.Recorder
	RecState       recorder.RecordingState
	RegionSelector *recorder.RegionSelector
	RecMessage     string
	RecMessageTime time.Time
	CompileErr     string

	// File watching
	WatchFile    string
	WatchModTime time.Time
	WatchCheck   time.Time

	// Profiling
	Profiling bool
	ProfFile  *os.File

	// Image mode (Kitty graphics)
	ImageTransmit  string // Kitty APC escape to send via tea.Raw
	ImageSupported bool   // whether terminal reports pixel dimensions
	ImageGPUMs     float64
	ImageZlibMs    float64
	ImageB64Ms     float64
	ImageShmMode   bool // true when using shm path
	FPSHovered     bool
}

// NewModel creates the initial application model with default settings.
func NewModel() Model {
	rc := core.NewRenderConfig(80, 24)
	cellW, cellH, _ := core.GetCellPixelSize()
	rc.CellPixelW = cellW
	rc.CellPixelH = cellH
	rc.Contrast = 1.25
	rc.Spread = 0.75
	rc.ExtDist = 1.0
	rc.Ambient = 0.6
	rc.SpecPower = 9.0
	rc.ShadowSteps = 8
	rc.AOSteps = 2

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

	return Model{
		Config:         rc,
		ImageSupported: cellW > 0 && cellH > 0,
		CamDist:        4.0,
		CamDistTarget:  4.0,
		Scene:          0,
		LastFrame:      time.Now(),
		Sidebar:        sb,
		RightPanel:     rp,
		BottomPanel:    bp,
		Controls:       controls.NewControlsTab(),
		SceneTree:      components.NewTreeView(),
		Editor:         editor.NewEditorTab(),
		Focus:          FocusViewport,
		Mode:           ViewShader,
		Gallery:        views.NewGalleryView(),
		HelpView:       views.NewHelpView(),
		PlayerView:     views.NewPlayerView(),
	}
}

func (m Model) Init() tea.Cmd {
	return Tick()
}

// Tick returns a command that fires a TickMsg after ~1ms.
func Tick() tea.Cmd {
	return tea.Tick(time.Millisecond, func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// HeaderHeight returns the height of the header.
func HeaderHeight() int { return 3 }

// FooterHeight returns the height of the footer.
func FooterHeight() int { return 1 }

// ContentWidth returns the width available for the main content area.
func (m Model) ContentWidth() int {
	w := m.Width - m.Sidebar.Width() - 2
	if m.Mode == ViewShader && m.RightPanel.Width() > 0 {
		w -= m.RightPanel.Width() + 2
	}
	if w < 1 {
		w = 1
	}
	return w
}

// ViewportHeight returns the height available for the viewport.
func (m Model) ViewportHeight() int {
	middleHeight := m.Height - HeaderHeight() - FooterHeight()
	if middleHeight < 1 {
		middleHeight = 1
	}
	if m.Mode == ViewShader && m.BottomPanel.Height() > 0 {
		middleHeight -= m.BottomPanel.Height()
	}
	if middleHeight < 1 {
		middleHeight = 1
	}
	return middleHeight
}

// ResizeViewport updates the renderer dimensions based on current layout.
func (m *Model) ResizeViewport() {
	cw := m.ContentWidth()
	vh := m.ViewportHeight()
	if cw < 1 {
		cw = 1
	}
	if vh < 1 {
		vh = 1
	}
	m.Config.Resize(cw, vh)
}

// UsesImageRenderer returns true if the current render mode uses the Kitty image path.
func (m *Model) UsesImageRenderer() bool {
	return m.ImageSupported && (m.Config.RenderMode == core.RenderImage ||
		m.Config.RenderMode == core.RenderSlice ||
		m.Config.RenderMode == core.RenderCost)
}

func (m *Model) GetRenderConfig() *core.RenderConfig { return m.Config }
func (m *Model) GetGPU() *gpupkg.GPURenderer         { return m.GPU }
func (m *Model) GetScene() int                       { return m.Scene }
func (m *Model) SetScene(v int)                      { m.Scene = v }
func (m *Model) NumScenes() int                      { return len(scene.Scenes) }
func (m *Model) SceneName(i int) string              { return scene.Scenes[i].Name }
func (m *Model) GetTime() float64                    { return m.Time }
func (m *Model) SetTime(v float64)                   { m.Time = v }
func (m *Model) SyncSceneGLSL()                      { syncSceneGLSL(m) }
func (m *Model) GetCamAngleX() float64               { return m.CamAngleX }
func (m *Model) GetCamAngleY() float64               { return m.CamAngleY }
func (m *Model) GetCamDist() float64                 { return m.CamDist }
func (m *Model) GetCamTarget() core.Vec3             { return m.CamTarget }
func (m *Model) IsAutoRotate() bool                  { return m.AutoRotate }
