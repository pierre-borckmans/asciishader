package main

import (
	"math"
	"sync"
)

const (
	maxSteps   = 80
	maxDist    = 50.0
	surfDist   = 0.005
	normalEps  = 0.001
	shadowSteps = 32
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
	Time          float64
	LightDir      Vec3
	ShapeMode     bool
	ShapeTable    *ShapeTable
	Contrast      float64
	Spread        float64 // sub-sample spread multiplier (1.0 = default)
	ExtDist       float64 // external sample distance multiplier (1.0 = default)
	Ambient       float64 // ambient light level (0-1)
	SpecPower     float64 // specular exponent
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
	res := 1.0
	t := mint
	for i := 0; i < shadowSteps; i++ {
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
	occ := 0.0
	scale := 1.0
	for i := 0; i < 5; i++ {
		h := 0.01 + 0.12*float64(i)
		d := r.Scene(p.Add(n.Mul(h)), r.Time)
		occ += (h - d) * scale
		scale *= 0.75
	}
	return clamp(1-1.5*occ, 0, 1)
}

// Shade a point - returns brightness 0..1
func (r *Renderer) shade(ro, rd Vec3, t float64) float64 {
	p := ro.Add(rd.Mul(t))
	n := r.normal(p)

	// Diffuse
	diff := clamp(n.Dot(r.LightDir), 0, 1)

	// Soft shadow
	shadow := r.softShadow(p.Add(n.Mul(0.02)), r.LightDir, 0.02, 10.0, 16.0)
	diff *= shadow

	// Specular (Blinn-Phong)
	half := r.LightDir.Sub(rd).Normalize()
	spec := math.Pow(clamp(n.Dot(half), 0, 1), r.SpecPower) * shadow

	// Ambient occlusion
	ao := r.ao(p, n)

	// Fresnel rim
	fresnel := math.Pow(1-clamp(-rd.Dot(n), 0, 1), 3) * 0.3

	// Combine
	ambient := r.Ambient * ao
	brightness := ambient + diff*0.65*ao + spec*0.25 + fresnel*ao

	// Fog
	fog := math.Exp(-t * t * 0.008)
	brightness *= fog

	return clamp(brightness, 0, 1)
}

// shadeCheap is a lightweight shading for sub-samples: diffuse + fog only.
func (r *Renderer) shadeCheap(ro, rd Vec3, t float64) float64 {
	p := ro.Add(rd.Mul(t))
	n := r.normal(p)

	// Diffuse only
	diff := clamp(n.Dot(r.LightDir), 0, 1)

	brightness := 0.15 + diff*0.75

	// Fog
	fog := math.Exp(-t * t * 0.008)
	brightness *= fog

	return clamp(brightness, 0, 1)
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
func (r *Renderer) renderCellShaped(ro, fwd, right, up Vec3, nx, ny, dx, dy, halfW, halfH, centerT float64) byte {
	if centerT >= maxDist {
		return ' '
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

	sv = DirectionalContrast(sv, ext, r.Contrast)
	sv = EnhanceContrast(sv, r.Contrast)
	return r.ShapeTable.Match(sv)
}

// Render the full frame, returns ASCII art as string
func (r *Renderer) Render() string {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	// Build camera matrix
	fwd := r.Camera.Target.Sub(r.Camera.Pos).Normalize()
	right := fwd.Cross(r.Camera.Up).Normalize()
	up := right.Cross(fwd)

	fovRad := r.Camera.FOV * math.Pi / 180
	halfH := math.Tan(fovRad / 2)
	// Terminal chars are ~2x taller than wide, so adjust aspect
	aspect := float64(w) / float64(h) * 0.45
	halfW := halfH * aspect

	ro := r.Camera.Pos

	// Per-pixel step sizes in normalized screen coordinates
	dx := 2.0 / float64(w-1)
	dy := 2.0 / float64(h-1)
	shapeMode := r.ShapeMode && r.ShapeTable != nil

	// Parallel rendering - one goroutine per row
	var wg sync.WaitGroup
	lines := make([][]byte, h)

	for y := 0; y < h; y++ {
		wg.Add(1)
		go func(y int) {
			defer wg.Done()
			line := make([]byte, w)
			// Normalized y: -1 to 1
			ny := 1.0 - 2.0*float64(y)/float64(h-1)

			for x := 0; x < w; x++ {
				// Normalized x: -1 to 1
				nx := 2.0*float64(x)/float64(w-1) - 1.0

				// Ray direction
				rd := fwd.Add(right.Mul(nx * halfW)).Add(up.Mul(ny * halfH)).Normalize()

				// Raymarch
				t, _ := r.raymarch(ro, rd)

				var ch byte
				if shapeMode {
					ch = r.renderCellShaped(ro, fwd, right, up, nx, ny, dx, dy, halfW, halfH, t)
				} else if t < maxDist {
					brightness := r.shade(ro, rd, t)
					idx := int(brightness * float64(len(asciiRamp)-1))
					if idx < 0 {
						idx = 0
					}
					if idx >= len(asciiRamp) {
						idx = len(asciiRamp) - 1
					}
					ch = asciiRamp[idx]
				} else {
					// Background - subtle gradient
					bgBright := 0.02 + 0.03*(ny+1)*0.5
					idx := int(bgBright * float64(len(asciiRamp)-1))
					if idx < 0 {
						idx = 0
					}
					if idx >= len(asciiRamp) {
						idx = len(asciiRamp) - 1
					}
					ch = asciiRamp[idx]
				}
				line[x] = ch
			}
			lines[y] = line
		}(y)
	}
	wg.Wait()

	// Build output string
	total := w*h + h // chars + newlines
	out := make([]byte, 0, total)
	for y := 0; y < h; y++ {
		if y > 0 {
			out = append(out, '\n')
		}
		out = append(out, lines[y]...)
	}
	return string(out)
}
