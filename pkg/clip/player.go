package clip

import (
	"strconv"
	"strings"
)

// Player plays back a loaded .asciirec clip.
type Player struct {
	clip         *Clip
	ScaleIdx     int     // which scale track to use
	CurrentFrame int     // current frame index
	timeAcc      float64 // accumulated time in seconds
	frameDur     float64 // duration of one frame in seconds
	Loop         bool
	Paused       bool
	width        int // available display width
	height       int // available display height
}

// NewPlayer creates a player for the given clip.
func NewPlayer(c *Clip) *Player {
	fps := float64(c.Header.FPS)
	if fps <= 0 {
		fps = 30
	}
	return &Player{
		clip:     c,
		frameDur: 1.0 / fps,
	}
}

// SetSize sets the available display area and selects the best scale track.
func (p *Player) SetSize(w, h int) {
	p.width = w
	p.height = h
	p.pickScale()
}

// Clip returns the underlying clip.
func (p *Player) Clip() *Clip {
	return p.clip
}

// pickScale selects the largest scale track that fits within (width, height).
// If none fits, uses the smallest scale.
func (p *Player) pickScale() {
	bestIdx := 0
	bestArea := 0

	for i, se := range p.clip.Scales {
		sw := int(se.Width)
		sh := int(se.Height)
		if sw <= p.width && sh <= p.height {
			area := sw * sh
			if area > bestArea {
				bestArea = area
				bestIdx = i
			}
		}
	}

	// If nothing fits, use smallest
	if bestArea == 0 {
		smallestArea := int(p.clip.Scales[0].Width) * int(p.clip.Scales[0].Height)
		for i, se := range p.clip.Scales {
			area := int(se.Width) * int(se.Height)
			if area < smallestArea {
				smallestArea = area
				bestIdx = i
			}
		}
	}

	p.ScaleIdx = bestIdx
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

	track := p.clip.Tracks[p.ScaleIdx]
	numFrames := len(track.Frames)
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
	track := p.clip.Tracks[p.ScaleIdx]
	if frame < 0 {
		frame = 0
	}
	if frame >= len(track.Frames) {
		frame = len(track.Frames) - 1
	}
	p.CurrentFrame = frame
	p.timeAcc = 0
}

// Render builds the ANSI string for the current frame.
func (p *Player) Render() string {
	track := p.clip.Tracks[p.ScaleIdx]
	if len(track.Frames) == 0 || p.CurrentFrame >= len(track.Frames) {
		return ""
	}

	frame := track.Frames[p.CurrentFrame]
	w := track.Width
	h := track.Height

	// Build ANSI output (same approach as render.go)
	out := make([]byte, 0, w*h*20+h*10)
	prevR, prevG, prevB := -1, -1, -1

	for y := 0; y < h; y++ {
		if y > 0 {
			out = append(out, '\n')
		}
		prevR, prevG, prevB = -1, -1, -1

		for x := 0; x < w; x++ {
			idx := y*w + x
			c := frame[idx]

			if c.Ch == ' ' || c.Ch == 0 {
				out = append(out, ' ')
				continue
			}

			cr, cg, cb := RGB565Decode(c.Color)

			if int(cr) != prevR || int(cg) != prevG || int(cb) != prevB {
				out = append(out, "\033[38;2;"...)
				out = strconv.AppendInt(out, int64(cr), 10)
				out = append(out, ';')
				out = strconv.AppendInt(out, int64(cg), 10)
				out = append(out, ';')
				out = strconv.AppendInt(out, int64(cb), 10)
				out = append(out, 'm')
				prevR, prevG, prevB = int(cr), int(cg), int(cb)
			}
			out = append(out, c.Ch)
		}
		out = append(out, "\033[0m"...)
	}

	// Center in available space if larger than the track
	rendered := string(out)
	if p.width > w || p.height > h {
		rendered = p.centerContent(rendered, w, h)
	}

	return rendered
}

// centerContent centers the rendered frame in the available display area.
func (p *Player) centerContent(content string, contentW, contentH int) string {
	lines := strings.Split(content, "\n")

	// Vertical centering
	padTop := 0
	if p.height > contentH {
		padTop = (p.height - contentH) / 2
	}

	// Horizontal centering
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
