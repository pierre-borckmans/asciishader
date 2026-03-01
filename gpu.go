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
	"strconv"
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
	pixW    int // pixel width (termW * 2)
	pixH    int // pixel height (termH * 3)
	termW   int // terminal columns
	termH   int // terminal rows
	pixels  []byte

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

func (g *GPURenderer) resize(termW, termH int) {
	if termW == g.termW && termH == g.termH {
		return
	}
	g.termW = termW
	g.termH = termH
	g.pixW = termW * 2
	g.pixH = termH * 3
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
// Renders at 2x3 sub-pixel resolution per terminal cell, then uses
// CPU-side shape matching for high-quality character selection.
func (g *GPURenderer) Render(r *Renderer) string {
	w, h := r.Width, r.Height
	if w <= 0 || h <= 0 {
		return ""
	}

	g.resize(w, h)

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

	// Shape matching on CPU using GPU pixel data
	return g.buildANSIShaped(r)
}

// pixelBrightness returns the lighting-only brightness (0-1) stored in the alpha channel.
func (g *GPURenderer) pixelBrightness(px, py int) float64 {
	if px < 0 || px >= g.pixW || py < 0 || py >= g.pixH {
		return 0
	}
	off := (py*g.pixW + px) * 4
	return float64(g.pixels[off+3]) / 255
}

// buildANSIShaped reads the 2x3 sub-pixel grid per terminal cell,
// applies shape matching (same as CPU renderer), and outputs ANSI-colored text.
func (g *GPURenderer) buildANSIShaped(r *Renderer) string {
	tw, th := g.termW, g.termH
	st := r.ShapeTable

	out := make([]byte, 0, tw*th*20+th*10)
	prevR, prevG, prevB := -1, -1, -1

	for cy := 0; cy < th; cy++ {
		if cy > 0 {
			out = append(out, '\n')
		}
		prevR, prevG, prevB = -1, -1, -1

		for cx := 0; cx < tw; cx++ {
			// Base pixel coordinates for this cell's 2x3 block
			bx := cx * 2
			by := cy * 3

			// Internal samples: 2 cols x 3 rows = ShapeVec [TL,TR,ML,MR,BL,BR]
			var sv ShapeVec
			sv[0] = g.pixelBrightness(bx, by)     // TL
			sv[1] = g.pixelBrightness(bx+1, by)   // TR
			sv[2] = g.pixelBrightness(bx, by+1)   // ML
			sv[3] = g.pixelBrightness(bx+1, by+1) // MR
			sv[4] = g.pixelBrightness(bx, by+2)   // BL
			sv[5] = g.pixelBrightness(bx+1, by+2) // BR

			// Average brightness for background detection
			avgBright := 0.0
			for i := 0; i < 6; i++ {
				avgBright += sv[i]
			}
			avgBright /= 6

			if avgBright < 0.01 {
				out = append(out, ' ')
				continue
			}

			// External samples: one pixel outside cell boundary in each direction
			var ext ShapeVec
			ext[0] = g.pixelBrightness(bx-1, by-1) // TL external
			ext[1] = g.pixelBrightness(bx+2, by-1) // TR external
			ext[2] = g.pixelBrightness(bx-1, by+1) // ML external
			ext[3] = g.pixelBrightness(bx+2, by+1) // MR external
			ext[4] = g.pixelBrightness(bx-1, by+3) // BL external
			ext[5] = g.pixelBrightness(bx+2, by+3) // BR external

			// Shape matching (same pipeline as CPU)
			sv = DirectionalContrast(sv, ext, r.Contrast)
			sv = EnhanceContrast(sv, r.Contrast)
			ch := st.Match(sv)

			// Color: average the 6 sub-pixels for a smooth cell color
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
			cr := int(colR / 6)
			cg := int(colG / 6)
			cb := int(colB / 6)

			if ch == ' ' {
				out = append(out, ' ')
				continue
			}

			if cr != prevR || cg != prevG || cb != prevB {
				out = append(out, "\033[38;2;"...)
				out = strconv.AppendInt(out, int64(cr), 10)
				out = append(out, ';')
				out = strconv.AppendInt(out, int64(cg), 10)
				out = append(out, ';')
				out = strconv.AppendInt(out, int64(cb), 10)
				out = append(out, 'm')
				prevR, prevG, prevB = cr, cg, cb
			}
			out = append(out, ch)
		}
		out = append(out, "\033[0m"...)
	}
	return string(out)
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
