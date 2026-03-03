package main

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

	"github.com/go-gl/gl/v4.1-core/gl"
)

const vertexShaderSource = `#version 150
in vec2 position;
void main() {
    gl_Position = vec4(position, 0.0, 1.0);
}
` + "\x00"

// fragmentShaderSource is assembled from shader_template.go parts.
// See shader_template.go for shaderPrefix, defaultUserCode, shaderSuffix.

// GPURenderer renders scenes using OpenGL fragment shaders.
type GPURenderer struct {
	program uint32
	vao     uint32
	fbo     uint32
	rbo     uint32
	pixW     int // pixel width (termW * 2)
	pixH     int // pixel height (termH * subH)
	termW    int // terminal columns
	termH    int // terminal rows
	subH     int // sub-pixel rows per cell (3 for shape, 2 for quadrant)
	pixels   []byte

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

	fragSrc := assembleShader(defaultUserCode)
	program, err := createProgram(vertexShaderSource, fragSrc)
	if err != nil {
		return nil, fmt.Errorf("shader program: %v", err)
	}

	vao := createQuadVAO()

	g := &GPURenderer{
		program:  program,
		vao:      vao,
		userCode: defaultUserCode,
	}

	g.cacheUniforms()

	return g, nil
}

func (g *GPURenderer) resize(termW, termH, subH int) {
	if termW == g.termW && termH == g.termH && subH == g.subH {
		return
	}
	g.termW = termW
	g.termH = termH
	g.subH = subH
	g.pixW = termW * 2
	g.pixH = termH * subH
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
func (g *GPURenderer) Render(r *Renderer) string {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	subH := 3
	if r.QuadrantMode {
		subH = 2
	}
	g.resize(w, h, subH)

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
	return buildANSI(g.RenderCells(r))
}

// RenderToCells does the full GPU render pass and returns the cell grid (no ANSI).
func (g *GPURenderer) RenderToCells(r *Renderer) [][]cell {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return nil
	}

	subH := 3
	if r.QuadrantMode {
		subH = 2
	}
	g.resize(w, h, subH)

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
func (g *GPURenderer) pixelColor(px, py int) Vec3 {
	if px < 0 || px >= g.pixW || py < 0 || py >= g.pixH {
		return Vec3{}
	}
	off := (py*g.pixW + px) * 4
	return Vec3{
		float64(g.pixels[off]) / 255,
		float64(g.pixels[off+1]) / 255,
		float64(g.pixels[off+2]) / 255,
	}
}

// RenderCells uses GPU pixel data to produce a cell grid.
func (g *GPURenderer) RenderCells(r *Renderer) [][]cell {
	tw, th := g.termW, g.termH

	if r.QuadrantMode {
		return g.renderCellsQuadrant(tw, th)
	}
	return g.renderCellsShaped(r, tw, th)
}

// renderCellsShaped uses shape matching on GPU pixel data (2×3 sub-pixels per cell).
func (g *GPURenderer) renderCellsShaped(r *Renderer, tw, th int) [][]cell {
	st := r.ShapeTable

	lines := make([][]cell, th)
	for cy := 0; cy < th; cy++ {
		line := make([]cell, tw)
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 3

			var sv ShapeVec
			sv[0] = g.pixelBrightness(bx, by)
			sv[1] = g.pixelBrightness(bx+1, by)
			sv[2] = g.pixelBrightness(bx, by+1)
			sv[3] = g.pixelBrightness(bx+1, by+1)
			sv[4] = g.pixelBrightness(bx, by+2)
			sv[5] = g.pixelBrightness(bx+1, by+2)

			avgBright := 0.0
			for i := 0; i < 6; i++ {
				avgBright += sv[i]
			}
			avgBright /= 6

			if avgBright < 0.01 {
				line[cx] = cell{ch: ' '}
				continue
			}

			var ext ShapeVec
			ext[0] = g.pixelBrightness(bx-1, by-1)
			ext[1] = g.pixelBrightness(bx+2, by-1)
			ext[2] = g.pixelBrightness(bx-1, by+1)
			ext[3] = g.pixelBrightness(bx+2, by+1)
			ext[4] = g.pixelBrightness(bx-1, by+3)
			ext[5] = g.pixelBrightness(bx+2, by+3)

			sv = DirectionalContrast(sv, ext, r.Contrast)
			sv = EnhanceContrast(sv, r.Contrast)
			ch := st.Match(sv)

			var colR, colG, colB float64
			for _, dy := range []int{0, 1, 2} {
				for _, dx := range []int{0, 1} {
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

			line[cx] = cell{ch: rune(ch), col: Vec3{colR / 6 / 255, colG / 6 / 255, colB / 6 / 255}}
		}
		lines[cy] = line
	}
	return lines
}

// renderCellsQuadrant uses 2×2 sub-pixels per cell to produce quadrant block characters.
func (g *GPURenderer) renderCellsQuadrant(tw, th int) [][]cell {
	lines := make([][]cell, th)
	for cy := 0; cy < th; cy++ {
		line := make([]cell, tw)
		for cx := 0; cx < tw; cx++ {
			bx := cx * 2
			by := cy * 2

			// Read 4 pixel colors in 2×2 grid: TL, TR, BL, BR
			colors := [4]Vec3{
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
				line[cx] = cell{ch: ' '}
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
					line[cx] = cell{ch: '█', col: V(clamp(avg.X, 0, 1), clamp(avg.Y, 0, 1), clamp(avg.Z, 0, 1))}
					continue
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
				line[cx] = cell{ch: ' '}
				continue
			}

			ch := quadrantChars[pattern]
			fg := fgCol.Mul(1.0 / float64(onCount))
			fg = V(clamp(fg.X, 0, 1), clamp(fg.Y, 0, 1), clamp(fg.Z, 0, 1))

			// Only set bg when off-pixels hit actual geometry
			if bgHitCount == 0 {
				line[cx] = cell{ch: ch, col: fg}
				continue
			}

			bg := bgCol.Mul(1.0 / float64(bgHitCount))
			bg = V(clamp(bg.X, 0, 1), clamp(bg.Y, 0, 1), clamp(bg.Z, 0, 1))

			line[cx] = cell{ch: ch, col: fg, bg: bg, hasBg: true}
		}
		lines[cy] = line
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
	fragSrc := assembleShader(code)
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
