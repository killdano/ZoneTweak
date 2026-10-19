// ---------- shared: noise, lights, BRDF ----------
uniform float u_time;
uniform float u_cheap; // POTATO quality: skip procedural surface detail
uniform vec3 u_cam;

#define MAXL 48
uniform vec4 u_L[MAXL * 3];
uniform int u_nL;

// two shadowed ceiling spot lights
uniform mat4 u_SM[2];
uniform vec4 u_Spos[2];   // xyz pos, w cos(outer)
uniform vec4 u_Sdir[2];   // xyz dir, w cos(inner)
uniform vec4 u_Scol[2];   // rgb intensity, w range
uniform sampler2DShadow u_shadow0;
uniform sampler2DShadow u_shadow1;
uniform int u_nS;
uniform float u_shadowTexel;

const float PI = 3.14159265;

float hash13(vec3 p) {
    p = fract(p * 0.1031);
    p += dot(p, p.zyx + 31.32);
    return fract((p.x + p.y) * p.z);
}
float hash12(vec2 p) {
    vec3 p3 = fract(vec3(p.xyx) * 0.1031);
    p3 += dot(p3, p3.yzx + 33.33);
    return fract((p3.x + p3.y) * p3.z);
}
vec2 hash22(vec2 p) {
    vec3 p3 = fract(vec3(p.xyx) * vec3(0.1031, 0.1030, 0.0973));
    p3 += dot(p3, p3.yzx + 33.33);
    return fract((p3.xx + p3.yz) * p3.zy);
}
float vnoise(vec3 p) {
    vec3 i = floor(p), f = fract(p);
    vec3 u = f * f * (3.0 - 2.0 * f);
    float a = hash13(i), b = hash13(i + vec3(1, 0, 0));
    float c = hash13(i + vec3(0, 1, 0)), d = hash13(i + vec3(1, 1, 0));
    float e = hash13(i + vec3(0, 0, 1)), f1 = hash13(i + vec3(1, 0, 1));
    float g = hash13(i + vec3(0, 1, 1)), h = hash13(i + vec3(1, 1, 1));
    return mix(mix(mix(a, b, u.x), mix(c, d, u.x), u.y), mix(mix(e, f1, u.x), mix(g, h, u.x), u.y), u.z);
}
float vnoise2(vec2 p) {
    vec2 i = floor(p), f = fract(p);
    vec2 u = f * f * (3.0 - 2.0 * f);
    return mix(mix(hash12(i), hash12(i + vec2(1, 0)), u.x), mix(hash12(i + vec2(0, 1)), hash12(i + vec2(1, 1)), u.x), u.y);
}
float fbm3(vec3 p) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < 4; i++) { s += a * vnoise(p); p = p * 2.03 + 17.1; a *= 0.5; }
    return s / 0.9375;
}
float fbm5(vec3 p) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < 5; i++) { s += a * vnoise(p); p = p * 2.01 + 7.3; a *= 0.5; }
    return s / 0.96875;
}
float fbm2(vec2 p) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < 4; i++) { s += a * vnoise2(p); p = p * 2.03 + 17.1; a *= 0.5; }
    return s / 0.9375;
}

// pixel footprint of the texture space, set per fragment; detail fades out before it can alias
float g_fw = 0.001;
float nAA(vec3 p, float freq) {
    float k = clamp(2.0 - freq * g_fw * 4.0, 0.0, 1.0);
    return mix(0.5, vnoise(p * freq), k);
}
float fbmAA(vec3 p, float freq) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < 5; i++) {
        float k = clamp(2.0 - freq * g_fw * 4.0, 0.0, 1.0);
        s += a * mix(0.5, vnoise(p * freq + float(i) * 7.3), k);
        freq *= 2.03;
        a *= 0.5;
    }
    return s / 0.96875;
}
float fbm2AA(vec2 p, float freq) {
    float s = 0.0, a = 0.5;
    for (int i = 0; i < 4; i++) {
        float k = clamp(2.0 - freq * g_fw * 4.0, 0.0, 1.0);
        s += a * mix(0.5, vnoise2(p * freq + float(i) * 17.1), k);
        freq *= 2.03;
        a *= 0.5;
    }
    return s / 0.9375;
}
float lineAA(float d, float w) {
    float f = max(g_fw, 1e-5);
    return 1.0 - smoothstep(w - f, w + f, d);
}

vec3 toLinear(vec3 c) { return pow(max(c, 0.0), vec3(2.2)); }

float D_GGX(float NdH, float a) {
    float a2 = a * a;
    float d = NdH * NdH * (a2 - 1.0) + 1.0;
    return a2 / (PI * d * d + 1e-7);
}
float V_SmithJoint(float NdV, float NdL, float a) {
    float k = a * 0.5;
    return 0.25 / ((NdL * (1.0 - k) + k) * (NdV * (1.0 - k) + k) + 1e-5);
}
vec3 F_Schlick(vec3 F0, float VdH) {
    return F0 + (1.0 - F0) * pow(1.0 - VdH, 5.0);
}

// closest point on the light primitive
vec3 lightPoint(int i, vec3 P, out vec3 col, out float rad, out float size) {
    vec4 A = u_L[i * 3], B = u_L[i * 3 + 1], C = u_L[i * 3 + 2];
    int type = int(A.w + 0.5);
    col = C.rgb;
    rad = B.w;
    size = 0.0;
    vec3 q = A.xyz;
    if (type >= 1) {
        vec3 ab = B.xyz - A.xyz;
        float t = clamp(dot(P - A.xyz, ab) / max(dot(ab, ab), 1e-6), 0.0, 1.0);
        q = A.xyz + ab * t;
        size = length(ab) * 0.5;
        if (type == 2) {
            q.y = clamp(P.y, A.y, A.y + C.w);
            size = max(size, C.w * 0.5);
        }
    }
    return q;
}

float spotShadow(int s, vec3 P, vec3 N) {
    vec4 lp = u_SM[s] * vec4(P + N * 0.02, 1.0);
    vec3 c = lp.xyz / lp.w * 0.5 + 0.5;
    if (c.x < 0.0 || c.x > 1.0 || c.y < 0.0 || c.y > 1.0 || c.z > 1.0) return 1.0;
    float sum = 0.0;
    float r = u_shadowTexel * 1.5;
    for (int y = -1; y <= 1; y++)
        for (int x = -1; x <= 1; x++) {
            vec3 cc = vec3(c.xy + vec2(x, y) * r, c.z - 0.0008);
            sum += (s == 0) ? texture(u_shadow0, cc) : texture(u_shadow1, cc);
        }
    return sum / 9.0;
}

float spotShadowFast(int s, vec3 P) {
    vec4 lp = u_SM[s] * vec4(P, 1.0);
    vec3 c = lp.xyz / lp.w * 0.5 + 0.5;
    if (c.x < 0.0 || c.x > 1.0 || c.y < 0.0 || c.y > 1.0 || c.z > 1.0) return 1.0;
    c.z -= 0.001;
    return (s == 0) ? texture(u_shadow0, c) : texture(u_shadow1, c);
}

vec3 brdfLight(vec3 N, vec3 V, vec3 L, vec3 diff, vec3 F0, float a, float extra) {
    float NdL = max(dot(N, L), 0.0);
    if (NdL <= 0.0) return vec3(0.0);
    vec3 H = normalize(L + V);
    float NdV = max(dot(N, V), 1e-3);
    float NdH = max(dot(N, H), 0.0);
    float VdH = max(dot(V, H), 0.0);
    float aa = clamp(a + extra, 0.002, 1.0);
    vec3 F = F_Schlick(F0, VdH);
    vec3 spec = D_GGX(NdH, aa) * V_SmithJoint(NdV, NdL, aa) * F;
    // normalize widened lobe energy for area lights
    spec *= (a * a) / (aa * aa) * 0.5 + 0.5;
    return (diff * (1.0 - F) / PI + spec) * NdL;
}

vec3 shadeAll(vec3 P, vec3 N, vec3 V, vec3 albedo, float metal, float rough) {
    vec3 F0 = mix(vec3(0.04), albedo, metal);
    vec3 diff = albedo * (1.0 - metal);
    float a = rough * rough;
    vec3 acc = vec3(0.0);
    for (int i = 0; i < u_nL; i++) {
        vec3 col; float rad, size;
        vec3 q = lightPoint(i, P, col, rad, size);
        vec3 Lv = q - P;
        float d2 = dot(Lv, Lv);
        if (d2 > rad * rad) continue;
        float d = sqrt(d2);
        vec3 L = Lv / max(d, 1e-4);
        float w = 1.0 - d2 / (rad * rad);
        float att = w * w / (d2 + 0.04);
        float extra = size / (d * 2.0 + size + 0.001) * 0.6;
        acc += col * att * brdfLight(N, V, L, diff, F0, a, extra);
    }
    for (int s = 0; s < u_nS; s++) {
        vec3 Lv = u_Spos[s].xyz - P;
        float d2 = dot(Lv, Lv);
        float d = sqrt(d2);
        vec3 L = Lv / d;
        float cd = dot(-L, u_Sdir[s].xyz);
        float cone = smoothstep(u_Spos[s].w, u_Sdir[s].w, cd);
        if (cone <= 0.0) continue;
        float rg = u_Scol[s].w;
        float w = clamp(1.0 - d2 / (rg * rg), 0.0, 1.0);
        float sh = spotShadow(s, P, N);
        acc += u_Scol[s].rgb * cone * w * w / (d2 + 0.1) * sh * brdfLight(N, V, L, diff, F0, a, 0.02);
    }
    return acc;
}

// fake environment reflection of the cell block: dark, light-bar bands, green laser glow low
vec3 envRefl(vec3 R, float rough) {
    float y = R.y;
    vec3 c = vec3(0.004, 0.006, 0.008);
    float band = smoothstep(0.08, 0.0, abs(y - 0.12)) * (0.6 + 0.4 * step(0.5, fract(atan(R.z, R.x) * 2.5)));
    c += vec3(0.5, 0.6, 0.7) * band * (1.0 - rough) * 0.6;
    c += vec3(0.05, 0.35, 0.12) * smoothstep(0.3, -0.25, y) * 0.25;
    c += vec3(0.6, 0.7, 0.8) * smoothstep(0.93, 1.0, y) * 0.25 * (1.0 - rough);
    c = mix(c, vec3(0.012, 0.03, 0.02), rough * 0.6);
    return c;
}

vec3 ambient(vec3 N, vec3 V, vec3 albedo, float metal, float rough, float ao) {
    vec3 F0 = mix(vec3(0.04), albedo, metal);
    float NdV = max(dot(N, V), 0.0);
    vec3 F = F0 + (max(vec3(1.0 - rough), F0) - F0) * pow(1.0 - NdV, 5.0);
    vec3 hemi = mix(vec3(0.016, 0.02, 0.018), vec3(0.04, 0.05, 0.058), N.y * 0.5 + 0.5);
    vec3 R = reflect(-V, N);
    return (albedo * (1.0 - metal) * hemi + envRefl(R, rough) * F) * ao;
}

vec3 bumpNormal(vec3 P, vec3 N, float h, float scale) {
    vec3 dpdx = dFdx(P), dpdy = dFdy(P);
    float dhx = dFdx(h), dhy = dFdy(h);
    vec3 r1 = cross(dpdy, N), r2 = cross(N, dpdx);
    float det = dot(dpdx, r1);
    if (abs(det) < 1e-12) return N;
    vec3 grad = sign(det) * (dhx * r1 + dhy * r2);
    vec3 nn = abs(det) * N - grad * scale;
    float l2 = dot(nn, nn);
    if (!(l2 > 1e-20)) return N;
    nn *= inversesqrt(l2);
    return dot(nn, N) > 0.05 ? nn : N;
}

vec3 sane(vec3 c) {
    bvec3 bad = isnan(c);
    if (bad.x || bad.y || bad.z || any(isinf(c))) return vec3(0.0);
    return min(c, vec3(4000.0));
}
