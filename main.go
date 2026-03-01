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
	return model{
		renderer:  r,
		camDist:   4.0,
		scene:     0,
		lastFrame: time.Now(),
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

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height - 3 // reserve space for HUD
		if m.height < 4 {
			m.height = 4
		}
		m.renderer.Resize(m.width, m.height)
		return m, nil

	case tickMsg:
		now := time.Now()
		dt := now.Sub(m.lastFrame).Seconds()

		// Cap at 60fps — skip if less than ~16.6ms elapsed
		if dt < 1.0/61.0 {
			return m, tick()
		}

		if dt > 0 {
			m.fps = m.fps*0.9 + (1.0/dt)*0.1 // smoothed FPS
		}
		m.lastFrame = now

		if !m.paused {
			m.time += dt
		}

		if m.autoRotate && !m.paused {
			m.camAngleY += dt * 0.3
		}

		// Update camera position
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

		// Render frame
		if m.gpuMode && m.gpu != nil {
			m.frame = m.gpu.Render(m.renderer)
		} else {
			m.frame = m.renderer.Render()
		}

		return m, tick()

	case tea.MouseMsg:
		switch msg.Action {
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft {
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
		if msg.Button == tea.MouseButtonWheelUp {
			m.camDist = clamp(m.camDist-0.3, 1.5, 12)
		} else if msg.Button == tea.MouseButtonWheelDown {
			m.camDist = clamp(m.camDist+0.3, 1.5, 12)
		}

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit

		// Camera controls
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

		// Zoom
		case "+", "=":
			m.camDist = clamp(m.camDist-0.3, 1.5, 12)
		case "-", "_":
			m.camDist = clamp(m.camDist+0.3, 1.5, 12)

		// Scene switching
		case "tab", "n":
			m.scene = (m.scene + 1) % len(scenes)
			m.time = 0
		case "shift+tab", "N":
			m.scene = (m.scene - 1 + len(scenes)) % len(scenes)
			m.time = 0

		// Toggle
		case " ":
			m.paused = !m.paused
		case "a":
			m.autoRotate = !m.autoRotate
		case "g":
			if m.gpu != nil {
				m.gpuMode = !m.gpuMode
			}
		// Tunable parameters
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
	}

	return m, nil
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	var sb strings.Builder

	// Frame
	sb.WriteString(m.frame)
	sb.WriteString("\n")

	// HUD
	pauseStr := ""
	if m.paused {
		pauseStr = " ⏸ PAUSED"
	}

	gpuStr := "CPU"
	if m.gpuMode && m.gpu != nil {
		gpuStr = "GPU"
	}

	hud := fmt.Sprintf(" 🎬 [%d/%d] %s%s │ %s │ C:%.1f S:%.1f E:%.1f A:%.2f P:%.0f Sh:%d AO:%d │ 🎯 %.0f fps",
		m.scene+1, len(scenes), scenes[m.scene].Name, pauseStr, gpuStr,
		m.renderer.Contrast, m.renderer.Spread, m.renderer.ExtDist,
		m.renderer.Ambient, m.renderer.SpecPower,
		m.renderer.ShadowSteps, m.renderer.AOSteps, m.fps)

	controls := " []:contrast  1:spread  2:extDist  3:ambient  4:specPow  5:shadow  6:AO  g:GPU  (shift+N to decrease)  r:reset  q:quit"

	hudStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("245")).
		Bold(true)

	ctrlStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240"))

	sb.WriteString(hudStyle.Render(hud))
	sb.WriteString("\n")
	sb.WriteString(ctrlStyle.Render(controls))

	return sb.String()
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
