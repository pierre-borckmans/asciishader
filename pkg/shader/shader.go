package shader

import "strings"

// ShaderPrefixUniforms contains just version, uniforms, and constants (no SDF library).
// Used for standalone GLSL files that define their own SDF primitives.
const ShaderPrefixUniforms = `#version 150
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
uniform int uProjection;  // 0=perspective, 1=orthographic, 2=isometric
uniform float uOrthoScale; // orthographic view scale

out vec4 fragColor;

const int MAX_STEPS = 80;
const float MAX_DIST = 50.0;
const float SURF_DIST = 0.005;
const float NORMAL_EPS = 0.001;
const float PI = 3.14159265;
`

// ShaderPrefix contains version, uniforms, constants, and the full SDF primitive library.
// Used for Chisel-compiled code that relies on built-in SDF functions.
const ShaderPrefix = ShaderPrefixUniforms + `
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

float sdPyramid(vec3 p, float h) {
    float m2 = h*h + 0.25;
    p.xz = abs(p.xz);
    p.xz = (p.z > p.x) ? p.zx : p.xz;
    p.xz -= 0.5;
    vec3 q = vec3(p.z, h*p.y - 0.5*p.x, h*p.x + 0.5*p.y);
    float s = max(-q.x, 0.0);
    float t = clamp((q.y - 0.5*p.z)/(m2 + 0.25), 0.0, 1.0);
    float a = m2*(q.x+s)*(q.x+s) + q.y*q.y;
    float b = m2*(q.x+0.5*t)*(q.x+0.5*t) + (q.y-m2*t)*(q.y-m2*t);
    float d2 = min(q.y, -q.x*m2 - q.y*0.5) > 0.0 ? 0.0 : min(a, b);
    return sqrt((d2 + q.z*q.z)/m2) * sign(max(q.z, -p.y));
}

float sdEllipsoid(vec3 p, vec3 r) {
    float k0 = length(p/r);
    float k1 = length(p/(r*r));
    return k0*(k0 - 1.0)/k1;
}

float sdRoundBox(vec3 p, vec3 b, float r) {
    vec3 q = abs(p) - b;
    return length(max(q, 0.0)) + min(max(q.x, max(q.y, q.z)), 0.0) - r;
}

float sdBoxFrame(vec3 p, vec3 b, float e) {
    p = abs(p) - b;
    vec3 q = abs(p + e) - e;
    return min(min(
        length(max(vec3(p.x, q.y, q.z), 0.0)) + min(max(p.x, max(q.y, q.z)), 0.0),
        length(max(vec3(q.x, p.y, q.z), 0.0)) + min(max(q.x, max(p.y, q.z)), 0.0)),
        length(max(vec3(q.x, q.y, p.z), 0.0)) + min(max(q.x, max(q.y, p.z)), 0.0));
}

float sdCappedTorus(vec3 p, vec2 sc, float ra, float rb) {
    p.x = abs(p.x);
    float k = (sc.y*p.x > sc.x*p.y) ? dot(p.xy, sc) : length(p.xy);
    return sqrt(dot(p, p) + ra*ra - 2.0*ra*k) - rb;
}

float sdHexPrism(vec3 p, vec2 h) {
    const vec3 k = vec3(-0.8660254, 0.5, 0.57735);
    p = abs(p);
    p.xy -= 2.0*min(dot(k.xy, p.xy), 0.0)*k.xy;
    vec2 d = vec2(
        length(p.xy - vec2(clamp(p.x, -k.z*h.x, k.z*h.x), h.x))*sign(p.y - h.x),
        p.z - h.y);
    return min(max(d.x, d.y), 0.0) + length(max(d, 0.0));
}

float sdOctogonPrism(vec3 p, float r, float h) {
    const vec3 k = vec3(-0.9238795325, 0.3826834323, 0.4142135623);
    p = abs(p);
    p.xy -= 2.0*min(dot(vec2(k.x, k.y), p.xy), 0.0)*vec2(k.x, k.y);
    p.xy -= 2.0*min(dot(vec2(-k.x, k.y), p.xy), 0.0)*vec2(-k.x, k.y);
    p.xy -= vec2(clamp(p.x, -k.z*r, k.z*r), r);
    vec2 d = vec2(length(p.xy)*sign(p.y), p.z - h);
    return min(max(d.x, d.y), 0.0) + length(max(d, 0.0));
}

float sdRoundCone(vec3 p, float r1, float r2, float h) {
    vec2 q = vec2(length(p.xz), p.y);
    float b = (r1 - r2)/h;
    float a = sqrt(1.0 - b*b);
    float k = dot(q, vec2(-b, a));
    if (k < 0.0) return length(q) - r1;
    if (k > a*h) return length(q - vec2(0.0, h)) - r2;
    return dot(q, vec2(a, b)) - r1;
}

float sdTriPrism(vec3 p, vec2 h) {
    const float k = sqrt(3.0);
    h.x *= 0.5*k;
    p.xy /= h.x;
    p.x = abs(p.x) - 1.0;
    p.y = p.y + 1.0/k;
    if (p.x + k*p.y > 0.0) p.xy = vec2(p.x - k*p.y, -k*p.x - p.y)/2.0;
    p.x -= clamp(p.x, -2.0, 0.0);
    float d1 = length(p.xy)*sign(-p.y)*h.x;
    float d2 = abs(p.z) - h.y;
    return length(max(vec2(d1, d2), 0.0)) + min(max(d1, d2), 0.0);
}

float sdCone(vec3 p, vec2 c, float h) {
    vec2 q = h*vec2(c.x, -c.y)/c.y;
    vec2 w = vec2(length(p.xz), p.y);
    vec2 a = w - q*clamp(dot(w, q)/dot(q, q), 0.0, 1.0);
    vec2 b = w - q*vec2(clamp(w.x/q.x, 0.0, 1.0), 1.0);
    float kk = sign(q.y);
    float d = min(dot(a, a), dot(b, b));
    float s = max(kk*(w.x*q.y - w.y*q.x), kk*(w.y - q.y));
    return sqrt(d)*sign(s);
}

float sdCappedCone(vec3 p, float h, float r1, float r2) {
    vec2 q = vec2(length(p.xz), p.y);
    vec2 k1 = vec2(r2, h);
    vec2 k2 = vec2(r2 - r1, 2.0*h);
    vec2 ca = vec2(q.x - min(q.x, (q.y < 0.0) ? r1 : r2), abs(q.y) - h);
    vec2 cb = q - k1 + k2*clamp(dot(k1 - q, k2)/dot(k2, k2), 0.0, 1.0);
    float s = (cb.x < 0.0 && ca.y < 0.0) ? -1.0 : 1.0;
    return s*sqrt(min(dot(ca, ca), dot(cb, cb)));
}

float sdSolidAngle(vec3 pos, vec2 c, float ra) {
    vec2 p = vec2(length(pos.xz), pos.y);
    float l = length(p) - ra;
    float m = length(p - c*clamp(dot(p, c), 0.0, ra));
    return max(l, m*sign(c.y*p.x - c.x*p.y));
}

float sdRhombus(vec3 p, float la, float lb, float h, float ra) {
    p = abs(p);
    vec2 b = vec2(la, lb);
    // ndot(a,b) = a.x*b.x - a.y*b.y (NOT regular dot product)
    float nd = b.x*(b.x - 2.0*p.x) - b.y*(b.y - 2.0*p.z);
    float f = clamp(nd / dot(b, b), -1.0, 1.0);
    vec2 q = vec2(length(p.xz - 0.5*b*vec2(1.0 - f, 1.0 + f))*sign(p.x*b.y + p.z*b.x - b.x*b.y) - ra, p.y - h);
    return min(max(q.x, q.y), 0.0) + length(max(q, 0.0));
}

float sdHorseshoe(vec3 p, vec2 c, float r, float le, vec2 w) {
    p.x = abs(p.x);
    float l = length(p.xy);
    p.xy = mat2(-c.x, c.y, c.y, c.x) * p.xy;
    p.xy = vec2((p.y > 0.0 || p.x > 0.0) ? p.x : l * sign(-c.x),
                (p.x > 0.0) ? p.y : l);
    p.xy = vec2(p.x, abs(p.y - r)) - vec2(le, 0.0);
    vec2 q = vec2(length(max(p.xy, 0.0)) + min(0.0, max(p.x, p.y)), p.z);
    vec2 d = abs(q) - w;
    return min(max(d.x, d.y), 0.0) + length(max(d, 0.0));
}

float sdCylinderAB(vec3 p, vec3 a, vec3 b, float r) {
    vec3 pa = p - a;
    vec3 ba = b - a;
    float baba = dot(ba, ba);
    float paba = dot(pa, ba);
    float x = length(pa * baba - ba * paba) - r * baba;
    float y = abs(paba - baba * 0.5) - baba * 0.5;
    float x2 = x * x;
    float y2 = y * y * baba;
    float d = (max(x, y) < 0.0) ? -min(x2, y2) : (((x > 0.0) ? x2 : 0.0) + ((y > 0.0) ? y2 : 0.0));
    return sign(d) * sqrt(abs(d)) / baba;
}

float sdCappedConeAB(vec3 p, vec3 a, vec3 b, float ra, float rb) {
    float rba = rb - ra;
    float baba = dot(b - a, b - a);
    float papa = dot(p - a, p - a);
    float paba = dot(p - a, b - a) / baba;
    float x = sqrt(papa - paba * paba * baba);
    float cax = max(0.0, x - ((paba < 0.5) ? ra : rb));
    float cay = abs(paba - 0.5) - 0.5;
    float k = rba * rba + baba;
    float f = clamp((rba * (x - ra) + paba * baba) / k, 0.0, 1.0);
    float cbx = x - ra - f * rba;
    float cby = paba - f;
    float s = (cbx < 0.0 && cay < 0.0) ? -1.0 : 1.0;
    return s * sqrt(min(cax * cax + cay * cay * baba, cbx * cbx + cby * cby * baba));
}

float sdRoundConeAB(vec3 p, vec3 a, vec3 b, float r1, float r2) {
    vec3 ba = b - a;
    float l2 = dot(ba, ba);
    float rr = r1 - r2;
    float a2 = l2 - rr * rr;
    float il2 = 1.0 / l2;
    vec3 pa = p - a;
    float y = dot(pa, ba);
    float z = y - l2;
    float x2 = dot(pa * l2 - ba * y, pa * l2 - ba * y);
    float y2 = y * y * l2;
    float z2 = z * z * l2;
    float k = sign(rr) * rr * rr * x2;
    if (sign(z) * a2 * z2 > k) return sqrt(x2 + z2) * il2 - r2;
    if (sign(y) * a2 * y2 < k) return sqrt(x2 + y2) * il2 - r1;
    return (sqrt(x2 * a2 * il2) + y * rr) * il2 - r1;
}

float sdRoundedCylinder(vec3 p, float ra, float rb, float h) {
    vec2 d = vec2(length(p.xz) - ra + rb, abs(p.y) - h);
    return min(max(d.x, d.y), 0.0) + length(max(d, 0.0)) - rb;
}

float sdTetrahedron(vec3 p, float r) {
    float md = max(max(-p.x - p.y - p.z, p.x + p.y - p.z),
                   max(-p.x + p.y + p.z, p.x - p.y + p.z));
    return (md - r) / sqrt(3.0);
}

float sdDodecahedron(vec3 p, float r) {
    // Golden ratio related constants
    float PHI = (1.0 + sqrt(5.0)) * 0.5;
    // Normals of the 6 unique face planes (other 6 are reflections)
    vec3 n1 = normalize(vec3(0, 1, PHI));
    vec3 n2 = normalize(vec3(0, 1, -PHI));
    vec3 n3 = normalize(vec3(1, PHI, 0));
    vec3 n4 = normalize(vec3(1, -PHI, 0));
    vec3 n5 = normalize(vec3(PHI, 0, 1));
    vec3 n6 = normalize(vec3(PHI, 0, -1));
    p = abs(p);
    float d = dot(p, n1);
    d = max(d, dot(p, n2));
    d = max(d, dot(p, n3));
    d = max(d, dot(p, n4));
    d = max(d, dot(p, n5));
    d = max(d, dot(p, n6));
    return d - r;
}

float sdIcosahedron(vec3 p, float r) {
    float PHI = (1.0 + sqrt(5.0)) * 0.5;
    vec3 n1 = normalize(vec3(1, PHI, 0));
    vec3 n2 = normalize(vec3(PHI, 0, 1));
    vec3 n3 = normalize(vec3(0, 1, PHI));
    vec3 n4 = normalize(vec3(1, 1, 1));
    vec3 n5 = normalize(vec3(PHI - 1.0, 0, PHI));
    p = abs(p);
    float d = dot(p, n1);
    d = max(d, dot(p, n2));
    d = max(d, dot(p, n3));
    d = max(d, dot(p, n4));
    d = max(d, dot(p, n5));
    return d - r;
}

float sdSlab(vec3 p, float h) {
    return abs(p.y) - h;
}

// ---- 2D SDF Primitives ----

float sdCircle2D(vec2 p, float r) {
    return length(p) - r;
}

float sdRect2D(vec2 p, vec2 b) {
    vec2 d = abs(p) - b;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0);
}

float sdRoundedRect2D(vec2 p, vec2 b, float r) {
    vec2 d = abs(p) - b;
    return length(max(d, 0.0)) + min(max(d.x, d.y), 0.0) - r;
}

float sdHexagon2D(vec2 p, float r) {
    const vec3 k = vec3(-0.866025404, 0.5, 0.577350269);
    p = abs(p);
    p -= 2.0 * min(dot(k.xy, p), 0.0) * k.xy;
    p -= vec2(clamp(p.x, -k.z * r, k.z * r), r);
    return length(p) * sign(p.y);
}

float sdEquilateralTriangle2D(vec2 p, float r) {
    const float k = sqrt(3.0);
    p.x = abs(p.x) - r;
    p.y = p.y + r / k;
    if (p.x + k * p.y > 0.0) p = vec2(p.x - k * p.y, -k * p.x - p.y) / 2.0;
    p.x -= clamp(p.x, -2.0 * r, 0.0);
    return -length(p) * sign(p.y);
}

// ---- 2D to 3D Operations ----

float sdExtrude(float d2d, float pz, float h) {
    vec2 w = vec2(d2d, abs(pz) - h);
    return min(max(w.x, w.y), 0.0) + length(max(w, 0.0));
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

// ShaderSuffix contains raymarch, shading pipeline, and main.
const ShaderSuffix = `
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

    float aspect = uTermSize.x / uTermSize.y * 0.45;
    vec3 ro, rd;

    if (uProjection == 1) {
        // Orthographic: parallel rays, offset origin
        float scale = uOrthoScale;
        ro = uCameraPos + right * ndc.x * scale * aspect + up * ndc.y * scale;
        rd = fwd;
    } else if (uProjection == 2) {
        // Isometric: orthographic at fixed angle
        float scale = uOrthoScale;
        ro = uCameraPos + right * ndc.x * scale * aspect + up * ndc.y * scale;
        rd = fwd;
    } else {
        // Perspective (default)
        float fovRad = uFOV * 3.14159265 / 180.0;
        float halfH = tan(fovRad / 2.0);
        float halfW = halfH * aspect;
        rd = normalize(fwd + right * ndc.x * halfW + up * ndc.y * halfH);
        ro = uCameraPos;
    }

    float t = raymarch(ro, rd);

    vec4 result = vec4(0);
    if (t < MAX_DIST) {
        result = shade(ro, rd, t);
    }

    fragColor = result;
}
`

// ShaderRawPrefix provides uniforms and Shadertoy compatibility aliases.
const ShaderRawPrefix = `#version 150
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

// ShaderRawSuffix wraps mainImage into main().
const ShaderRawSuffix = `
void main() {
    vec2 fc = vec2(gl_FragCoord.x, uResolution.y - gl_FragCoord.y);
    mainImage(fragColor, fc);
    float luma = dot(fragColor.rgb, vec3(0.299, 0.587, 0.114));
    fragColor.a = pow(luma, 0.5);
}
`

// DefaultUserCode is a minimal structured shader.
const DefaultUserCode = `
float sceneSDF(vec3 p) { return sdSphere(p, 1.0); }
vec3 sceneColor(vec3 p) { return vec3(1); }
`

// IsRawShader returns true if the code contains mainImage.
func IsRawShader(code string) bool {
	return strings.Contains(code, "mainImage")
}

// PrefixLineCount returns the number of lines in the prefix used for the given code.
func PrefixLineCount(code string) int {
	if IsRawShader(code) {
		return strings.Count(ShaderRawPrefix, "\n")
	}
	return strings.Count(ShaderPrefix, "\n")
}

// Assemble combines the appropriate prefix + user code + suffix into
// a complete fragment shader.
func Assemble(userCode string) string {
	if IsRawShader(userCode) {
		return ShaderRawPrefix + userCode + "\n" + ShaderRawSuffix + "\x00"
	}
	return ShaderPrefix + userCode + "\n" + ShaderSuffix + "\x00"
}

// AssembleGLSL assembles a standalone GLSL file that defines its own SDF
// primitives. Uses minimal prefix (uniforms only, no SDF library) to avoid
// redefinition conflicts.
func AssembleGLSL(userCode string) string {
	if IsRawShader(userCode) {
		return ShaderRawPrefix + userCode + "\n" + ShaderRawSuffix + "\x00"
	}
	return ShaderPrefixUniforms + userCode + "\n" + ShaderSuffix + "\x00"
}
