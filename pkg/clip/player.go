package clip

import (
	"strings"

	"asciishader/pkg/core"
)

// Player plays back a loaded .asciirec clip.
type Player struct {
	clip         *Clip
	CurrentFrame int
	timeAcc      float64
	frameDur     float64
	Loop         bool
	Paused       bool
	width        int
	height       int
	RenderMode   int
	Contrast     float64
	BlockGamma   float64
	BrailleGamma float64
	shapeTable   *core.ShapeTable

	// Render cache — avoids re-running shape matching when nothing changed
	cachedFrame        int
	cachedMode         int
	cachedContrast     float64
	cachedBlockGamma   float64
	cachedBrailleGamma float64
	cachedWidth        int
	cachedHeight       int
	cachedOutput       string
	ansiBuf            []byte
}

// NewPlayer creates a player for the given clip.
func NewPlayer(c *Clip) *Player {
	fps := float64(c.Header.FPS)
	if fps <= 0 {
		fps = 30
	}
	return &Player{
		clip:         c,
		frameDur:     1.0 / fps,
		Contrast:     1.0,
		BlockGamma:   core.DefaultBlockGamma,
		BrailleGamma: core.DefaultBrailleGamma,
		cachedFrame:  -1,
	}
}

// SetSize sets the available display area.
func (p *Player) SetSize(w, h int) {
	p.width = w
	p.height = h
}

// Clip returns the underlying clip.
func (p *Player) Clip() *Clip {
	return p.clip
}

// SetLoop toggles looping.
func (p *Player) SetLoop(loop bool) {
	p.Loop = loop
}

// Tick advances playback by dt seconds.
func (p *Player) Tick(dt float64) {
	if p.Paused {
		return
	}

	numFrames := len(p.clip.Frames)
	if numFrames == 0 {
		return
	}

	p.timeAcc += dt
	for p.timeAcc >= p.frameDur {
		p.timeAcc -= p.frameDur
		p.CurrentFrame++
		if p.CurrentFrame >= numFrames {
			if p.Loop {
				p.CurrentFrame = 0
			} else {
				p.CurrentFrame = numFrames - 1
				p.Paused = true
			}
		}
	}
}

// Seek jumps to a specific frame.
func (p *Player) Seek(frame int) {
	if frame < 0 {
		frame = 0
	}
	if frame >= len(p.clip.Frames) {
		frame = len(p.clip.Frames) - 1
	}
	p.CurrentFrame = frame
	p.timeAcc = 0
}

// Render reconstructs cells from the current frame's sub-pixel data
// and builds an ANSI-colored string. Results are cached to avoid
// redundant shape matching when the frame/mode/size haven't changed.
func (p *Player) Render() string {
	if len(p.clip.Frames) == 0 || p.CurrentFrame >= len(p.clip.Frames) {
		return ""
	}

	// Return cached result if nothing changed
	if p.CurrentFrame == p.cachedFrame &&
		p.RenderMode == p.cachedMode &&
		p.Contrast == p.cachedContrast &&
		p.BlockGamma == p.cachedBlockGamma &&
		p.BrailleGamma == p.cachedBrailleGamma &&
		p.width == p.cachedWidth &&
		p.height == p.cachedHeight {
		return p.cachedOutput
	}

	// Lazily initialize shape table (needed for Shapes mode)
	if p.shapeTable == nil {
		p.shapeTable = core.NewShapeTable()
	}

	w := p.clip.Width
	h := p.clip.Height

	// Only reconstruct cells if the frame or mode changed
	if p.CurrentFrame != p.cachedFrame ||
		p.RenderMode != p.cachedMode ||
		p.Contrast != p.cachedContrast ||
		p.BlockGamma != p.cachedBlockGamma ||
		p.BrailleGamma != p.cachedBrailleGamma {

		frame := p.clip.Frames[p.CurrentFrame]
		cells := CellsFromFrame(frame, w, h, p.RenderMode, p.shapeTable, p.Contrast, p.BlockGamma, p.BrailleGamma)
		p.ansiBuf = core.AppendANSI(p.ansiBuf[:0], cells)
	}

	rendered := string(p.ansiBuf)

	if p.width > w || p.height > h {
		rendered = p.centerContent(rendered, w, h)
	}

	p.cachedFrame = p.CurrentFrame
	p.cachedMode = p.RenderMode
	p.cachedContrast = p.Contrast
	p.cachedBlockGamma = p.BlockGamma
	p.cachedBrailleGamma = p.BrailleGamma
	p.cachedWidth = p.width
	p.cachedHeight = p.height
	p.cachedOutput = rendered

	return rendered
}

// centerContent centers the rendered frame in the available display area.
func (p *Player) centerContent(content string, contentW, contentH int) string {
	lines := strings.Split(content, "\n")

	padTop := 0
	if p.height > contentH {
		padTop = (p.height - contentH) / 2
	}

	padLeft := 0
	if p.width > contentW {
		padLeft = (p.width - contentW) / 2
	}

	var out strings.Builder
	leftPad := strings.Repeat(" ", padLeft)

	for i := 0; i < padTop; i++ {
		out.WriteString(strings.Repeat(" ", p.width))
		out.WriteByte('\n')
	}

	for i, line := range lines {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(leftPad)
		out.WriteString(line)
	}

	return out.String()
}
