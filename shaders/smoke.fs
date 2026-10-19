in vec4 v_col;
in vec2 v_uv;
in vec3 v_pos;
uniform float u_gain;
out vec4 o_col;

// smoke / steam / mist billboards: wispy domain-warped density, lit by the scene lights
void main() {
    vec2 d = v_uv * 2.0 - 1.0;
    float r2 = dot(d, d);
    if (r2 >= 1.0) discard;
    float body = exp(-2.2 * r2) * (1.0 - r2);
    vec3 q = v_pos * 2.2 + vec3(0.0, -u_time * 0.18, u_time * 0.05);
    vec3 w = vec3(vnoise(q * 0.55), vnoise(q * 0.55 + 7.3), vnoise(q * 0.55 + 13.1)) - 0.5;
    float n = vnoise(q + w * 2.8) * 0.62 + vnoise(q * 2.03 + w * 1.7) * 0.38;
    float n2 = vnoise(q * 3.3 - w * 3.5);
    float wisp = smoothstep(0.3, 1.0, n * 0.95 + n2 * 0.25);
    bool co2 = v_col.b > 0.995 && v_col.r > 0.94 && v_col.r < 0.96;
    if (co2) {
        // CO2: a dense cold core with spiral arms curling round it
        float rr = sqrt(r2);
        float sw = 3.2 * (1.0 - rr) + u_time * 1.4;
        vec2 dr = vec2(d.x * cos(sw) - d.y * sin(sw), d.x * sin(sw) + d.y * cos(sw));
        vec3 qs = vec3(dr * 1.7, 0.0) + v_pos * 1.1;
        float sp = vnoise(qs) * 0.65 + vnoise(qs * 2.3 + w) * 0.35;
        float arms = smoothstep(0.32, 0.85, sp);
        body = exp(-1.7 * r2) * (1.0 - r2);
        wisp = mix(arms, 1.0, 0.45 * (1.0 - rr));
    }
    float a = body * wisp * v_col.a;
    float dc = length(v_pos - u_cam);
    a *= smoothstep(0.3, 1.3, dc);
    if (a < 0.002) discard;

    vec3 L = vec3(0.07, 0.08, 0.082);
    int nl = min(u_nL, 10);
    for (int j = 0; j < nl; j++) {
        vec3 col; float rad, size;
        vec3 lp = lightPoint(j, v_pos, col, rad, size);
        vec3 lv = lp - v_pos;
        float d2 = dot(lv, lv);
        if (d2 > rad * rad) continue;
        float fall = 1.0 - d2 / (rad * rad);
        float k = (int(u_L[j * 3].w + 0.5) == 2) ? 0.35 : 1.0;
        L += min(col * fall * fall / (d2 + 0.3), vec3(5.0)) * 0.5 * k;
    }
    for (int s = 0; s < u_nS; s++) {
        vec3 lv = u_Spos[s].xyz - v_pos;
        float d2 = dot(lv, lv);
        vec3 l = lv * inversesqrt(d2);
        float cone = smoothstep(u_Spos[s].w, u_Sdir[s].w, dot(-l, u_Sdir[s].xyz));
        float rg = u_Scol[s].w;
        float fall = clamp(1.0 - d2 / (rg * rg), 0.0, 1.0);
        L += u_Scol[s].rgb * cone * fall * fall / (d2 + 0.3) * 0.5;
    }
    if (co2) L += vec3(0.42, 0.45, 0.5); // dense cold vapour: bright, catching every light in the room
    vec3 alb = pow(max(v_col.rgb, 0.0), vec3(2.2));
    // denser cores self-shadow a little
    // premultiplied: pale vapour mostly scatters light and barely blocks it, dark smoke occludes
    float lum = dot(alb, vec3(0.3, 0.5, 0.2));
    float ext = mix(1.0, 0.15, smoothstep(0.05, 0.5, lum));
    o_col = vec4(sane(alb * L * u_gain) * a, a * ext);
}
