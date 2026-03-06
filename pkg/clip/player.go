package clip

import (
	"strconv"
	"strings"
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

// Render builds the ANSI string for the current frame.
func (p *Player) Render() string {
	if len(p.clip.Frames) == 0 || p.CurrentFrame >= len(p.clip.Frames) {
		return ""
	}

	frame := p.clip.Frames[p.CurrentFrame]
	w := p.clip.Width
	h := p.clip.Height

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

	rendered := string(out)
	if p.width > w || p.height > h {
		rendered = p.centerContent(rendered, w, h)
	}

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
