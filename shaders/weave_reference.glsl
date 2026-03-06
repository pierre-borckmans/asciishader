// Scene: Weave Reference
// Direct GLSL translation of fogleman/sdf/examples/weave.py
// Fogleman Z-up → Our Y-up: swap Y↔Z

float sdRoundBox(vec3 p, vec3 b, float r) {
    vec3 q = abs(p) - b;
    return length(max(q, 0.0)) + min(max(q.x, max(q.y, q.z)), 0.0) - r;
}

float sdInfiniteCylinder(vec3 p, float r) {
    return length(p.xz) - r;
}

float sdSlab(vec3 p, float h) {
    return abs(p.y) - h;
}

float opSmoothIntersect(float d1, float d2, float k) {
    float h = clamp(0.5 - 0.5*(d2-d1)/k, 0.0, 1.0);
    return mix(d2, d1, h) + k*h*(1.0-h);
}

float easeInOutQuad(float t) {
    return t < 0.5 ? 2.0*t*t : -1.0 + (4.0 - 2.0*t)*t;
}

// Single ribbon: box + translate + bend_linear
float ribbon(vec3 p) {
    // bend_linear(p0, p1, v, ease):
    //   v = -v (negated internally!)
    //   t = clamp(dot(p-p0, p1-p0) / dot(p1-p0, p1-p0), 0, 1)
    //   t = ease(t)
    //   return other(p + t * v)
    //
    // Fogleman call: bend_linear(X*0.75, X*2.25, Z*-0.1875, ease.in_out_quad)
    // p0 = (0.75, 0, 0), p1 = (2.25, 0, 0)
    // v = -(-0.1875 * Z) = (0, 0, 0.1875) in Z-up
    // Our Y-up: v = (0, 0.1875, 0)

    vec3 ab = vec3(1.5, 0, 0); // p1 - p0
    float t = clamp(dot(p - vec3(0.75, 0, 0), ab) / dot(ab, ab), 0.0, 1.0);
    t = easeInOutQuad(t);

    // Displace p by t * v, then evaluate translated box
    vec3 dp = p + t * vec3(0, 0.1875, 0);

    // translate((1.5, 0, 0.0625)) → our: (1.5, 0.0625, 0)
    vec3 bp = dp - vec3(1.5, 0.0625, 0);

    // rounded_box([3.2, 1, 0.25], 0.1) → our half-extents: (1.6, 0.125, 0.5)
    return sdRoundBox(bp, vec3(1.6, 0.125, 0.5), 0.1);
}

// circular_array(3, 0) — 3 copies around Y axis (our up)
// Fogleman evaluates TWO adjacent sectors and takes min
float ribbonArray(vec3 p) {
    float da = 6.28318530718 / 3.0;
    float d = length(p.xz);
    float a = mod(atan(p.z, p.x), da);

    // Evaluate at current sector angle and adjacent sector
    vec3 p1 = vec3(cos(a) * d, p.y, sin(a) * d);
    vec3 p2 = vec3(cos(a - da) * d, p.y, sin(a - da) * d);

    return min(ribbon(p1), ribbon(p2));
}

// Repeated grid with padding — evaluate neighboring cells
float ribbonRepeat(vec3 p, vec2 spacing) {
    // Find cell index
    vec2 cell = floor((p.xz + spacing * 0.5) / spacing);
    float d = 1e10;
    // Check current cell and all 8 neighbors (padding=1)
    for (int dx = -1; dx <= 1; dx++) {
        for (int dz = -1; dz <= 1; dz++) {
            vec2 c = cell + vec2(float(dx), float(dz));
            vec3 rp = p - vec3(c.x * spacing.x, 0, c.y * spacing.y);
            d = min(d, ribbonArray(rp));
        }
    }
    return d;
}

float ribbonGrid(vec3 p) {
    vec2 spacing = vec2(2.7, 5.4);
    float d = ribbonRepeat(p, spacing);

    // f |= f.translate((1.35, 2.7, 0)) → our: (1.35, 0, 2.7)
    d = min(d, ribbonRepeat(p - vec3(1.35, 0, 2.7), spacing));

    return d;
}

float sceneSDF(vec3 p) {
    float grid = ribbonGrid(p);
    float cyl = sdInfiniteCylinder(p, 10.0);
    float f = max(grid, cyl);

    float ring = max(sdInfiniteCylinder(p, 12.0), -sdInfiniteCylinder(p, 10.0));
    float slab = sdSlab(p, 0.5);
    float frame = opSmoothIntersect(ring, slab, 0.25);

    return min(f, frame);
}

vec3 sceneColor(vec3 p) {
    return vec3(0.4, 0.6, 0.9);
}
