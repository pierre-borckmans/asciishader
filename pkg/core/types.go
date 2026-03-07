package core

// Render modes
const (
	RenderShapes    = 0 // shape matching, fg only (default)
	RenderBlocks    = 1 // quadrant blocks, fg+bg
	RenderBraille   = 2 // braille 2×4 dot grid
	RenderSlice     = 3 // 2D SDF slice heatmap
	RenderCost      = 4 // raymarching step count heatmap
	RenderModeCount = 5
)

// Camera holds the view parameters.
type Camera struct {
	Pos    Vec3
	Target Vec3
	Up     Vec3
	FOV    float64
}

// Cell is a single terminal cell with character and color.
type Cell struct {
	Ch    rune
	Col   Vec3 // foreground RGB 0-1
	Bg    Vec3 // background RGB 0-1
	HasBg bool // whether to emit background color escape
}

// RenderConfig holds all rendering parameters needed by the GPU renderer.
type RenderConfig struct {
	Width, Height int
	Camera        Camera
	Time          float64
	LightDir      Vec3
	RenderMode    int
	Contrast      float64
	Spread        float64
	ExtDist       float64
	Ambient       float64
	SpecPower     float64
	ShadowSteps   int
	AOSteps       int
	Projection    int     // 0=perspective, 1=orthographic, 2=isometric
	OrthoScale    float64 // orthographic view scale
	SliceMode     int     // 0=normal, 1=SDF slice heatmap
	SliceY        float64 // Y position of slice plane
}

// Projection mode constants
const (
	ProjectionPerspective  = 0
	ProjectionOrthographic = 1
	ProjectionIsometric    = 2
	ProjectionCount        = 3
)

// NewRenderConfig creates a RenderConfig with default values.
func NewRenderConfig(w, h int) *RenderConfig {
	return &RenderConfig{
		Width:  w,
		Height: h,
		Camera: Camera{
			Pos:    V(0, 0, -4),
			Target: V(0, 0, 0),
			Up:     V(0, 1, 0),
			FOV:    60,
		},
		LightDir:   V(-0.5, 0.8, -0.6).Normalize(),
		OrthoScale: 3.0,
	}
}

// Resize updates the config dimensions.
func (rc *RenderConfig) Resize(w, h int) {
	rc.Width = w
	rc.Height = h
}
