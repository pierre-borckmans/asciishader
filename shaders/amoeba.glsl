// ---- Scene: Amoeba ----
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
