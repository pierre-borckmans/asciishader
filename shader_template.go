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

// defaultUserCode is the Plasma Orb scene — the default user-editable GLSL code.
const defaultUserCode = `// ---- Scene: Plasma Orb ----
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
`

const railwayExpressGLSL = `// ---- Scene: Railway Express ----
// A sleek bullet train on elevated tracks racing through
// glowing portal rings in Railway's purple/cyan/magenta palette.

float sceneSDF(vec3 p) {
    float speed = uTime * 3.0;

    // === BULLET TRAIN ===
    vec3 tp = p - vec3(0, 0.35, 1.5);

    // Main fuselage — rounded box
    float body = sdBox(tp - vec3(0, 0, 2.5), vec3(0.75, 0.6, 3.2));
    body = opRound(body, 0.22);

    // Aerodynamic nose — stretched sphere
    vec3 np = tp;
    np.z *= 0.36;
    float nose = sdSphere(np, 0.82);
    float train = opSmoothUnion(body, nose, 0.5);

    // Windshield recess
    float ws = sdBox(tp - vec3(0, 0.6, 0.15), vec3(0.38, 0.16, 0.08));
    train = opSubtract(train, ws);

    // Racing stripe groove
    float stripe = sdBox(tp - vec3(0, 0.32, 2.0), vec3(1.05, 0.02, 4.2));
    train = opSubtract(train, stripe);

    // Headlights
    train = opUnion(train, sdSphere(tp - vec3(-0.45, -0.05, -0.58), 0.07));
    train = opUnion(train, sdSphere(tp - vec3( 0.45, -0.05, -0.58), 0.07));

    // Roof antenna
    train = opUnion(train, sdBox(tp - vec3(0, 0.95, 3.0), vec3(0.06, 0.02, 1.8)));

    // === ENERGY RAILS ===
    float r1 = sdBox(p - vec3(-0.55, -0.28, 0), vec3(0.035, 0.035, 50.0));
    float r2 = sdBox(p - vec3( 0.55, -0.28, 0), vec3(0.035, 0.035, 50.0));
    float rails = opUnion(r1, r2);

    // Track bed
    float bed = sdBox(p - vec3(0, -0.35, 0), vec3(0.72, 0.025, 50.0));
    rails = opUnion(rails, bed);

    // Sliding ties (speed illusion)
    vec3 tieP = p - vec3(0, -0.40, 0);
    tieP.z = mod(tieP.z + speed + 0.35, 0.7) - 0.35;
    float ties = sdBox(tieP, vec3(0.88, 0.012, 0.035));
    rails = opUnion(rails, ties);

    // === PORTAL RINGS ===
    // Torus oriented in XY plane, sliding toward camera
    vec3 ringP = p;
    float ringRep = 5.0;
    ringP.z = mod(ringP.z + speed * 0.7 + ringRep * 0.5, ringRep) - ringRep * 0.5;
    float ring = sdTorus(vec3(ringP.x, ringP.z, ringP.y - 0.15), 2.4, 0.04);

    // === SUPPORT PYLONS ===
    vec3 pylP = p;
    float pylRep = 5.0;
    pylP.z = mod(pylP.z + pylRep * 0.5, pylRep) - pylRep * 0.5;
    float pilL = sdCylinder(pylP - vec3(-0.7, -1.25, 0), 0.05, 0.92);
    float pilR = sdCylinder(pylP - vec3( 0.7, -1.25, 0), 0.05, 0.92);
    float pylons = opUnion(pilL, pilR);
    float brace = sdBox(pylP - vec3(0, -1.75, 0), vec3(0.82, 0.025, 0.05));
    pylons = opUnion(pylons, brace);

    // === GROUND ===
    float ground = sdPlane(p, vec3(0, 1, 0), 2.2);

    // Combine
    float scene = opUnion(train, rails);
    scene = opUnion(scene, ring);
    scene = opUnion(scene, pylons);
    scene = opUnion(scene, ground);
    return scene;
}

vec3 sceneColor(vec3 p) {
    float speed = uTime * 3.0;

    // Railway palette — brightened for shading pipeline
    vec3 lavender = vec3(0.7, 0.55, 0.9);
    vec3 purple   = vec3(0.6, 0.35, 0.95);
    vec3 cyan     = vec3(0.3, 0.95, 1.0);
    vec3 magenta  = vec3(1.0, 0.55, 0.85);
    vec3 white    = vec3(0.97);

    // --- Train body ---
    vec3 tp = p - vec3(0, 0.35, 1.5);
    float bodyD = opRound(sdBox(tp - vec3(0, 0, 2.5), vec3(0.75, 0.6, 3.2)), 0.22);
    vec3 np = tp; np.z *= 0.36;
    float noseD = sdSphere(np, 0.82);
    float trainD = min(bodyD, noseD);

    if (trainD < 0.15) {
        // Bright white body with lavender tint
        vec3 col = vec3(0.9, 0.88, 0.95);

        // Cyan racing stripe with energy pulse
        if (abs(tp.y - 0.32) < 0.06) {
            float pulse = sin(tp.z * 2.5 - uTime * 6.0) * 0.5 + 0.5;
            col = mix(cyan * 0.7, cyan, pulse);
        }

        // Windshield: bright cyan glass
        if (tp.y > 0.42 && tp.z < 0.35 && abs(tp.x) < 0.42) {
            col = mix(lavender, cyan * 0.8, 0.5);
        }

        // Headlights
        float hlD = min(
            length(tp - vec3(-0.45, -0.05, -0.58)),
            length(tp - vec3( 0.45, -0.05, -0.58))
        );
        if (hlD < 0.1) col = white;

        // Nose tip: purple accent
        if (tp.z < 0.0) col = mix(col, purple, 0.3);

        return clamp(col, 0.0, 1.0);
    }

    // --- Rails: pulsing cyan energy ---
    if (p.y > -0.38 && p.y < -0.18) {
        if (abs(abs(p.x) - 0.55) < 0.07) {
            float pulse = sin(p.z * 3.0 - uTime * 8.0) * 0.5 + 0.5;
            return mix(cyan * 0.6, cyan, pulse);
        }
        return lavender * 0.9;
    }

    // --- Ties ---
    if (p.y > -0.45 && p.y < -0.36) {
        return lavender * 0.75;
    }

    // --- Portal rings: magenta/purple energy swirl ---
    vec3 ringP = p;
    float ringRep = 5.0;
    ringP.z = mod(ringP.z + speed * 0.7 + ringRep * 0.5, ringRep) - ringRep * 0.5;
    float ringDist = abs(length(vec2(ringP.x, ringP.y - 0.15)) - 2.4);
    if (ringDist < 0.12) {
        float angle = atan(ringP.y - 0.15, ringP.x);
        float glow = sin(angle * 4.0 + uTime * 3.5) * 0.5 + 0.5;
        return mix(purple, magenta, glow);
    }

    // --- Support pylons ---
    if (p.y < -0.35 && p.y > -2.3) {
        return lavender * 0.65;
    }

    // --- Ground: lavender with scrolling vaporwave grid ---
    float gx = smoothstep(0.03, 0.0, abs(fract(p.x * 0.5) - 0.5) - 0.47);
    float gz = smoothstep(0.03, 0.0, abs(fract(p.z * 0.25 + uTime * 0.5) - 0.5) - 0.47);
    float grid = max(gx, gz);
    return mix(vec3(0.25, 0.2, 0.35), purple * 0.7, grid);
}
`

const lavaLampGLSL = `// ---- Scene: Lava Lamp ----
// Warm blobby masses rising and sinking, merging and splitting.

float sceneSDF(vec3 p) {
    p = rotateY(p, uTime * 0.12);

    // Central mass
    float d = sdSphere(p, 0.6 + sin(uTime * 0.7) * 0.1);

    // Rising/sinking blobs
    for (int i = 0; i < 8; i++) {
        float fi = float(i);
        float phase = uTime * (0.35 + fi * 0.06) + fi * 1.8;
        float y = sin(phase) * 1.6;
        float swing = 0.9 + sin(fi * 2.3) * 0.3;
        float a = fi * 0.785 + uTime * 0.2;
        vec3 sp = vec3(cos(a) * swing, y, sin(a) * swing);
        float sr = 0.25 + sin(fi * 1.1 + uTime * 0.5) * 0.1;
        d = opSmoothUnion(d, sdSphere(p - sp, sr), 0.55);
    }

    // Smaller orbiting droplets
    for (int i = 0; i < 5; i++) {
        float fi = float(i);
        float a = uTime * 0.8 + fi * 1.257;
        float r = 1.8 + sin(uTime * 0.3 + fi) * 0.3;
        float y = cos(uTime * 0.6 + fi * 0.9) * 0.8;
        d = opSmoothUnion(d, sdSphere(p - vec3(cos(a)*r, y, sin(a)*r), 0.12), 0.4);
    }

    return d;
}

vec3 sceneColor(vec3 p) {
    p = rotateY(p, uTime * 0.12);
    float dist = length(p);

    // Height-based temperature: hot orange core, cool red edges
    float h = p.y * 0.3 + 0.5;
    float wave = sin(p.x*3.0 + uTime) * sin(p.z*3.0 + uTime*0.8) * 0.2;
    h += wave;

    float r = 1.0;
    float g = 0.3 + 0.5 * clamp(h, 0.0, 1.0);
    float b = 0.05 + 0.2 * clamp(h * h, 0.0, 1.0);

    // Core glow
    float core = exp(-dist * 1.5);
    g += core * 0.3;
    b += core * 0.1;

    return clamp(vec3(r, g, b), 0.0, 1.0);
}
`

const mercuryGLSL = `// ---- Scene: Mercury ----
// Liquid metal ball with droplets being absorbed and ejected.

float sceneSDF(vec3 p) {
    p = rotateY(p, uTime * 0.25);

    // Main mass — slightly squished, with surface tension ripple
    vec3 mp = vec3(p.x, p.y * 0.85, p.z);
    float d = sdSphere(mp, 0.9);
    float ripple = sin(p.x*6.0 + uTime*2.5) * cos(p.z*6.0 + uTime*1.8) * 0.04;
    d += ripple;

    // Droplets — orbit in and out, merging on approach
    for (int i = 0; i < 7; i++) {
        float fi = float(i);
        float phase = uTime * 0.7 + fi * 0.898;
        float extend = sin(phase) * 0.5 + 0.5;
        float orbit = 0.7 + extend * 1.4;
        float a = fi * 0.898 + uTime * 0.35;
        float y = sin(fi * 0.8 + uTime * 0.4) * 0.5 * extend;
        vec3 sp = vec3(cos(a) * orbit, y, sin(a) * orbit);
        float sr = 0.18 - extend * 0.06;
        float k = 0.25 + extend * 0.45;
        d = opSmoothUnion(d, sdSphere(p - sp, sr), k);
    }

    // Satellite ring — thin torus
    float ring = sdTorus(p, 2.0, 0.02);
    d = opUnion(d, ring);

    // Ground reflection plane
    float ground = sdPlane(p, vec3(0, 1, 0), 2.0);
    return opUnion(d, ground);
}

vec3 sceneColor(vec3 p) {
    p = rotateY(p, uTime * 0.25);
    float dist = length(p);

    // Ground
    if (p.y < -1.9) {
        float refl = exp(-dist * 0.3);
        return vec3(0.15 + refl * 0.1, 0.15 + refl * 0.1, 0.18 + refl * 0.12);
    }

    // Ring
    float ringD = abs(length(p.xz) - 2.0);
    if (ringD < 0.06 && abs(p.y) < 0.06) {
        return vec3(0.7, 0.75, 0.8);
    }

    // Mercury surface — chrome with rainbow caustics
    float angle = atan(p.z, p.x);
    float caustic = sin(angle * 5.0 + p.y * 8.0 + uTime * 2.0) * 0.5 + 0.5;
    float fresnel = pow(1.0 - clamp(abs(dist - 0.9) * 3.0, 0.0, 1.0), 2.0);

    // Base chrome
    vec3 col = vec3(0.8, 0.82, 0.85);

    // Rainbow tint on edges
    vec3 rainbow;
    float h = angle + p.y * 2.0 + uTime * 0.5;
    rainbow.r = sin(h) * 0.5 + 0.5;
    rainbow.g = sin(h + 2.094) * 0.5 + 0.5;
    rainbow.b = sin(h + 4.189) * 0.5 + 0.5;

    col = mix(col, rainbow * 0.7 + 0.3, fresnel * 0.4 + caustic * 0.15);

    return clamp(col, 0.0, 1.0);
}
`

const amoebaGLSL = `// ---- Scene: Amoeba ----
// Pulsating organic creature with extending pseudopods.

float sceneSDF(vec3 p) {
    p = rotateY(p, uTime * 0.18);
    p = rotateX(p, sin(uTime * 0.3) * 0.15);

    // Core body — breathing sphere
    float breath = sin(uTime * 1.2) * 0.1;
    float d = sdSphere(p, 0.7 + breath);

    // Organic displacement on the body
    float disp = sin(p.x*5.0 + uTime*1.5) * sin(p.y*4.0 + uTime*1.2) * sin(p.z*5.0 + uTime*1.8) * 0.08;
    d += disp;

    // Pseudopods — extending arms reaching outward
    for (int i = 0; i < 5; i++) {
        float fi = float(i);
        float a = fi * 1.257 + sin(uTime * 0.4 + fi) * 0.3;
        float extend = sin(uTime * 0.6 + fi * 1.3) * 0.5 + 0.5;
        float reach = 0.6 + extend * 1.2;

        // Each pseudopod is a chain of 3 spheres
        for (int j = 0; j < 3; j++) {
            float fj = float(j);
            float t = (fj + 1.0) / 3.0;
            float y = sin(fi * 0.7 + uTime * 0.5) * 0.3 * t;
            float wobble = sin(uTime * 1.5 + fi * 2.0 + fj * 1.5) * 0.15 * t;
            vec3 sp = vec3(cos(a) * reach * t + wobble, y, sin(a) * reach * t);
            float sr = 0.2 - t * 0.1;
            d = opSmoothUnion(d, sdSphere(p - sp, sr), 0.35);
        }
    }

    // Nucleus — inner sphere visible through translucent body
    float nucleus = sdSphere(p - vec3(sin(uTime*0.5)*0.15, cos(uTime*0.4)*0.1, 0), 0.3);
    d = opSmoothUnion(d, nucleus, 0.3);

    return d;
}

vec3 sceneColor(vec3 p) {
    p = rotateY(p, uTime * 0.18);
    p = rotateX(p, sin(uTime * 0.3) * 0.15);
    float dist = length(p);

    // Nucleus — bright golden center
    float nucD = length(p - vec3(sin(uTime*0.5)*0.15, cos(uTime*0.4)*0.1, 0));
    if (nucD < 0.35) {
        float pulse = sin(uTime * 2.0) * 0.15 + 0.85;
        return vec3(0.95, 0.85, 0.3) * pulse;
    }

    // Body — translucent green with bio-luminescent pulses
    float wave = sin(p.x*5.0 + uTime*1.5) * sin(p.z*5.0 + uTime*1.8);
    float pulse = sin(dist * 4.0 - uTime * 2.5) * 0.3 + 0.7;

    float r = 0.2 + 0.15 * wave;
    float g = 0.7 + 0.2 * wave * pulse;
    float b = 0.35 + 0.15 * sin(dist * 3.0 + uTime);

    // Pseudopod tips — brighter
    if (dist > 1.2) {
        float tip = clamp((dist - 1.2) * 2.0, 0.0, 1.0);
        g += tip * 0.2;
        b += tip * 0.3;
    }

    return clamp(vec3(r, g, b), 0.0, 1.0);
}
`

// ShaderPrefixLineCount returns the number of lines in shaderPrefix,
// used to adjust error line numbers for user code.
func ShaderPrefixLineCount() int {
	return strings.Count(shaderPrefix, "\n")
}

// assembleShader combines prefix + user code + suffix into a complete fragment shader.
func assembleShader(userCode string) string {
	return shaderPrefix + userCode + "\n" + shaderSuffix + "\x00"
}
