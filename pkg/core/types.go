package core

import "math"

// Render modes
const (
	RenderShapes    = 0 // shape matching, fg only (default)
	RenderBraille   = 1 // braille 2×4 dot grid
	RenderBlocks    = 2 // quadrant blocks, fg+bg
	RenderSlice     = 3 // 2D SDF slice heatmap
	RenderCost      = 4 // raymarching step count heatmap
	RenderModeCount = 5
)

// Default luminosity compensation. Shapes (ASCII) is the reference.
// Factor >1 brightens with contrast enhancement, <1 dims linearly.
const (
	DefaultBlockGamma   = 0.6
	DefaultBrailleGamma = 1.3
)

// CompensateColor adjusts perceived brightness while preserving hue.
// For factor < 1 (dimming): simple linear luminance scale.
// For factor > 1 (brightening): applies L^power × factor where power
// grows with factor. This simultaneously boosts brightness and enhances
// contrast — darks stay dark, brights get pushed up — compensating for
// the lack of character-density-based brightness in braille/block modes.
func CompensateColor(c Vec3, factor float64) Vec3 {
	if factor == 1.0 {
		return c
	}
	lum := c.X*0.299 + c.Y*0.587 + c.Z*0.114
	if lum < 1e-10 {
		return c
	}
	var target float64
	if factor < 1.0 {
		// Simple linear dim
		target = lum * factor
	} else {
		// Contrast-enhancing boost: power grows with factor so darks
		// stay dark (mimicking sparse-character dimming in shapes mode)
		// while the multiplier pushes brights up.
		power := 1.0 + (factor-1.0)*0.5
		target = math.Pow(lum, power) * factor
		if target > 1.0 {
			target = 1.0
		}
	}
	scale := target / lum
	return Vec3{
		X: Clamp(c.X*scale, 0, 1),
		Y: Clamp(c.Y*scale, 0, 1),
		Z: Clamp(c.Z*scale, 0, 1),
	}
}

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
	BlockGamma    float64 // luminosity compensation for block renderer
	BrailleGamma  float64 // luminosity compensation for braille renderer
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
		LightDir:     V(-0.5, 0.8, -0.6).Normalize(),
		OrthoScale:   3.0,
		BlockGamma:   DefaultBlockGamma,
		BrailleGamma: DefaultBrailleGamma,
	}
}

// Resize updates the config dimensions.
func (rc *RenderConfig) Resize(w, h int) {
	rc.Width = w
	rc.Height = h
}
