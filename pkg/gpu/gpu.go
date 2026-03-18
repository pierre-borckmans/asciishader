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
	"bytes"
	"fmt"
	"os"
	"runtime"
	"unsafe"

	"asciishader/pkg/core"
	"asciishader/pkg/shader"

	"github.com/go-gl/gl/v4.1-core/gl"
	"github.com/klauspost/compress/zlib"
)

// pidOnce is used to create unique shm segment names.
var pidOnce = os.Getpid()

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

	// Shape table for ASCII shape matching
	ShapeTable *core.ShapeTable

	// Reusable buffers (avoid per-frame allocation)
	cellBuf [][]core.Cell
	ansiBuf []byte

	// Image mode: shm path + zlib fallback with pipelining
	imgShmOK      bool
	imgShm        [2]*shmSegment
	imgShmIdx     int
	imgShmCounter int
	imgRGBBufs    [2][]byte
	imgBufIdx     int
	imgZBuf       bytes.Buffer
	imgB64        bytes.Buffer
	imgZlib       *zlib.Writer
	imgEncOut     chan imgEncResult
	imgEncPending bool
	imgPlaced     bool
	imgLastRow    int
	imgLastCol    int
	imgLastW      int
	imgLastH      int

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
	uProjection   int32
	uOrthoScale   int32
	uSliceMode    int32
	uSliceY       int32
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
		program:    program,
		vao:        vao,
		userCode:   shader.DefaultUserCode,
		ShapeTable: core.NewShapeTable(),
		imgEncOut:  make(chan imgEncResult, 1),
		imgShmOK:   true, // try shm first, falls back to zlib
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

// renderPass runs the GPU shader and reads back pixels at the given sub-pixel resolution.
func (g *GPURenderer) renderPass(r *core.RenderConfig, subW, subH int) {
	g.resize(r.Width, r.Height, subW, subH)

	gl.BindFramebuffer(gl.FRAMEBUFFER, g.fbo)
	gl.Viewport(0, 0, int32(g.pixW), int32(g.pixH))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.UseProgram(g.program)

	gl.Uniform2f(g.uResolution, float32(g.pixW), float32(g.pixH))
	gl.Uniform2f(g.uTermSize, float32(r.Width), float32(r.Height))
	gl.Uniform1f(g.uTime, float32(r.Time))
	gl.Uniform3f(g.uCameraPos, float32(r.Camera.Pos.X), float32(r.Camera.Pos.Y), float32(r.Camera.Pos.Z))
	gl.Uniform3f(g.uCameraTarget, float32(r.Camera.Target.X), float32(r.Camera.Target.Y), float32(r.Camera.Target.Z))
	gl.Uniform3f(g.uLightDir, float32(r.LightDir.X), float32(r.LightDir.Y), float32(r.LightDir.Z))
	gl.Uniform1f(g.uFOV, float32(r.Camera.FOV))
	gl.Uniform1f(g.uAmbient, float32(r.Ambient))
	gl.Uniform1f(g.uSpecPower, float32(r.SpecPower))
	gl.Uniform1i(g.uShadowSteps, int32(r.ShadowSteps))
	gl.Uniform1i(g.uAOSteps, int32(r.AOSteps))
	gl.Uniform1i(g.uProjection, int32(r.Projection))
	gl.Uniform1f(g.uOrthoScale, float32(r.OrthoScale))
	gl.Uniform1i(g.uSliceMode, int32(r.SliceMode))
	gl.Uniform1f(g.uSliceY, float32(r.SliceY))

	gl.BindVertexArray(g.vao)
	gl.DrawArrays(gl.TRIANGLES, 0, 6)

	gl.ReadPixels(0, 0, int32(g.pixW), int32(g.pixH), gl.RGBA, gl.UNSIGNED_BYTE, unsafe.Pointer(&g.pixels[0]))
	gl.BindFramebuffer(gl.FRAMEBUFFER, 0)
}

// Render uses the GPU to raytrace and returns an ANSI-colored string.
func (g *GPURenderer) Render(r *core.RenderConfig) string {
	if r.Width <= 0 || r.Height <= 0 {
		return ""
	}
	subW, subH := renderSubPixels(r.RenderMode)
	g.renderPass(r, subW, subH)
	g.ansiBuf = core.AppendANSI(g.ansiBuf[:0], g.RenderCells(r))
	return string(g.ansiBuf)
}

// RenderToCells does the full GPU render pass and returns the core.Cell grid (no ANSI).
func (g *GPURenderer) RenderToCells(r *core.RenderConfig) [][]core.Cell {
	if r.Width <= 0 || r.Height <= 0 {
		return nil
	}
	subW, subH := renderSubPixels(r.RenderMode)
	g.renderPass(r, subW, subH)
	return g.RenderCells(r)
}

// RenderRaw runs the GPU shader at the given sub-pixel resolution without cell conversion.
// Access raw pixel data afterwards via RawPixels().
func (g *GPURenderer) RenderRaw(r *core.RenderConfig, subW, subH int) {
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	g.renderPass(r, subW, subH)
}

// RawPixels returns the RGBA pixel buffer and its width from the last render pass.
func (g *GPURenderer) RawPixels() ([]byte, int) {
	return g.pixels, g.pixW
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
	case core.RenderShapes:
		return 2, 3
	case core.RenderSlice:
		return 1, 2 // half-block for color fidelity
	case core.RenderCost:
		return 1, 2
	case core.RenderImage:
		return 1, 1 // image mode uses RenderImageFrame directly
	default: // RenderBlocks, RenderBraille use 2×4
		return 2, 4
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
func (g *GPURenderer) RenderCells(r *core.RenderConfig) [][]core.Cell {
	tw, th := g.termW, g.termH

	switch r.RenderMode {
	case core.RenderBlocks:
		return g.renderCellsQuadrant(r, tw, th)
	case core.RenderBraille:
		return g.renderCellsBraille(r, tw, th)
	case core.RenderSlice:
		return g.renderCellsHalfBlock(tw, th)
	case core.RenderCost:
		return g.renderCellsHalfBlock(tw, th)
	default:
		return g.renderCellsShaped(r, tw, th)
	}
}

// renderCellsShaped uses shape matching on GPU pixel data (2×3 sub-pixels per cell).
func (g *GPURenderer) renderCellsShaped(r *core.RenderConfig, tw, th int) [][]core.Cell {
	st := g.ShapeTable
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

			var sv core.ShapeVec
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
			var ext core.ShapeVec
			ext[0] = g.pixelBrightness(bx-1, by-1)
			ext[1] = g.pixelBrightness(bx+2, by-1)
			ext[2] = g.pixelBrightness(bx-1, by+1)
			ext[3] = g.pixelBrightness(bx+2, by+1)
			ext[4] = g.pixelBrightness(bx-1, by+3)
			ext[5] = g.pixelBrightness(bx+2, by+3)

			sv = core.DirectionalContrast(sv, ext, contrast)
			sv = core.EnhanceContrast(sv, contrast)
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

// renderCellsQuadrant uses 2×4 sub-pixels per core.Cell to produce quadrant block characters.
// Each quadrant averages 2 vertical sub-pixel rows for hit detection and color.
func (g *GPURenderer) renderCellsQuadrant(r *core.RenderConfig, tw, th int) [][]core.Cell {
	lines := g.getCellBuf(tw, th)
	for cy := 0; cy < th; cy++ {
		line := lines[cy]
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 4

			// Read 8 sub-pixel colors and alpha from 2×4 grid
			type subPx struct {
				col core.Vec3
				hit bool
			}
			var sp [8]subPx
			for i := 0; i < 8; i++ {
				px := bx + (i % 2)
				py := by + (i / 2)
				sp[i].col = g.pixelColor(px, py)
				off := (py*g.pixW + px) * 4
				sp[i].hit = g.pixels[off+3] > 2
			}

			// Compute quadrant brightness and colors (TL, TR, BL, BR)
			// Each quadrant = 2 vertical sub-pixels
			type quad struct {
				bright float64
				col    core.Vec3
				hit    bool
			}
			quads := [4]quad{
				{}, // TL: sp[0], sp[2] (col0, rows 0-1)
				{}, // TR: sp[1], sp[3] (col1, rows 0-1)
				{}, // BL: sp[4], sp[6] (col0, rows 2-3)
				{}, // BR: sp[5], sp[7] (col1, rows 2-3)
			}
			qIdx := [4][2]int{{0, 2}, {1, 3}, {4, 6}, {5, 7}}
			hitCount := 0
			for qi := 0; qi < 4; qi++ {
				a, b := qIdx[qi][0], qIdx[qi][1]
				quads[qi].hit = sp[a].hit || sp[b].hit
				if quads[qi].hit {
					hitCount++
					// Average color from hit sub-pixels in this quadrant
					var col core.Vec3
					n := 0.0
					if sp[a].hit {
						col = col.Add(sp[a].col)
						n++
					}
					if sp[b].hit {
						col = col.Add(sp[b].col)
						n++
					}
					if n > 0 {
						col = col.Mul(1.0 / n)
					}
					quads[qi].col = col
					quads[qi].bright = col.X*0.299 + col.Y*0.587 + col.Z*0.114
				}
			}

			mean := (quads[0].bright + quads[1].bright + quads[2].bright + quads[3].bright) * 0.25
			if mean < 0.01 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			// Uniform surface check
			if hitCount == 4 {
				minB, maxB := quads[0].bright, quads[0].bright
				for i := 1; i < 4; i++ {
					if quads[i].bright < minB {
						minB = quads[i].bright
					}
					if quads[i].bright > maxB {
						maxB = quads[i].bright
					}
				}
				if maxB-minB < 0.08 {
					avg := quads[0].col.Add(quads[1].col).Add(quads[2].col).Add(quads[3].col).Mul(0.25)
					line[cx] = core.Cell{Ch: '█', Col: core.CompensateColor(avg, r.BlockGamma)}
					continue
				}
			}

			// Threshold each quadrant around mean
			var pattern int
			var onCount int
			var fgCol, bgCol core.Vec3
			bgHitCount := 0
			for i := 0; i < 4; i++ {
				bit := 3 - i
				if quads[i].hit && quads[i].bright > mean {
					pattern |= 1 << uint(bit)
					fgCol = fgCol.Add(quads[i].col)
					onCount++
				} else if quads[i].hit {
					bgCol = bgCol.Add(quads[i].col)
					bgHitCount++
				}
			}

			if onCount == 0 {
				line[cx] = core.Cell{Ch: ' '}
				continue
			}

			ch := core.QuadrantChars[pattern]
			fg := core.CompensateColor(fgCol.Mul(1.0/float64(onCount)), r.BlockGamma)

			if bgHitCount == 0 {
				line[cx] = core.Cell{Ch: ch, Col: fg}
				continue
			}

			bg := core.CompensateColor(bgCol.Mul(1.0/float64(bgHitCount)), r.BlockGamma)

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
func (g *GPURenderer) renderCellsBraille(r *core.RenderConfig, tw, th int) [][]core.Cell {
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
			col := core.CompensateColor(core.Vec3{X: colR * inv, Y: colG * inv, Z: colB * inv}, r.BrailleGamma)
			line[cx] = core.Cell{Ch: 0x2800 + pattern, Col: col}
		}
	}
	return lines
}

func (g *GPURenderer) Destroy() {
	g.CleanupImage()
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
	g.uProjection = gl.GetUniformLocation(g.program, gl.Str("uProjection\x00"))
	g.uOrthoScale = gl.GetUniformLocation(g.program, gl.Str("uOrthoScale\x00"))
	g.uSliceMode = gl.GetUniformLocation(g.program, gl.Str("uSliceMode\x00"))
	g.uSliceY = gl.GetUniformLocation(g.program, gl.Str("uSliceY\x00"))
}

// CompileUserCode compiles new user GLSL code into the shader program.
// On success, swaps in the new program. On failure, keeps the old program
// and returns the error.
func (g *GPURenderer) CompileUserCode(code string) error {
	return g.compileWithAssembler(code, shader.Assemble)
}

// CompileGLSLCode compiles standalone GLSL code using minimal prefix (no SDF library).
func (g *GPURenderer) CompileGLSLCode(code string) error {
	return g.compileWithAssembler(code, shader.AssembleGLSL)
}

func (g *GPURenderer) compileWithAssembler(code string, assemble func(string) string) error {
	fragSrc := assemble(code)
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
