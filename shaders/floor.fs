in vec3 v_pos;
in vec3 v_nrm;
in vec4 v_col;
in vec4 v_mat;
in vec3 v_tex;
uniform vec3 u_fo;
uniform vec3 u_fx;
uniform vec3 u_fz;
uniform float u_reflScale;
// the reflection, rendered at reduced resolution into its own texture (when u_reflTex is 1)
uniform sampler2D u_refl;
uniform float u_reflTex;
uniform vec2 u_screen;
out vec4 o_col;

// floor: riveted steel deck plates, drain grates, oil, standing water that mirrors the cell
void main() {
    vec3 P = v_pos;
    vec3 rel = P - u_fo;
    vec2 uv = vec2(dot(rel, u_fx), dot(rel, u_fz));
    g_fw = max(length(dFdx(uv)), length(dFdy(uv)));
    vec2 cell = floor(uv);
    vec2 f = fract(uv);
    float ch = hash12(cell);
    vec3 N = vec3(0.0, 1.0, 0.0);
    vec3 V = normalize(u_cam - P);

    // plate base
    vec3 albedo = toLinear(vec3(0.20, 0.21, 0.22)) * (0.75 + 0.5 * ch);
    float metal = 0.75;
    float rough = 0.55 + 0.2 * ch;
    float h = 0.0;
    float ao = 1.0;
    if (u_cheap > 0.5) {
        // POTATO: plain plates and seams - no tread, rust, water, ripples or oil
        float edgeC = min(min(f.x, 1.0 - f.x), min(f.y, 1.0 - f.y));
        albedo *= 1.0 - (1.0 - smoothstep(0.0, 0.01, edgeC)) * 0.6;
        vec3 cc = shadeAll(P, N, V, albedo, metal, rough) + ambient(N, V, albedo, metal, rough, 1.0);
        o_col = vec4(sane(cc), 1.0);
        return;
    }

    // raised lozenge tread on some plates (classic checker plate)
    if (ch > 0.55) {
        vec2 q = uv * 14.0;
        vec2 id = floor(q);
        vec2 g = fract(q) - 0.5;
        float flip = mod(id.x + id.y, 2.0);
        vec2 r = flip > 0.5 ? vec2(g.x * 0.7 + g.y * 0.7, -g.x * 0.7 + g.y * 0.7) : vec2(g.x * 0.7 - g.y * 0.7, g.x * 0.7 + g.y * 0.7);
        float lz = length(vec2(r.x * 3.2, r.y));
        float bump = (1.0 - smoothstep(0.26, 0.32, lz)) * clamp(1.6 - g_fw * 14.0 * 2.2, 0.0, 1.0);
        h += bump * 0.6;
        rough -= bump * 0.12;
    }
    // scratches and wear
    float scr = nAA(vec3(uv.x * 0.014 + ch * 0.06, uv.y, ch * 0.02), 140.0);
    float scr2 = nAA(vec3(uv.x, uv.y * 0.023 + ch * 0.05, ch * 0.01), 130.0);
    float sc = max(smoothstep(0.9, 0.97, scr), smoothstep(0.92, 0.98, scr2));
    rough = mix(rough, 0.25, sc);
    albedo = mix(albedo, toLinear(vec3(0.45)), sc * 0.5);
    float grime = fbm2AA(uv + 4.0, 0.9);
    albedo *= 0.55 + 0.7 * grime;
    rough = clamp(rough + (0.5 - grime) * 0.25, 0.08, 0.95);

    // rust blooms toward plate edges
    float edge = min(min(f.x, 1.0 - f.x), min(f.y, 1.0 - f.y));
    float rn = fbm2AA(uv + ch * 3.0, 3.0);
    float rust = smoothstep(0.78, 0.9, rn + smoothstep(0.12, 0.0, edge) * 0.25) * 0.8;
    albedo = mix(albedo, toLinear(vec3(0.24, 0.10, 0.045)) * (0.7 + 0.6 * rn), rust);
    metal = mix(metal, 0.0, rust);
    rough = mix(rough, 0.9, rust);
    h += rust * 0.3 * rn;

    // seams + rivets
    float seam = lineAA(edge, 0.006);
    h -= seam;
    ao *= 1.0 - seam * 0.7;
    vec2 q = min(f, 1.0 - f);
    float rv = 0.0;
    for (int k = 0; k < 4; k++) {
        float along = 0.125 + 0.25 * float(k);
        rv = max(rv, lineAA(length(vec2(q.x - 0.04, abs(f.y - along))), 0.011));
        rv = max(rv, lineAA(length(vec2(abs(f.x - along), q.y - 0.04)), 0.011));
    }
    h += rv * 0.9;
    rough = mix(rough, 0.35, rv);

    // drain grates
    bool grate = ch < 0.08;
    float hole = 0.0;
    if (grate) {
        vec2 g = fract(uv * vec2(1.0, 14.0));
        float inner = step(0.08, f.x) * step(f.x, 0.92) * step(0.08, f.y) * step(f.y, 0.92);
        hole = inner * step(0.3, g.y) * step(g.y, 0.75);
        albedo = mix(albedo, vec3(0.002), hole);
        rough = mix(rough, 1.0, hole);
        ao *= 1.0 - hole * 0.95;
        h -= hole;
    }

    // standing water
    float wn = fbm2AA(uv + 21.0, 0.35) + 0.15 * fbm2AA(uv, 2.0);
    float wet = smoothstep(0.62, 0.68, wn) * (grate ? 0.0 : 1.0);
    float damp = smoothstep(0.52, 0.66, wn);
    albedo *= 1.0 - damp * 0.45;
    rough = mix(rough, 0.04, wet);
    rough = mix(rough, rough * 0.6, damp * (1.0 - wet));
    metal = mix(metal, 0.0, wet);

    // drips: expanding ripple rings in puddles
    float rip = 0.0;
    if (wet > 0.0) {
        vec2 dc = floor(uv * 1.3);
        for (int k = 0; k < 4; k++) {
            vec2 o = vec2(float(k & 1), float(k >> 1));
            vec2 c = dc + o;
            float hs = hash12(c * 3.1);
            float period = 2.0 + 3.0 * hs;
            float ph = fract(u_time / period + hs);
            vec2 center = (c + hash22(c + floor(u_time / period + hs))) / 1.3;
            float d = length(uv - center);
            float r = ph * 0.35;
            rip += sin((d - r) * 120.0) * exp(-abs(d - r) * 60.0) * (1.0 - ph);
        }
        h += rip * 0.25 * wet;
    }
    // oil sheen
    float oil = smoothstep(0.75, 0.8, fbm2AA(uv + 77.0, 0.6)) * (1.0 - wet);
    albedo = mix(albedo, vec3(0.004), oil * 0.8);
    rough = mix(rough, 0.12, oil);

    N = bumpNormal(P, N, h, mix(0.0007, 0.00015, wet) * 600.0 * clamp(1.5 - g_fw * 150.0, 0.0, 1.0));
    vec3 dn = fwidth(N);
    rough = clamp(sqrt(rough * rough + min(dot(dn, dn) * 2.0, 0.25)), 0.03, 1.0);

    vec3 c = shadeAll(P, N, V, albedo, metal, rough);
    c += ambient(N, V, albedo, metal, rough, ao);
    if (oil > 0.0) {
        float th = dot(N, V) * 6.0 + fbm2(uv * 8.0) * 4.0;
        c += 0.02 * oil * (0.5 + 0.5 * cos(th + vec3(0.0, 2.1, 4.2)));
    }

    // planar reflection already rendered below: blend by fresnel and smoothness
    float NdV = max(dot(N, V), 0.0);
    float fres = 0.04 + 0.96 * pow(1.0 - NdV, 5.0);
    float refl = mix(0.10, 0.92, wet) * mix(1.0, 0.25, smoothstep(0.1, 0.6, rough));
    refl = clamp(mix(refl, 1.0, fres * (1.0 - rough)), 0.0, 0.95) * (1.0 - hole) * u_reflScale;
    float keep = clamp(1.0 - refl, 0.0, 1.0);
    if (u_reflTex > 0.5) {
        // ripples in the standing water bend the reflection a little
        vec2 suv = gl_FragCoord.xy / u_screen + N.xz * 0.006 * wet;
        // four bilinear taps: smooths the low-res, un-antialiased reflection
        vec2 rt = 0.75 / vec2(textureSize(u_refl, 0));
        vec3 rc = 0.25 * (texture(u_refl, suv + vec2(rt.x, rt.y)).rgb + texture(u_refl, suv + vec2(-rt.x, rt.y)).rgb +
                          texture(u_refl, suv + vec2(rt.x, -rt.y)).rgb + texture(u_refl, suv - rt).rgb);
        o_col = vec4(sane(mix(rc, c, keep)), 1.0);
        return;
    }
    o_col = vec4(sane(c), keep);
}
