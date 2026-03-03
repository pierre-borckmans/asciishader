package main

import "strings"

// shaderPrefix contains version, uniforms, constants, and the full SDF primitive library.
// User code comes after this and must define sceneSDF(vec3) and sceneColor(vec3).
const shaderPrefix = `#version 150
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
const float PI = 3.14159265;

// ---- SDF Primitives ----

float sdSphere(vec3 p, float r) {
    return length(p) - r;
}

float sdTorus(vec3 p, float R, float r) {
    float q = length(p.xz) - R;
    return length(vec2(q, p.y)) - r;
}

float sdBox(vec3 p, vec3 b) {
    vec3 q = abs(p) - b;
    return length(max(q, 0.0)) + min(max(q.x, max(q.y, q.z)), 0.0);
}

float sdCylinder(vec3 p, float r, float h) {
    vec2 d = abs(vec2(length(p.xz), p.y)) - vec2(r, h);
    return min(max(d.x, d.y), 0.0) + length(max(d, 0.0));
}

float sdPlane(vec3 p, vec3 n, float h) {
    return dot(p, n) + h;
}

float sdCapsule(vec3 p, vec3 a, vec3 b, float r) {
    vec3 pa = p - a, ba = b - a;
    float h = clamp(dot(pa, ba) / dot(ba, ba), 0.0, 1.0);
    return length(pa - ba * h) - r;
}

float sdOctahedron(vec3 p, float s) {
    p = abs(p);
    return (p.x + p.y + p.z - s) * 0.57735027;
}

// ---- Operations ----

float opUnion(float a, float b) { return min(a, b); }
float opSubtract(float a, float b) { return max(a, -b); }
float opIntersect(float a, float b) { return max(a, b); }

float opSmoothUnion(float a, float b, float k) {
    float h = clamp(0.5 + 0.5*(b-a)/k, 0.0, 1.0);
    return mix(b, a, h) - k*h*(1.0-h);
}

float opRound(float d, float r) { return d - r; }

vec3 opRep(vec3 p, vec3 c) {
    return mod(p + 0.5*c, c) - 0.5*c;
}

vec3 opRepXZ(vec3 p, float cx, float cz) {
    p.x = mod(p.x + 0.5*cx, cx) - 0.5*cx;
    p.z = mod(p.z + 0.5*cz, cz) - 0.5*cz;
    return p;
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

// ---- User code below (must define sceneSDF and sceneColor) ----
`

// shaderSuffix contains raymarch, shading pipeline, and main.
// It calls sceneSDF() and sceneColor() which are defined in user code.
const shaderSuffix = `
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

vec4 shade(vec3 ro, vec3 rd, float t) {
    vec3 p = ro + rd * t;
    vec3 n = calcNormal(p);
    vec3 mat = sceneColor(p);

    float diff = clamp(dot(n, uLightDir), 0.0, 1.0);
    float shadow = softShadow(p + n*0.02, uLightDir, 0.02, 10.0, 16.0);
    shadow = mix(1.0, shadow, 0.6); // soften shadow intensity
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
`

// shaderRawPrefix provides uniforms and Shadertoy compatibility aliases
// for raw shaders that define mainImage() instead of sceneSDF()/sceneColor().
const shaderRawPrefix = `#version 150
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

#define iResolution vec3(uResolution, 1.0)
#define iTime uTime
#define iMouse vec4(0)
#define iFrame 0
#define HW_PERFORMANCE 1
`

// shaderRawSuffix wraps mainImage into main(), flips Y to match the structured
// renderer's coordinate convention, and stores brightness in alpha for ASCII
// shape matching.
const shaderRawSuffix = `
void main() {
    vec2 fc = vec2(gl_FragCoord.x, uResolution.y - gl_FragCoord.y);
    mainImage(fragColor, fc);
    float luma = dot(fragColor.rgb, vec3(0.299, 0.587, 0.114));
    fragColor.a = pow(luma, 0.5);
}
`

// defaultUserCode is a minimal structured shader used for GPU init before
// external shader files are loaded.
const defaultUserCode = `
float sceneSDF(vec3 p) { return sdSphere(p, 1.0); }
vec3 sceneColor(vec3 p) { return vec3(1); }
`

// isRawShader returns true if the code contains mainImage, indicating a
// Shadertoy-style shader that should use the raw prefix/suffix instead of
// the structured SDF pipeline.
func isRawShader(code string) bool {
	return strings.Contains(code, "mainImage")
}

// PrefixLineCount returns the number of lines in the prefix used for the
// given code. Raw shaders use the shorter raw prefix; structured shaders
// use the full SDF prefix.
func PrefixLineCount(code string) int {
	if isRawShader(code) {
		return strings.Count(shaderRawPrefix, "\n")
	}
	return strings.Count(shaderPrefix, "\n")
}

// assembleShader combines the appropriate prefix + user code + suffix into
// a complete fragment shader. Raw shaders (containing mainImage) get the
// minimal raw prefix/suffix; structured shaders get the full SDF pipeline.
func assembleShader(userCode string) string {
	if isRawShader(userCode) {
		return shaderRawPrefix + userCode + "\n" + shaderRawSuffix + "\x00"
	}
	return shaderPrefix + userCode + "\n" + shaderSuffix + "\x00"
}
