package core

// Render modes
const (
	RenderShapes    = 0 // shape matching, fg only (default)
	RenderDual      = 1 // shape matching, fg+bg
	RenderBlocks    = 2 // quadrant blocks, fg+bg
	RenderHalfBlock = 3 // half-block ▀ with fg+bg color
	RenderBraille   = 4 // braille 2×4 dot grid
	RenderDensity   = 5 // classic density ramp
	RenderModeCount = 6
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

// AsciiRamp is the density ramp from dark to bright.
var AsciiRamp = []byte(" .`'^\",:;Il!i><~+_-?][}{1)(|/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$")
