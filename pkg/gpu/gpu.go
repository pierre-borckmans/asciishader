package gpu

/*
#cgo CFLAGS: -DGL_SILENCE_DEPRECATION
#cgo LDFLAGS: -framework OpenGL
#include <OpenGL/OpenGL.h>
#include <OpenGL/gl3.h>

static int initCGLContext() {
	CGLPixelFormatAttribute attrs[] = {
		kCGLPFAOpenGLProfile, (CGLPixelFormatAttribute)kCGLOGLPVersion_3_2_Core,
		kCGLPFAAccelerated,
		kCGLPFAColorSize, (CGLPixelFormatAttribute)24,
		(CGLPixelFormatAttribute)0
	};
	CGLPixelFormatObj pix;
	GLint num;
	CGLError err = CGLChoosePixelFormat(attrs, &pix, &num);
	if (err != kCGLNoError) return -1;

	CGLContextObj ctx;
	err = CGLCreateContext(pix, NULL, &ctx);
	CGLDestroyPixelFormat(pix);
	if (err != kCGLNoError) return -2;

	err = CGLSetCurrentContext(ctx);
	if (err != kCGLNoError) return -3;

	return 0;
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"asciishader/pkg/core"
	"asciishader/pkg/render"
	"asciishader/pkg/shader"
	"asciishader/pkg/shape"

	"github.com/go-gl/gl/v4.1-core/gl"
)

const vertexShaderSource = `#version 150
in vec2 position;
void main() {
    gl_Position = vec4(position, 0.0, 1.0);
}
` + "\x00"

// fragmentShaderSource is assembled from shader_template.go parts.
// See shader_template.go for shaderPrefix, shader.DefaultUserCode, shaderSuffix.

// GPURenderer renders scenes using OpenGL fragment shaders.
type GPURenderer struct {
	program uint32
	vao     uint32
	fbo     uint32
	rbo     uint32
	pixW    int // pixel width (termW * 2)
	pixH    int // pixel height (termH * subH)
	termW   int // terminal columns
	termH   int // terminal rows
	subH    int // sub-pixel rows per core.Cell (3 for shape, 2 for quadrant)
	pixels  []byte

	// Reusable buffers (avoid per-frame allocation)
	cellBuf [][]core.Cell
	ansiBuf []byte

	// Hot-reload state
	userCode   string // current user GLSL code
	compileErr string // last compile error, empty if OK

	// Cached uniform locations
	uResolution   int32
	uTime         int32
	uCameraPos    int32
	uCameraTarget int32
	uLightDir     int32
	uFOV          int32
	uAmbient      int32
	uSpecPower    int32
	uShadowSteps  int32
	uAOSteps      int32
	uTermSize     int32
}

func NewGPURenderer() (*GPURenderer, error) {
	runtime.LockOSThread()

	if rc := C.initCGLContext(); rc != 0 {
		return nil, fmt.Errorf("CGL context creation failed: %d", rc)
	}

	if err := gl.Init(); err != nil {
		return nil, fmt.Errorf("GL init failed: %v", err)
	}

	fragSrc := shader.Assemble(shader.DefaultUserCode)
	program, err := createProgram(vertexShaderSource, fragSrc)
	if err != nil {
		return nil, fmt.Errorf("shader program: %v", err)
	}

	vao := createQuadVAO()

	g := &GPURenderer{
		program:  program,
		vao:      vao,
		userCode: shader.DefaultUserCode,
	}

	g.cacheUniforms()

	return g, nil
}

func (g *GPURenderer) resize(termW, termH, subW, subH int) {
	pixW := termW * subW
	pixH := termH * subH
	if termW == g.termW && termH == g.termH && pixW == g.pixW && pixH == g.pixH {
		return
	}
	g.termW = termW
	g.termH = termH
	g.subH = subH
	g.pixW = pixW
	g.pixH = pixH
	g.pixels = make([]byte, g.pixW*g.pixH*4)

	// Delete old FBO/RBO
	if g.fbo != 0 {
		gl.DeleteFramebuffers(1, &g.fbo)
		gl.DeleteRenderbuffers(1, &g.rbo)
	}

	// Create FBO with color renderbuffer at sub-pixel resolution
	gl.GenFramebuffers(1, &g.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, g.fbo)

	gl.GenRenderbuffers(1, &g.rbo)
	gl.BindRenderbuffer(gl.RENDERBUFFER, g.rbo)
	gl.RenderbufferStorage(gl.RENDERBUFFER, gl.RGBA8, int32(g.pixW), int32(g.pixH))
	gl.FramebufferRenderbuffer(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.RENDERBUFFER, g.rbo)

	if status := gl.CheckFramebufferStatus(gl.FRAMEBUFFER); status != gl.FRAMEBUFFER_COMPLETE {
		panic(fmt.Sprintf("framebuffer incomplete: 0x%x", status))
	}

	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// Render uses the GPU to raytrace and returns an ANSI-colored string.
func (g *GPURenderer) Render(r *render.Renderer) string {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	subW, subH := renderSubPixels(r.RenderMode)
	g.resize(w, h, subW, subH)

	gl.BindFramebuffer(gl.FRAMEBUFFER, g.fbo)
	gl.Viewport(0, 0, int32(g.pixW), int32(g.pixH))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.UseProgram(g.program)

	// Set uniforms — resolution is pixel resolution, termSize is terminal cells
	gl.Uniform2f(g.uResolution, float32(g.pixW), float32(g.pixH))
	gl.Uniform2f(g.uTermSize, float32(w), float32(h))
	gl.Uniform1f(g.uTime, float32(r.Time))
	gl.Uniform3f(g.uCameraPos, float32(r.Camera.Pos.X), float32(r.Camera.Pos.Y), float32(r.Camera.Pos.Z))
	gl.Uniform3f(g.uCameraTarget, float32(r.Camera.Target.X), float32(r.Camera.Target.Y), float32(r.Camera.Target.Z))
	gl.Uniform3f(g.uLightDir, float32(r.LightDir.X), float32(r.LightDir.Y), float32(r.LightDir.Z))
	gl.Uniform1f(g.uFOV, float32(r.Camera.FOV))
	gl.Uniform1f(g.uAmbient, float32(r.Ambient))
	gl.Uniform1f(g.uSpecPower, float32(r.SpecPower))
	gl.Uniform1i(g.uShadowSteps, int32(r.ShadowSteps))
	gl.Uniform1i(g.uAOSteps, int32(r.AOSteps))

	// Draw fullscreen quad
	gl.BindVertexArray(g.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)

	// Read pixels at sub-pixel resolution
	gl.ReadPixels(0, 0, int32(g.pixW), int32(g.pixH), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&g.pixels[0]))
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)

	// Build cells then convert to ANSI
	g.ansiBuf = render.AppendANSI(g.ansiBuf[:0], g.RenderCells(r))
	return string(g.ansiBuf)
}

// RenderToCells does the full GPU render pass and returns the core.Cell grid (no ANSI).
func (g *GPURenderer) RenderToCells(r *render.Renderer) [][]core.Cell {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return nil
	}

	subW, subH := renderSubPixels(r.RenderMode)
	g.resize(w, h, subW, subH)

	gl.BindFramebuffer(gl.FRAMEBUFFER, g.fbo)
	gl.Viewport(0, 0, int32(g.pixW), int32(g.pixH))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.UseProgram(g.program)

	gl.Uniform2f(g.uResolution, float32(g.pixW), float32(g.pixH))
	gl.Uniform2f(g.uTermSize, float32(w), float32(h))
	gl.Uniform1f(g.uTime, float32(r.Time))
	gl.Uniform3f(g.uCameraPos, float32(r.Camera.Pos.X), float32(r.Camera.Pos.Y), float32(r.Camera.Pos.Z))
	gl.Uniform3f(g.uCameraTarget, float32(r.Camera.Target.X), float32(r.Camera.Target.Y), float32(r.Camera.Target.Z))
	gl.Uniform3f(g.uLightDir, float32(r.LightDir.X), float32(r.LightDir.Y), float32(r.LightDir.Z))
	gl.Uniform1f(g.uFOV, float32(r.Camera.FOV))
	gl.Uniform1f(g.uAmbient, float32(r.Ambient))
	gl.Uniform1f(g.uSpecPower, float32(r.SpecPower))
	gl.Uniform1i(g.uShadowSteps, int32(r.ShadowSteps))
	gl.Uniform1i(g.uAOSteps, int32(r.AOSteps))

	gl.BindVertexArray(g.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)

	gl.ReadPixels(0, 0, int32(g.pixW), int32(g.pixH), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&g.pixels[0]))
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)

	return g.RenderCells(r)
}

// pixelBrightness returns the lighting-only brightness (0-1) stored in the alpha channel.
func (g *GPURenderer) pixelBrightness(px, py int) float64 {
	if px < 0 || px >= g.pixW || py < 0 || py >= g.pixH {
		return 0
	}
	off := (py*g.pixW + px) * 4
	return float64(g.pixels[off+3]) / 255
}

// pixelColor returns the RGB color (0-1) at the given pixel coordinate.
func (g *GPURenderer) pixelColor(px, py int) core.Vec3 {
	if px < 0 || px >= g.pixW || py < 0 || py >= g.pixH {
		return core.Vec3{}
	}
	off := (py*g.pixW + px) * 4
	return core.Vec3{
		X: float64(g.pixels[off]) / 255,
		Y: float64(g.pixels[off+1]) / 255,
		Z: float64(g.pixels[off+2]) / 255,
	}
}

// renderSubPixels returns the sub-pixel dimensions for a given render mode.
func renderSubPixels(mode int) (subW, subH int) {
	switch mode {
	case core.RenderBlocks:
		return 2, 2
	case core.RenderDual:
		return 4, 6
	case core.RenderHalfBlock:
		return 1, 2
	case core.RenderBraille:
		return 2, 4
	case core.RenderDensity:
		return 2, 3
	default: // RenderShapes
		return 2, 3
	}
}

// getCellBuf returns a zeroed tw×th core.Cell grid, reusing the internal buffer.
func (g *GPURenderer) getCellBuf(tw, th int) [][]core.Cell {
	if cap(g.cellBuf) >= th {
		g.cellBuf = g.cellBuf[:th]
	} else {
		g.cellBuf = make([][]core.Cell, th)
	}
	for i := range g.cellBuf {
		if cap(g.cellBuf[i]) >= tw {
			g.cellBuf[i] = g.cellBuf[i][:tw]
			clear(g.cellBuf[i])
		} else {
			g.cellBuf[i] = make([]core.Cell, tw)
		}
	}
	return g.cellBuf
}

// RenderCells uses GPU pixel data to produce a core.Cell grid.
func (g *GPURenderer) RenderCells(r *render.Renderer) [][]core.Cell {
	tw, th := g.termW, g.termH

	switch r.RenderMode {
	case core.RenderBlocks:
		return g.renderCellsQuadrant(tw, th)
	case core.RenderDual:
		return g.renderCellsDual(r, tw, th)
	case core.RenderHalfBlock:
		return g.renderCellsHalfBlock(tw, th)
	case core.RenderBraille:
		return g.renderCellsBraille(tw, th)
	case core.RenderDensity:
		return g.renderCellsDensity(tw, th)
	default:
		return g.renderCellsShaped(r, tw, th)
	}
}

// renderCellsShaped uses shape matching on GPU pixel data (2×3 sub-pixels per cell).
func (g *GPURenderer) renderCellsShaped(r *render.Renderer, tw, th int) [][]core.Cell {
	st := r.ShapeTable
	pixels := g.pixels
	stride := g.pixW * 4
	contrast := r.Contrast
	const inv255 = 1.0 / 255.0

	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 3

			// Interior 2×3 block: always in bounds (pixel buffer = termW*2 × termH*3)
			off00 := by*stride + bx*4
			off10 := off00 + 4
			off01 := off00 + stride
			off11 := off01 + 4
			off02 := off01 + stride
			off12 := off02 + 4

			var sv shape.ShapeVec
			sv[0] = float64(pixels[off00+3]) * inv255
			sv[1] = float64(pixels[off10+3]) * inv255
			sv[2] = float64(pixels[off01+3]) * inv255
			sv[3] = float64(pixels[off11+3]) * inv255
			sv[4] = float64(pixels[off02+3]) * inv255
			sv[5] = float64(pixels[off12+3]) * inv255

			avgBright := (sv[0] + sv[1] + sv[2] + sv[3] + sv[4] + sv[5])
			if avgBright < 0.06 { // 0.01 * 6
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// External boundary samples (need bounds checks for edge cells)
			var ext shape.ShapeVec
			ext[0] = g.pixelBrightness(bx-1, by-1)
			ext[1] = g.pixelBrightness(bx+2, by-1)
			ext[2] = g.pixelBrightness(bx-1, by+1)
			ext[3] = g.pixelBrightness(bx+2, by+1)
			ext[4] = g.pixelBrightness(bx-1, by+3)
			ext[5] = g.pixelBrightness(bx+2, by+3)

			sv = shape.DirectionalContrast(sv, ext, contrast)
			sv = shape.EnhanceContrast(sv, contrast)
			ch := st.Match(sv)

			// Average color from the 6 interior pixels (already bounds-safe)
			colR := float64(pixels[off00]) + float64(pixels[off10]) +
				float64(pixels[off01]) + float64(pixels[off11]) +
				float64(pixels[off02]) + float64(pixels[off12])
			colG := float64(pixels[off00+1]) + float64(pixels[off10+1]) +
				float64(pixels[off01+1]) + float64(pixels[off11+1]) +
				float64(pixels[off02+1]) + float64(pixels[off12+1])
			colB := float64(pixels[off00+2]) + float64(pixels[off10+2]) +
				float64(pixels[off01+2]) + float64(pixels[off11+2]) +
				float64(pixels[off02+2]) + float64(pixels[off12+2])

			const inv6x255 = 1.0 / (6 * 255)
			line[cx] = core.Cell{Ch: rune(ch), Col: core.Vec3{X: colR * inv6x255, Y: colG * inv6x255, Z: colB * inv6x255}}
		}
	}
	return lines
}

// renderCellsDual uses 4×6 pixel blocks per core.Cell for broader shape matching.
// Output grid: tw × th (fills viewport). Pixel buffer is subW=4, subH=6.
func (g *GPURenderer) renderCellsDual(r *render.Renderer, tw, th int) [][]core.Cell {
	st := r.ShapeTable

	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			bx := cx * 4
			by := cy * 6

			// Downsample 4×6 to 2×3 shape vector: each element = average of 2×2 pixel block
			var sv shape.ShapeVec
			for si := 0; si < 6; si++ {
				r0 := si / 2
				c0 := si % 2
				px := bx + c0*2
				py := by + r0*2
				sv[si] = (g.pixelBrightness(px, py) + g.pixelBrightness(px+1, py) +
					g.pixelBrightness(px, py+1) + g.pixelBrightness(px+1, py+1)) / 4
			}

			avgBright := 0.0
			for i := 0; i < 6; i++ {
				avgBright += sv[i]
			}
			avgBright /= 6

			if avgBright < 0.01 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// External samples: just outside the 4×6 block
			var ext shape.ShapeVec
			ext[0] = g.pixelBrightness(bx-1, by-1)
			ext[1] = g.pixelBrightness(bx+4, by-1)
			ext[2] = g.pixelBrightness(bx-1, by+3)
			ext[3] = g.pixelBrightness(bx+4, by+3)
			ext[4] = g.pixelBrightness(bx-1, by+6)
			ext[5] = g.pixelBrightness(bx+4, by+6)

			sv = shape.DirectionalContrast(sv, ext, r.Contrast)
			sv = shape.EnhanceContrast(sv, r.Contrast)
			ch := st.Match(sv)

			// Average color from all 24 pixels (4×6)
			var colR, colG, colB float64
			for dy := 0; dy < 6; dy++ {
				for dx := 0; dx < 4; dx++ {
					px := bx + dx
					py := by + dy
					if px >= 0 && px < g.pixW && py >= 0 && py < g.pixH {
						off := (py*g.pixW + px) * 4
						colR += float64(g.pixels[off])
						colG += float64(g.pixels[off+1])
						colB += float64(g.pixels[off+2])
					}
				}
			}

			line[cx] = core.Cell{Ch: rune(ch), Col: core.Vec3{X: colR / 24 / 255, Y: colG / 24 / 255, Z: colB / 24 / 255}}
		}
	}
	return lines
}

// renderCellsQuadrant uses 2×2 sub-pixels per core.Cell to produce quadrant block characters.
func (g *GPURenderer) renderCellsQuadrant(tw, th int) [][]core.Cell {
	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 2

			// Read 4 pixel colors in 2×2 grid: TL, TR, BL, BR
			colors := [4]core.Vec3{
				g.pixelColor(bx, by),     // TL
				g.pixelColor(bx+1, by),   // TR
				g.pixelColor(bx, by+1),   // BL
				g.pixelColor(bx+1, by+1), // BR
			}

			// Compute brightness per pixel and detect hits via alpha
			var bright [4]float64
			hit := [4]bool{}
			hitCount := 0
			for i := 0; i < 4; i++ {
				bright[i] = colors[i].X*0.299 + colors[i].Y*0.587 + colors[i].Z*0.114
				// Use alpha channel to detect actual surface hits
				px := bx + (i % 2)
				py := by + (i / 2)
				if px >= 0 && px < g.pixW && py >= 0 && py < g.pixH {
					off := (py*g.pixW + px) * 4
					if g.pixels[off+3] > 2 {
						hit[i] = true
						hitCount++
					}
				}
			}

			// Mean brightness across all 4 (misses contribute 0)
			mean := (bright[0] + bright[1] + bright[2] + bright[3]) / 4

			if mean < 0.01 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// Uniform surface check: all 4 hit with similar brightness → full block
			if hitCount == 4 {
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
					line[cx] = core.Cell{Ch: '█', Col: core.V(core.Clamp(avg.X, 0, 1), core.Clamp(avg.Y, 0, 1), core.Clamp(avg.Z, 0, 1))}
					continue
				}
			}

			// Threshold each pixel around mean
			var pattern int
			var onCount int
			var fgCol, bgCol core.Vec3
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
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			ch := render.QuadrantChars[pattern]
			fg := fgCol.Mul(1.0 / float64(onCount))
			fg = core.V(core.Clamp(fg.X, 0, 1), core.Clamp(fg.Y, 0, 1), core.Clamp(fg.Z, 0, 1))

			// Only set bg when off-pixels hit actual geometry
			if bgHitCount == 0 {
				line[cx] = core.Cell{Ch: ch, Col: fg}
				continue
			}

			bg := bgCol.Mul(1.0 / float64(bgHitCount))
			bg = core.V(core.Clamp(bg.X, 0, 1), core.Clamp(bg.Y, 0, 1), core.Clamp(bg.Z, 0, 1))

			line[cx] = core.Cell{Ch: ch, Col: fg, Bg: bg, HasBg: true}
		}
	}
	return lines
}

// renderCellsHalfBlock uses ▀ with fg=top color, bg=bottom color for 1×2 sub-pixels per cell.
func (g *GPURenderer) renderCellsHalfBlock(tw, th int) [][]core.Cell {
	pixels := g.pixels
	stride := g.pixW * 4
	const inv255 = 1.0 / 255.0

	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			// Top pixel at (cx, cy*2), bottom pixel at (cx, cy*2+1)
			offTop := (cy*2)*stride + cx*4
			offBot := offTop + stride

			topR := float64(pixels[offTop]) * inv255
			topG := float64(pixels[offTop+1]) * inv255
			topB := float64(pixels[offTop+2]) * inv255
			topA := pixels[offTop+3]

			botR := float64(pixels[offBot]) * inv255
			botG := float64(pixels[offBot+1]) * inv255
			botB := float64(pixels[offBot+2]) * inv255
			botA := pixels[offBot+3]

			topHit := topA > 2
			botHit := botA > 2

			if !topHit && !botHit {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			if topHit && botHit {
				// Both lit: ▀ with fg=top, bg=bottom
				line[cx] = core.Cell{
					Ch:    '▀',
					Col:   core.Vec3{X: topR, Y: topG, Z: topB},
					Bg:    core.Vec3{X: botR, Y: botG, Z: botB},
					HasBg: true,
				}
			} else if topHit {
				// Only top lit: ▀ with fg=top, no bg
				line[cx] = core.Cell{Ch: '▀', Col: core.Vec3{X: topR, Y: topG, Z: topB}}
			} else {
				// Only bottom lit: ▄ with fg=bottom, no bg
				line[cx] = core.Cell{Ch: '▄', Col: core.Vec3{X: botR, Y: botG, Z: botB}}
			}
		}
	}
	return lines
}

// renderCellsBraille uses 2×4 sub-pixels per core.Cell to produce Unicode braille characters.
// Braille dot layout and bit mapping (Unicode U+2800 + bits):
//
//	col0 col1
//
// row0  bit0 bit3
// row1  bit1 bit4
// row2  bit2 bit5
// row3  bit6 bit7
func (g *GPURenderer) renderCellsBraille(tw, th int) [][]core.Cell {
	pixels := g.pixels
	stride := g.pixW * 4
	const hitThresh byte = 2 // same as half-block / quadrant

	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 4

			offBase := by*stride + bx*4
			off1 := offBase + stride
			off2 := off1 + stride
			off3 := off2 + stride

			// Read alpha for each of the 2×4 dots
			a00 := pixels[offBase+3]   // col0, row0
			a10 := pixels[offBase+4+3] // col1, row0
			a01 := pixels[off1+3]      // col0, row1
			a11 := pixels[off1+4+3]    // col1, row1
			a02 := pixels[off2+3]      // col0, row2
			a12 := pixels[off2+4+3]    // col1, row2
			a03 := pixels[off3+3]      // col0, row3
			a13 := pixels[off3+4+3]    // col1, row3

			// Build braille pattern from hit detection
			var pattern rune
			if a00 > hitThresh {
				pattern |= 0x01
			}
			if a01 > hitThresh {
				pattern |= 0x02
			}
			if a02 > hitThresh {
				pattern |= 0x04
			}
			if a10 > hitThresh {
				pattern |= 0x08
			}
			if a11 > hitThresh {
				pattern |= 0x10
			}
			if a12 > hitThresh {
				pattern |= 0x20
			}
			if a03 > hitThresh {
				pattern |= 0x40
			}
			if a13 > hitThresh {
				pattern |= 0x80
			}

			if pattern == 0 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// Average color from lit pixels only
			var colR, colG, colB float64
			var litCount float64
			alphas := [8]byte{a00, a10, a01, a11, a02, a12, a03, a13}
			offsets := [8]int{
				offBase, offBase + 4,
				off1, off1 + 4,
				off2, off2 + 4,
				off3, off3 + 4,
			}
			for i := 0; i < 8; i++ {
				if alphas[i] > hitThresh {
					o := offsets[i]
					colR += float64(pixels[o])
					colG += float64(pixels[o+1])
					colB += float64(pixels[o+2])
					litCount++
				}
			}
			inv := 1.0 / (litCount * 255)
			line[cx] = core.Cell{Ch: 0x2800 + pattern, Col: core.Vec3{X: colR * inv, Y: colG * inv, Z: colB * inv}}
		}
	}
	return lines
}

// renderCellsDensity uses a classic ASCII density ramp based on average brightness.
// Uses 2×3 sub-pixels per core.Cell (same as shaped mode) for quality averaging.
func (g *GPURenderer) renderCellsDensity(tw, th int) [][]core.Cell {
	pixels := g.pixels
	stride := g.pixW * 4
	const inv255 = 1.0 / 255.0

	ramp := core.AsciiRamp
	rampMax := len(ramp) - 1

	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 3

			off00 := by*stride + bx*4
			off10 := off00 + 4
			off01 := off00 + stride
			off11 := off01 + 4
			off02 := off01 + stride
			off12 := off02 + 4

			// Average brightness from alpha channel
			avgBright := (float64(pixels[off00+3]) + float64(pixels[off10+3]) +
				float64(pixels[off01+3]) + float64(pixels[off11+3]) +
				float64(pixels[off02+3]) + float64(pixels[off12+3])) * inv255 / 6

			if avgBright < 0.01 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// Map brightness to ramp character
			idx := int(avgBright * float64(rampMax))
			if idx > rampMax {
				idx = rampMax
			}
			ch := rune(ramp[idx])

			// Average color
			const inv6x255 = 1.0 / (6 * 255)
			colR := float64(pixels[off00]) + float64(pixels[off10]) +
				float64(pixels[off01]) + float64(pixels[off11]) +
				float64(pixels[off02]) + float64(pixels[off12])
			colG := float64(pixels[off00+1]) + float64(pixels[off10+1]) +
				float64(pixels[off01+1]) + float64(pixels[off11+1]) +
				float64(pixels[off02+1]) + float64(pixels[off12+1])
			colB := float64(pixels[off00+2]) + float64(pixels[off10+2]) +
				float64(pixels[off01+2]) + float64(pixels[off11+2]) +
				float64(pixels[off02+2]) + float64(pixels[off12+2])

			line[cx] = core.Cell{Ch: ch, Col: core.Vec3{X: colR * inv6x255, Y: colG * inv6x255, Z: colB * inv6x255}}
		}
	}
	return lines
}

func (g *GPURenderer) Destroy() {
	if g.program != 0 {
		gl.DeleteProgram(g.program)
	}
	if g.vao != 0 {
		gl.DeleteVertexArrays(1, &g.vao)
	}
	if g.fbo != 0 {
		gl.DeleteFramebuffers(1, &g.fbo)
	}
	if g.rbo != 0 {
		gl.DeleteRenderbuffers(1, &g.rbo)
	}
}

// cacheUniforms looks up and caches all uniform locations for the current program.
func (g *GPURenderer) cacheUniforms() {
	gl.UseProgram(g.program)
	g.uResolution = gl.GetUniformLocation(g.program, gl.Str("uResolution\x00"))
	g.uTime = gl.GetUniformLocation(g.program, gl.Str("uTime\x00"))
	g.uCameraPos = gl.GetUniformLocation(g.program, gl.Str("uCameraPos\x00"))
	g.uCameraTarget = gl.GetUniformLocation(g.program, gl.Str("uCameraTarget\x00"))
	g.uLightDir = gl.GetUniformLocation(g.program, gl.Str("uLightDir\x00"))
	g.uFOV = gl.GetUniformLocation(g.program, gl.Str("uFOV\x00"))
	g.uAmbient = gl.GetUniformLocation(g.program, gl.Str("uAmbient\x00"))
	g.uSpecPower = gl.GetUniformLocation(g.program, gl.Str("uSpecPower\x00"))
	g.uShadowSteps = gl.GetUniformLocation(g.program, gl.Str("uShadowSteps\x00"))
	g.uAOSteps = gl.GetUniformLocation(g.program, gl.Str("uAOSteps\x00"))
	g.uTermSize = gl.GetUniformLocation(g.program, gl.Str("uTermSize\x00"))
}

// CompileUserCode compiles new user GLSL code into the shader program.
// On success, swaps in the new program. On failure, keeps the old program
// and returns the error.
func (g *GPURenderer) CompileUserCode(code string) error {
	fragSrc := shader.Assemble(code)
	newProg, err := createProgram(vertexShaderSource, fragSrc)
	if err != nil {
		g.compileErr = err.Error()
		return err
	}

	// Success — swap programs
	gl.DeleteProgram(g.program)
	g.program = newProg
	g.userCode = code
	g.compileErr = ""
	g.cacheUniforms()
	return nil
}

// --- GL helpers ---

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)
	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength)
		gl.GetShaderInfoLog(shader, logLength, nil, &log[0])
		gl.DeleteShader(shader)
		return 0, fmt.Errorf("shader compile: %s", string(log))
	}
	return shader, nil
}

func createProgram(vertSrc, fragSrc string) (uint32, error) {
	vert, err := compileShader(vertSrc, gl.VERTEX_SHADER)
	if err != nil {
		return 0, fmt.Errorf("vertex: %v", err)
	}
	frag, err := compileShader(fragSrc, gl.FRAGMENT_SHADER)
	if err != nil {
		gl.DeleteShader(vert)
		return 0, fmt.Errorf("fragment: %v", err)
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vert)
	gl.AttachShader(program, frag)
	gl.LinkProgram(program)
	gl.DeleteShader(vert)
	gl.DeleteShader(frag)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := make([]byte, logLength)
		gl.GetProgramInfoLog(program, logLength, nil, &log[0])
		gl.DeleteProgram(program)
		return 0, fmt.Errorf("link: %s", string(log))
	}
	return program, nil
}

func createQuadVAO() uint32 {
	vertices := []float32{
		-1, -1,
		1, -1,
		-1, 1,
		-1, 1,
		1, -1,
		1, 1,
	}

	var vao uint32
	gl.GenVertexArrays(1, &vao)
	gl.BindVertexArray(vao)

	var vbo uint32
	gl.GenBuffers(1, &vbo)
	gl.BindBuffer(gl.ARRAY_BUFFER, vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, unsafe.Pointer(&vertices[0]), gl.STATIC_DRAW)

	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 0, nil)

	gl.BindVertexArray(0)
	return vao
}
