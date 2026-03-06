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
