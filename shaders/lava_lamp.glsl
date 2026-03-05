// ---- Scene: Lava Lamp ----
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
