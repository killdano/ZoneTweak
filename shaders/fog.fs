in vec2 v_uv;
uniform sampler2D u_depth;
uniform mat4 u_ivp;
uniform float u_density;
uniform int u_steps;
uniform float u_maxDist;
uniform int u_fogLights;
uniform float u_frame;
uniform vec4 u_FV[24];   // xyz centre, w radius
uniform vec4 u_FVp[24];  // x strength, y vertical stretch
uniform int u_nFV;
out vec4 o_col;

vec3 worldAt(vec2 uv, float d) {
    vec4 p = u_ivp * vec4(uv * 2.0 - 1.0, d * 2.0 - 1.0, 1.0);
    return p.xyz / p.w;
}
float fogDensity(vec3 p) {
    float h = exp(-max(p.y, 0.0) * 0.6);
    vec3 w = vec3(u_time * 0.06, u_time * 0.015, -u_time * 0.045);
    float n = fbm3(p * 0.45 + w);
    float n2 = vnoise(p * 1.7 - w * 2.0);
    float d = u_density * (0.25 + 1.4 * h) * (0.25 + 1.5 * n * n) * (0.8 + 0.4 * n2);
    float vol = 0.0;
    for (int i = 0; i < u_nFV; i++) {
        vec3 q = p - u_FV[i].xyz;
        q.y /= u_FVp[i].y;
        float r = length(q) / u_FV[i].w;
        if (r < 1.0) {
            float f = 1.0 - r;
            vol += u_FVp[i].x * f * f * (3.0 - 2.0 * f);
        }
    }
    if (vol > 0.0) {
        // curling, uneven vapour: domain-warped noise drifting upward
        vec3 q = p * 1.5 + vec3(u_time * 0.03, -u_time * 0.09, u_time * 0.02);
        vec3 wq = vec3(vnoise(q * 0.7), vnoise(q * 0.7 + 5.2), vnoise(q * 0.7 + 9.7)) - 0.5;
        float sw = fbm3(q * 1.3 + wq * 2.2);
        float fine = vnoise(p * 6.0 + wq * 3.0 + vec3(0.0, -u_time * 0.25, 0.0));
        float dens = smoothstep(0.35, 0.8, sw * 0.85 + fine * 0.25);
        d += vol * 2.6 * dens;
    }
    return d;
}
float phaseHG(float c, float g) {
    float g2 = g * g;
    return (1.0 - g2) / (4.0 * PI * pow(1.0 + g2 - 2.0 * g * c, 1.5));
}
void main() {
    vec2 ts = 1.0 / vec2(textureSize(u_depth, 0));
    float d = texture(u_depth, v_uv).r;
    d = max(d, texture(u_depth, v_uv + vec2(ts.x, 0)).r);
    d = max(d, texture(u_depth, v_uv + vec2(0, ts.y)).r);
    if (d <= 0.0) { o_col = vec4(0.0, 0.0, 0.0, 1.0); return; } // behind the lens edge: never seen
    vec3 P = worldAt(v_uv, d);
    vec3 ro = u_cam;
    vec3 rd = P - ro;
    float L = min(length(rd), u_maxDist);
    rd = normalize(rd);
    float jit = fract(52.9829189 * fract(dot(gl_FragCoord.xy, vec2(0.06711056, 0.00583715))) + u_frame * 0.61803);
    float dt = L / float(u_steps);
    float T = 1.0;
    vec3 S = vec3(0.0);
    int nl = min(u_nL, u_fogLights);
    for (int i = 0; i < u_steps; i++) {
        float t = (float(i) + jit) * dt;
        vec3 x = ro + rd * t;
        float dens = fogDensity(x);
        vec3 li = vec3(0.006, 0.009, 0.011);
        for (int j = 0; j < nl; j++) {
            vec3 col; float rad, size;
            vec3 q = lightPoint(j, x, col, rad, size);
            vec3 lv = q - x;
            float d2 = dot(lv, lv);
            if (d2 > rad * rad) continue;
            float w = 1.0 - d2 / (rad * rad);
            float ph = phaseHG(dot(rd, lv * inversesqrt(d2 + 1e-6)), 0.25);
            float k = (int(u_L[j * 3].w + 0.5) == 2) ? 0.35 : 1.0;
            li += min(col * w * w / (d2 + 0.25), vec3(6.0)) * ph * 4.0 * PI * 0.25 * k;
        }
        for (int s = 0; s < u_nS; s++) {
            vec3 lv = u_Spos[s].xyz - x;
            float d2 = dot(lv, lv);
            vec3 l = lv * inversesqrt(d2);
            float cone = smoothstep(u_Spos[s].w, u_Sdir[s].w, dot(-l, u_Sdir[s].xyz));
            if (cone <= 0.0) continue;
            float rg = u_Scol[s].w;
            float w = clamp(1.0 - d2 / (rg * rg), 0.0, 1.0);
            li += u_Scol[s].rgb * cone * w * w / (d2 + 0.3) * spotShadowFast(s, x) * phaseHG(dot(rd, l), 0.45) * 4.0 * PI * 0.35;
        }
        float ext = dens * dt;
        S += T * li * dens * dt;
        T *= exp(-ext);
    }
    o_col = vec4(sane(S), (isnan(T) ? 1.0 : clamp(T, 0.0, 1.0)));
}
