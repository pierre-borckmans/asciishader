// ---- Scene: Mercury ----
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
