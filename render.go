package main

import (
	"math"
	"strconv"
	"sync"
	"unicode/utf8"
)

const (
	maxSteps  = 80
	maxDist   = 50.0
	surfDist  = 0.005
	normalEps = 0.001
)

// Render modes
const (
	RenderShapes = 0 // shape matching, fg only (default)
	RenderBlocks = 1 // quadrant blocks, fg+bg
	RenderDual   = 2 // shape matching, fg+bg
)

// ASCII ramp from dark to bright
var asciiRamp = []byte(" .`'^\",:;Il!i><~+_-?][}{1)(|/tfjrxnuvczXYUJCLQ0OZmwqpdbkhao*#MW&8%B@$")

// Camera holds the view parameters
type Camera struct {
	Pos    Vec3
	Target Vec3
	Up     Vec3
	FOV    float64
}

// Renderer does the raymarching
type Renderer struct {
	Width, Height int
	Camera        Camera
	Scene         func(Vec3, float64) float64
	ColorFunc     func(Vec3, float64) Vec3 // material color at point (nil = white)
	Time          float64
	LightDir      Vec3
	RenderMode    int // RenderShapes, RenderBlocks, or RenderDual
	ShapeTable    *ShapeTable
	Contrast      float64
	Spread        float64 // sub-sample spread multiplier (1.0 = default)
	ExtDist       float64 // external sample distance multiplier (1.0 = default)
	Ambient       float64 // ambient light level (0-1)
	SpecPower     float64 // specular exponent
	ShadowSteps   int     // shadow ray steps (0 = off, 32 = full)
	AOSteps       int     // ambient occlusion samples (0 = off, 5 = full)
	buf           []byte
}

func NewRenderer(w, h int) *Renderer {
	return &Renderer{
		Width:  w,
		Height: h,
		Camera: Camera{
			Pos:    V(0, 0, -4),
			Target: V(0, 0, 0),
			Up:     V(0, 1, 0),
			FOV:    60,
		},
		LightDir: V(-0.5, 0.8, -0.6).Normalize(),
		buf:      make([]byte, w*h),
	}
}

func (r *Renderer) Resize(w, h int) {
	r.Width = w
	r.Height = h
	r.buf = make([]byte, w*h)
}

// March a ray, return distance traveled and total steps
func (r *Renderer) raymarch(ro, rd Vec3) (float64, int) {
	t := 0.0
	for i := 0; i < maxSteps; i++ {
		p := ro.Add(rd.Mul(t))
		d := r.Scene(p, r.Time)
		if d < surfDist {
			return t, i
		}
		t += d
		if t > maxDist {
			break
		}
	}
	return maxDist, maxSteps
}

// Compute surface normal via central differences
func (r *Renderer) normal(p Vec3) Vec3 {
	e := normalEps
	t := r.Time
	d := r.Scene(p, t)
	return Vec3{
		r.Scene(V(p.X+e, p.Y, p.Z), t) - d,
		r.Scene(V(p.X, p.Y+e, p.Z), t) - d,
		r.Scene(V(p.X, p.Y, p.Z+e), t) - d,
	}.Normalize()
}

// Soft shadow
func (r *Renderer) softShadow(ro, rd Vec3, mint, maxt, k float64) float64 {
	if r.ShadowSteps <= 0 {
		return 1.0
	}
	res := 1.0
	t := mint
	for i := 0; i < r.ShadowSteps; i++ {
		p := ro.Add(rd.Mul(t))
		d := r.Scene(p, r.Time)
		if d < surfDist*0.5 {
			return 0.0
		}
		res = math.Min(res, k*d/t)
		t += clamp(d, 0.02, 0.2)
		if t > maxt {
			break
		}
	}
	return clamp(res, 0, 1)
}

// Ambient occlusion
func (r *Renderer) ao(p, n Vec3) float64 {
	if r.AOSteps <= 0 {
		return 1.0
	}
	occ := 0.0
	scale := 1.0
	for i := 0; i < r.AOSteps; i++ {
		h := 0.01 + 0.12*float64(i)
		d := r.Scene(p.Add(n.Mul(h)), r.Time)
		occ += (h - d) * scale
		scale *= 0.75
	}
	return clamp(1-1.5*occ, 0, 1)
}

// shade computes brightness 0..1 using ShadowSteps/AOSteps from the Renderer.
func (r *Renderer) shade(ro, rd Vec3, t float64) float64 {
	p := ro.Add(rd.Mul(t))
	n := r.normal(p)

	// Diffuse
	diff := clamp(n.Dot(r.LightDir), 0, 1)

	// Shadow + specular (skip entirely when ShadowSteps=0)
	shadow := r.softShadow(p.Add(n.Mul(0.02)), r.LightDir, 0.02, 10.0, 16.0)
	diff *= shadow

	spec := 0.0
	if r.ShadowSteps > 0 {
		half := r.LightDir.Sub(rd).Normalize()
		spec = math.Pow(clamp(n.Dot(half), 0, 1), r.SpecPower) * shadow
	}

	// AO + fresnel (skip when AOSteps=0)
	ao := r.ao(p, n)
	fresnel := 0.0
	if r.AOSteps > 0 {
		fresnel = math.Pow(1-clamp(-rd.Dot(n), 0, 1), 3) * 0.3
	}

	ambient := r.Ambient * ao
	brightness := ambient + diff*0.65*ao + spec*0.25 + fresnel*ao

	fog := math.Exp(-t * t * 0.008)
	brightness *= fog

	return clamp(brightness, 0, 1)
}

// shadeColor returns RGB color using the same lighting pipeline as shade().
func (r *Renderer) shadeColor(ro, rd Vec3, t float64) Vec3 {
	p := ro.Add(rd.Mul(t))
	n := r.normal(p)

	mat := V(1, 1, 1)
	if r.ColorFunc != nil {
		mat = r.ColorFunc(p, r.Time)
	}

	diff := clamp(n.Dot(r.LightDir), 0, 1)

	shadow := r.softShadow(p.Add(n.Mul(0.02)), r.LightDir, 0.02, 10.0, 16.0)
	diff *= shadow

	spec := 0.0
	if r.ShadowSteps > 0 {
		half := r.LightDir.Sub(rd).Normalize()
		spec = math.Pow(clamp(n.Dot(half), 0, 1), r.SpecPower) * shadow
	}

	ao := r.ao(p, n)
	fresnel := 0.0
	if r.AOSteps > 0 {
		fresnel = math.Pow(1-clamp(-rd.Dot(n), 0, 1), 3) * 0.3
	}

	ambient := r.Ambient * ao
	diffContrib := diff * 0.65 * ao
	col := mat.Mul(ambient + diffContrib)
	col = col.Add(V(1, 1, 1).Mul(spec * 0.25))
	col = col.Add(mat.Mul(fresnel * ao))

	fog := math.Exp(-t * t * 0.008)
	col = col.Mul(fog)

	return V(clamp(col.X, 0, 1), clamp(col.Y, 0, 1), clamp(col.Z, 0, 1))
}

// raymarchFrom starts a raymarch near a known surface hit.
// It begins at startT - 0.5 and uses half the normal max steps.
func (r *Renderer) raymarchFrom(ro, rd Vec3, startT float64) (float64, bool) {
	t := math.Max(0, startT-0.5)
	steps := maxSteps / 2
	for i := 0; i < steps; i++ {
		p := ro.Add(rd.Mul(t))
		d := r.Scene(p, r.Time)
		if d < surfDist {
			return t, true
		}
		t += d
		if t > maxDist {
			break
		}
	}
	return maxDist, false
}

// sampleBrightness casts a ray and returns its brightness (0 if missed).
func (r *Renderer) sampleBrightness(ro, fwd, right, up Vec3, snx, sny, halfW, halfH, hintT float64) float64 {
	rd := fwd.Add(right.Mul(snx * halfW)).Add(up.Mul(sny * halfH)).Normalize()
	t, hit := r.raymarchFrom(ro, rd, hintT)
	if hit {
		return r.shade(ro, rd, t)
	}
	return 0
}

// renderCellShaped casts 6 internal + 6 external sub-cell rays,
// applies directional and global contrast, then matches against shape vectors.
// Returns the matched character and the center-ray color.
func (r *Renderer) renderCellShaped(ro, fwd, right, up Vec3, nx, ny, dx, dy, halfW, halfH, centerT float64, centerRd Vec3) (byte, Vec3) {
	if centerT >= maxDist {
		return ' ', Vec3{}
	}

	// Internal offsets: 2 cols x 3 rows within the cell
	// Row order: top, middle, bottom (positive ny = up = top of cell)
	s := r.Spread
	colOff := [2]float64{-0.25 * s, 0.25 * s}
	rowOff := [3]float64{1.0 / 3.0 * s, 0, -1.0 / 3.0 * s}

	// External offsets: just outside cell boundary, paired with each internal position
	e := r.ExtDist
	extColOff := [2]float64{-0.75 * e, 0.75 * e}
	extRowOff := [3]float64{1.0 * e, 0, -1.0 * e}

	var sv, ext ShapeVec
	idx := 0
	for row := 0; row < 3; row++ {
		for col := 0; col < 2; col++ {
			// Internal sample
			sv[idx] = r.sampleBrightness(ro, fwd, right, up,
				nx+colOff[col]*dx, ny+rowOff[row]*dy, halfW, halfH, centerT)
			// External sample (further out in the same direction)
			ext[idx] = r.sampleBrightness(ro, fwd, right, up,
				nx+extColOff[col]*dx, ny+extRowOff[row]*dy, halfW, halfH, centerT)
			idx++
		}
	}

	// Average brightness from sub-samples (before contrast for shape matching)
	avgBright := 0.0
	for i := 0; i < 6; i++ {
		avgBright += sv[i]
	}
	avgBright /= 6

	sv = DirectionalContrast(sv, ext, r.Contrast)
	sv = EnhanceContrast(sv, r.Contrast)
	ch := r.ShapeTable.Match(sv)

	// Color: material color * average brightness (no extra shade pass)
	mat := V(1, 1, 1)
	if r.ColorFunc != nil {
		p := ro.Add(centerRd.Mul(centerT))
		mat = r.ColorFunc(p, r.Time)
	}
	col := mat.Mul(avgBright)
	return ch, V(clamp(col.X, 0, 1), clamp(col.Y, 0, 1), clamp(col.Z, 0, 1))
}

// renderCellDual is like renderCellShaped but over a double-sized cell.
// dx/dy should be the cell spacing of the dual grid (2× normal).
func (r *Renderer) renderCellDual(ro, fwd, right, up Vec3, nx, ny, dx, dy, halfW, halfH, centerT float64, centerRd Vec3) (byte, Vec3) {
	if centerT >= maxDist {
		return ' ', Vec3{}
	}

	s := r.Spread
	colOff := [2]float64{-0.25 * s, 0.25 * s}
	rowOff := [3]float64{1.0 / 3.0 * s, 0, -1.0 / 3.0 * s}

	e := r.ExtDist
	extColOff := [2]float64{-0.75 * e, 0.75 * e}
	extRowOff := [3]float64{1.0 * e, 0, -1.0 * e}

	var sv, ext ShapeVec
	idx := 0
	for row := 0; row < 3; row++ {
		for c := 0; c < 2; c++ {
			sv[idx] = r.sampleBrightness(ro, fwd, right, up,
				nx+colOff[c]*dx, ny+rowOff[row]*dy, halfW, halfH, centerT)
			ext[idx] = r.sampleBrightness(ro, fwd, right, up,
				nx+extColOff[c]*dx, ny+extRowOff[row]*dy, halfW, halfH, centerT)
			idx++
		}
	}

	avgBright := 0.0
	for i := 0; i < 6; i++ {
		avgBright += sv[i]
	}
	avgBright /= 6

	sv = DirectionalContrast(sv, ext, r.Contrast)
	sv = EnhanceContrast(sv, r.Contrast)
	ch := r.ShapeTable.Match(sv)

	mat := V(1, 1, 1)
	if r.ColorFunc != nil {
		p := ro.Add(centerRd.Mul(centerT))
		mat = r.ColorFunc(p, r.Time)
	}
	col := mat.Mul(avgBright)
	return ch, V(clamp(col.X, 0, 1), clamp(col.Y, 0, 1), clamp(col.Z, 0, 1))
}

// quadrantChars maps 4-bit patterns (TL=bit3, TR=bit2, BL=bit1, BR=bit0) to Unicode quadrant block characters.
var quadrantChars = [16]rune{
	' ', '▗', '▖', '▄', '▝', '▐', '▞', '▟',
	'▘', '▚', '▌', '▙', '▀', '▜', '▛', '█',
}

// renderCellQuadrant casts 4 rays in a 2×2 pattern via raymarchFrom,
// thresholds brightness, picks a quadrant block character, and computes fg/bg colors.
func (r *Renderer) renderCellQuadrant(ro, fwd, right, up Vec3, nx, ny, dx, dy, halfW, halfH, centerT float64) cell {
	if centerT >= maxDist {
		return cell{ch: ' '}
	}

	// 2×2 sub-pixel offsets within the cell
	offX := [2]float64{-0.25, 0.25}
	offY := [2]float64{0.25, -0.25} // top, bottom (positive ny = up)

	var bright [4]float64
	var colors [4]Vec3
	hit := [4]bool{}

	// Sample order: TL(0), TR(1), BL(2), BR(3)
	idx := 0
	for row := 0; row < 2; row++ {
		for col := 0; col < 2; col++ {
			snx := nx + offX[col]*dx
			sny := ny + offY[row]*dy
			rd := fwd.Add(right.Mul(snx * halfW)).Add(up.Mul(sny * halfH)).Normalize()
			t, ok := r.raymarchFrom(ro, rd, centerT)
			if ok {
				colors[idx] = r.shadeColor(ro, rd, t)
				bright[idx] = colors[idx].X*0.299 + colors[idx].Y*0.587 + colors[idx].Z*0.114
				hit[idx] = true
			}
			idx++
		}
	}

	// Mean brightness across all 4 (misses contribute 0, pulling mean down
	// so that hit pixels are above mean → correct silhouette pattern)
	mean := (bright[0] + bright[1] + bright[2] + bright[3]) / 4

	// Uniform surface check: all 4 hit with similar brightness → full block
	if hit[0] && hit[1] && hit[2] && hit[3] {
		minB, maxB := bright[0], bright[0]
		for i := 1; i < 4; i++ {
			if bright[i] < minB {
				minB = bright[i]
			}
			if bright[i] > maxB {
				maxB = bright[i]
			}
		}
		if maxB-minB < 0.08 {
			avg := colors[0].Add(colors[1]).Add(colors[2]).Add(colors[3]).Mul(0.25)
			return cell{ch: '█', col: V(clamp(avg.X, 0, 1), clamp(avg.Y, 0, 1), clamp(avg.Z, 0, 1))}
		}
	}

	// Threshold each pixel around mean
	var pattern int
	var onCount int
	var fgCol, bgCol Vec3
	bgHitCount := 0
	for i := 0; i < 4; i++ {
		bit := 3 - i // TL=bit3, TR=bit2, BL=bit1, BR=bit0
		if hit[i] && bright[i] > mean {
			pattern |= 1 << uint(bit)
			fgCol = fgCol.Add(colors[i])
			onCount++
		} else if hit[i] {
			bgCol = bgCol.Add(colors[i])
			bgHitCount++
		}
	}

	if onCount == 0 {
		return cell{ch: ' '}
	}

	ch := quadrantChars[pattern]
	fg := fgCol.Mul(1.0 / float64(onCount))
	fg = V(clamp(fg.X, 0, 1), clamp(fg.Y, 0, 1), clamp(fg.Z, 0, 1))

	// Only set bg when off-pixels hit actual geometry (not ray misses)
	if bgHitCount == 0 {
		return cell{ch: ch, col: fg}
	}

	bg := bgCol.Mul(1.0 / float64(bgHitCount))
	bg = V(clamp(bg.X, 0, 1), clamp(bg.Y, 0, 1), clamp(bg.Z, 0, 1))

	return cell{ch: ch, col: fg, bg: bg, hasBg: true}
}

// cell holds a character and its foreground/background colors.
type cell struct {
	ch    rune
	col   Vec3 // foreground RGB 0-1
	bg    Vec3 // background RGB 0-1
	hasBg bool // whether to emit background color escape
}

// RenderCells renders the scene and returns the raw cell grid (no ANSI encoding).
func (r *Renderer) RenderCells() [][]cell {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return nil
	}

	// Dual mode: half-resolution grid, each cell covers 2×2 normal cells
	if r.RenderMode == RenderDual && r.ShapeTable != nil {
		return r.renderCellsDualGrid()
	}

	fwd := r.Camera.Target.Sub(r.Camera.Pos).Normalize()
	right := fwd.Cross(r.Camera.Up).Normalize()
	up := right.Cross(fwd)

	fovRad := r.Camera.FOV * math.Pi / 180
	halfH := math.Tan(fovRad / 2)
	aspect := float64(w) / float64(h) * 0.45
	halfW := halfH * aspect

	ro := r.Camera.Pos
	dx := 2.0 / float64(w-1)
	dy := 2.0 / float64(h-1)
	shapeMode := r.ShapeTable != nil

	var wg sync.WaitGroup
	lines := make([][]cell, h)

	for y := 0; y < h; y++ {
		wg.Add(1)
		go func(y int) {
			defer wg.Done()
			line := make([]cell, w)
			ny := 1.0 - 2.0*float64(y)/float64(h-1)

			for x := 0; x < w; x++ {
				nx := 2.0*float64(x)/float64(w-1) - 1.0
				rd := fwd.Add(right.Mul(nx * halfW)).Add(up.Mul(ny * halfH)).Normalize()
				t, _ := r.raymarch(ro, rd)

				if r.RenderMode == RenderBlocks {
					line[x] = r.renderCellQuadrant(ro, fwd, right, up, nx, ny, dx, dy, halfW, halfH, t)
				} else {
					var ch byte
					var col Vec3
					if shapeMode {
						ch, col = r.renderCellShaped(ro, fwd, right, up, nx, ny, dx, dy, halfW, halfH, t, rd)
					} else if t < maxDist {
						col = r.shadeColor(ro, rd, t)
						brightness := col.X*0.299 + col.Y*0.587 + col.Z*0.114
						idx := int(brightness * float64(len(asciiRamp)-1))
						if idx < 0 {
							idx = 0
						}
						if idx >= len(asciiRamp) {
							idx = len(asciiRamp) - 1
						}
						ch = asciiRamp[idx]
					} else {
						bgBright := 0.02 + 0.03*(ny+1)*0.5
						idx := int(bgBright * float64(len(asciiRamp)-1))
						if idx < 0 {
							idx = 0
						}
						if idx >= len(asciiRamp) {
							idx = len(asciiRamp) - 1
						}
						ch = asciiRamp[idx]
						col = Vec3{}
					}
					line[x] = cell{ch: rune(ch), col: col}
				}
			}
			lines[y] = line
		}(y)
	}
	wg.Wait()
	return lines
}

// renderCellsDualGrid renders W × H cells (fills viewport) but each cell's
// sub-pixel grid covers 2× the normal area, capturing broader spatial features.
func (r *Renderer) renderCellsDualGrid() [][]cell {
	w, h := r.Width, r.Height

	fwd := r.Camera.Target.Sub(r.Camera.Pos).Normalize()
	right := fwd.Cross(r.Camera.Up).Normalize()
	up := right.Cross(fwd)

	fovRad := r.Camera.FOV * math.Pi / 180
	halfH := math.Tan(fovRad / 2)
	aspect := float64(w) / float64(h) * 0.45
	halfW := halfH * aspect

	ro := r.Camera.Pos
	// 2× the normal cell spacing so sub-pixels spread wider
	dx := 2.0 / float64(w-1) * 2
	dy := 2.0 / float64(h-1) * 2

	var wg sync.WaitGroup
	lines := make([][]cell, h)

	for y := 0; y < h; y++ {
		wg.Add(1)
		go func(y int) {
			defer wg.Done()
			line := make([]cell, w)
			ny := 1.0 - 2.0*float64(y)/float64(h-1)

			for x := 0; x < w; x++ {
				nx := 2.0*float64(x)/float64(w-1) - 1.0
				rd := fwd.Add(right.Mul(nx * halfW)).Add(up.Mul(ny * halfH)).Normalize()
				t, _ := r.raymarch(ro, rd)
				ch, col := r.renderCellDual(ro, fwd, right, up, nx, ny, dx, dy, halfW, halfH, t, rd)
				line[x] = cell{ch: rune(ch), col: col}
			}
			lines[y] = line
		}(y)
	}
	wg.Wait()
	return lines
}

// buildANSI converts a cell grid to an ANSI true-color string with fg+bg support.
func buildANSI(lines [][]cell) string {
	if len(lines) == 0 {
		return ""
	}
	w := len(lines[0])
	h := len(lines)
	out := make([]byte, 0, w*h*20+h*10)
	prevFR, prevFG, prevFB := -1, -1, -1
	prevBR, prevBG, prevBB := -1, -1, -1
	var runeBuf [utf8.UTFMax]byte
	for y := 0; y < h; y++ {
		if y > 0 {
			out = append(out, '\n')
		}
		prevFR, prevFG, prevFB = -1, -1, -1
		prevBR, prevBG, prevBB = -1, -1, -1
		for x := 0; x < len(lines[y]); x++ {
			c := lines[y][x]
			if c.ch == ' ' && !c.hasBg {
				if prevBR != -1 || prevBG != -1 || prevBB != -1 {
					out = append(out, "\033[0m"...)
					prevFR, prevFG, prevFB = -1, -1, -1
					prevBR, prevBG, prevBB = -1, -1, -1
				}
				out = append(out, ' ')
				continue
			}
			// If previous cell set a bg but this cell doesn't use bg, reset it
			if !c.hasBg && (prevBR != -1 || prevBG != -1 || prevBB != -1) {
				out = append(out, "\033[0m"...)
				prevFR, prevFG, prevFB = -1, -1, -1
				prevBR, prevBG, prevBB = -1, -1, -1
			}

			cr := int(c.col.X * 255)
			cg := int(c.col.Y * 255)
			cb := int(c.col.Z * 255)
			fgChanged := cr != prevFR || cg != prevFG || cb != prevFB
			bgChanged := false
			var br, bg2, bb int
			if c.hasBg {
				br = int(c.bg.X * 255)
				bg2 = int(c.bg.Y * 255)
				bb = int(c.bg.Z * 255)
				bgChanged = br != prevBR || bg2 != prevBG || bb != prevBB
			}
			if fgChanged || bgChanged {
				out = append(out, "\033["...)
				if fgChanged {
					out = append(out, "38;2;"...)
					out = strconv.AppendInt(out, int64(cr), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(cg), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(cb), 10)
					prevFR, prevFG, prevFB = cr, cg, cb
				}
				if bgChanged {
					if fgChanged {
						out = append(out, ';')
					}
					out = append(out, "48;2;"...)
					out = strconv.AppendInt(out, int64(br), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(bg2), 10)
					out = append(out, ';')
					out = strconv.AppendInt(out, int64(bb), 10)
					prevBR, prevBG, prevBB = br, bg2, bb
				}
				out = append(out, 'm')
			}
			n := utf8.EncodeRune(runeBuf[:], c.ch)
			out = append(out, runeBuf[:n]...)
		}
		out = append(out, "\033[0m"...)
	}
	return string(out)
}

// Render the full frame, returns ANSI true-color string
func (r *Renderer) Render() string {
	if r.Width <= 0 || r.Height <= 0 {
		return ""
	}
	return buildANSI(r.RenderCells())
}
