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

const fragmentShaderSource = `#version 150
uniform vec2 uResolution;
uniform float uTime;
uniform vec3 uCameraPos;
uniform vec3 uCameraTarget;
uniform vec3 uLightDir;
uniform float uFOV;
uniform float uAmbient;
uniform float uSpecPower;
uniform int uShadowSteps;
uniform int uAOSteps;
uniform vec2 uTermSize;

out vec4 fragColor;

const int MAX_STEPS = 80;
const float MAX_DIST = 50.0;
const float SURF_DIST = 0.005;
const float NORMAL_EPS = 0.001;

// ---- SDF Primitives ----
float sdSphere(vec3 p, float r) {
    return length(p) - r;
}

float sdTorus(vec3 p, float R, float r) {
    float q = length(p.xz) - R;
    return length(vec2(q, p.y)) - r;
}

// ---- Operations ----
float opSmoothUnion(float a, float b, float k) {
    float h = clamp(0.5 + 0.5*(b-a)/k, 0.0, 1.0);
    return mix(b, a, h) - k*h*(1.0-h);
}

float opSubtract(float a, float b) {
    return max(a, -b);
}

// ---- Rotation ----
vec3 rotateY(vec3 p, float a) {
    float c = cos(a), s = sin(a);
    return vec3(p.x*c + p.z*s, p.y, -p.x*s + p.z*c);
}

vec3 rotateX(vec3 p, float a) {
    float c = cos(a), s = sin(a);
    return vec3(p.x, p.y*c - p.z*s, p.y*s + p.z*c);
}

// ---- Scene: Plasma Orb ----
float sceneSDF(vec3 p) {
    p = rotateY(p, uTime * 0.4);
    p = rotateX(p, uTime * 0.15);

    float d = sdSphere(p, 1.3);

    float disp1 = sin(p.x*4.0+uTime*1.5) * cos(p.y*3.0+uTime*1.2) * sin(p.z*4.0+uTime*1.8) * 0.15;
    float disp2 = sin(p.x*8.0+uTime*2.5) * sin(p.y*7.0-uTime*2.0) * cos(p.z*6.0+uTime*1.3) * 0.06;
    d += disp1 + disp2;

    float inner = sdSphere(p, 0.5 + sin(uTime*1.5)*0.15);
    d = opSubtract(d, inner);

    for (int i = 0; i < 3; i++) {
        float a = uTime*1.2 + float(i)*6.283185/3.0;
        vec3 sp = vec3(cos(a)*1.6, sin(a*0.7)*0.4, sin(a)*1.6);
        d = opSmoothUnion(d, sdSphere(p - sp, 0.15), 0.3);
    }

    return d;
}

// ---- Plasma Orb Color: cyan core, magenta edges ----
vec3 sceneColor(vec3 p) {
    p = rotateY(p, uTime * 0.4);
    p = rotateX(p, uTime * 0.15);

    float dist = length(p);
    float wave = sin(p.x*4.0+uTime*1.5)*cos(p.y*3.0+uTime*1.2) + sin(p.z*4.0+uTime*1.8);
    float f = wave*0.25 + 0.5;

    float r = 0.4 + 0.6*f;
    float g = 0.3 + 0.7*(1.0-f);
    float b = 0.8 + 0.2*sin(dist*3.0+uTime);
    return clamp(vec3(r, g, b), 0.0, 1.0);
}

// ---- Raymarching ----
float raymarch(vec3 ro, vec3 rd) {
    float t = 0.0;
    for (int i = 0; i < MAX_STEPS; i++) {
        vec3 p = ro + rd * t;
        float d = sceneSDF(p);
        if (d < SURF_DIST) return t;
        t += d;
        if (t > MAX_DIST) break;
    }
    return MAX_DIST;
}

// ---- Shading ----
vec3 calcNormal(vec3 p) {
    float e = NORMAL_EPS;
    float d = sceneSDF(p);
    return normalize(vec3(
        sceneSDF(vec3(p.x+e, p.y, p.z)) - d,
        sceneSDF(vec3(p.x, p.y+e, p.z)) - d,
        sceneSDF(vec3(p.x, p.y, p.z+e)) - d
    ));
}

float softShadow(vec3 ro, vec3 rd, float mint, float maxt, float k) {
    if (uShadowSteps <= 0) return 1.0;
    float res = 1.0;
    float t = mint;
    for (int i = 0; i < 48; i++) {
        if (i >= uShadowSteps) break;
        vec3 p = ro + rd * t;
        float d = sceneSDF(p);
        if (d < SURF_DIST * 0.5) return 0.0;
        res = min(res, k*d/t);
        t += clamp(d, 0.02, 0.2);
        if (t > maxt) break;
    }
    return clamp(res, 0.0, 1.0);
}

float ambientOcclusion(vec3 p, vec3 n) {
    if (uAOSteps <= 0) return 1.0;
    float occ = 0.0;
    float scale = 1.0;
    for (int i = 0; i < 10; i++) {
        if (i >= uAOSteps) break;
        float h = 0.01 + 0.12 * float(i);
        float d = sceneSDF(p + n * h);
        occ += (h - d) * scale;
        scale *= 0.75;
    }
    return clamp(1.0 - 1.5*occ, 0.0, 1.0);
}

// Returns vec4: RGB = material-colored shading, A = lighting-only brightness.
// Alpha stores the same brightness the CPU shade() returns (no material color),
// so the CPU-side shape matching uses identical values.
vec4 shade(vec3 ro, vec3 rd, float t) {
    vec3 p = ro + rd * t;
    vec3 n = calcNormal(p);
    vec3 mat = sceneColor(p);

    float diff = clamp(dot(n, uLightDir), 0.0, 1.0);
    float shadow = softShadow(p + n*0.02, uLightDir, 0.02, 10.0, 16.0);
    diff *= shadow;

    float spec = 0.0;
    if (uShadowSteps > 0) {
        vec3 half_v = normalize(uLightDir - rd);
        spec = pow(clamp(dot(n, half_v), 0.0, 1.0), uSpecPower) * shadow;
    }

    float ao = ambientOcclusion(p, n);
    float fresnel = 0.0;
    if (uAOSteps > 0) {
        fresnel = pow(1.0 - clamp(dot(-rd, n), 0.0, 1.0), 3.0) * 0.3;
    }

    float ambient = uAmbient * ao;
    float diffContrib = diff * 0.65 * ao;
    vec3 col = mat * (ambient + diffContrib);
    col += vec3(1.0) * spec * 0.25;
    col += mat * fresnel * ao;

    float fog = exp(-t * t * 0.008);
    col *= fog;

    // Lighting-only brightness (matches CPU shade())
    float brightness = (ambient + diffContrib + spec * 0.25 + fresnel * ao) * fog;

    return vec4(clamp(col, 0.0, 1.0), clamp(brightness, 0.0, 1.0));
}

void main() {
    vec2 ndc;
    ndc.x = gl_FragCoord.x / uResolution.x * 2.0 - 1.0;
    ndc.y = 1.0 - gl_FragCoord.y / uResolution.y * 2.0;

    vec3 fwd = normalize(uCameraTarget - uCameraPos);
    vec3 right = normalize(cross(fwd, vec3(0, 1, 0)));
    vec3 up = cross(right, fwd);

    float fovRad = uFOV * 3.14159265 / 180.0;
    float halfH = tan(fovRad / 2.0);
    float aspect = uTermSize.x / uTermSize.y * 0.45;
    float halfW = halfH * aspect;

    vec3 rd = normalize(fwd + right * ndc.x * halfW + up * ndc.y * halfH);
    vec3 ro = uCameraPos;

    float t = raymarch(ro, rd);

    vec4 result = vec4(0);
    if (t < MAX_DIST) {
        result = shade(ro, rd, t);
    }

    fragColor = result;
}
` + "\x00"

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

	program, err := createProgram(vertexShaderSource, fragmentShaderSource)
	if err != nil {
		return nil, fmt.Errorf("shader program: %v", err)
	}

	vao := createQuadVAO()

	g := &GPURenderer{
		program: program,
		vao:     vao,
	}

	// Cache uniform locations
	gl.UseProgram(program)
	g.uResolution = gl.GetUniformLocation(program, gl.Str("uResolution\x00"))
	g.uTime = gl.GetUniformLocation(program, gl.Str("uTime\x00"))
	g.uCameraPos = gl.GetUniformLocation(program, gl.Str("uCameraPos\x00"))
	g.uCameraTarget = gl.GetUniformLocation(program, gl.Str("uCameraTarget\x00"))
	g.uLightDir = gl.GetUniformLocation(program, gl.Str("uLightDir\x00"))
	g.uFOV = gl.GetUniformLocation(program, gl.Str("uFOV\x00"))
	g.uAmbient = gl.GetUniformLocation(program, gl.Str("uAmbient\x00"))
	g.uSpecPower = gl.GetUniformLocation(program, gl.Str("uSpecPower\x00"))
	g.uShadowSteps = gl.GetUniformLocation(program, gl.Str("uShadowSteps\x00"))
	g.uAOSteps = gl.GetUniformLocation(program, gl.Str("uAOSteps\x00"))
	g.uTermSize = gl.GetUniformLocation(program, gl.Str("uTermSize\x00"))

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
