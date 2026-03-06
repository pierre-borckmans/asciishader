// ---- Scene: Railway Express ----
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
