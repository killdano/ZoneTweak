in vec3 v_pos;
in vec3 v_nrm;
in vec4 v_col;
in vec4 v_mat;
in vec3 v_tex;
uniform float u_emisGain;
uniform vec3 u_emisTint = vec3(1.0); // the hall's own lamps dim and redden when the lights go out
uniform float u_alphaMode;
out vec4 o_col;

// material kinds (v_col.a * 16)
// 0 painted metal, 1 bare steel / chrome, 2 rubber, 3 wall panel, 4 circuit board,
// 5 dark glass, 6 hazard paint, 7 cast iron / heavy machined

struct Surf { vec3 albedo; float metal; float rough; float h; float bump; vec3 emis; float ao; };

void rustLayer(inout Surf s, vec3 t, vec3 P, float wear, float seed) {
    if (wear < 0.05) return;
    float r = fbmAA(t + seed, 2.6);
    float drip = fbmAA(vec3(t.x * 6.0, t.y * 0.7, t.z * 6.0) + seed * 1.7, 1.3);
    float m = smoothstep(0.80 - wear * 0.30, 0.88 - wear * 0.30, r * 0.8 + drip * 0.25);
    float pit = nAA(t + seed, 70.0);
    vec3 rustA = vec3(0.20, 0.075, 0.03), rustB = vec3(0.36, 0.14, 0.05), rustC = vec3(0.09, 0.045, 0.025);
    vec3 rc = mix(mix(rustC, rustA, smoothstep(0.25, 0.65, pit)), rustB, smoothstep(0.75, 0.95, r) * 0.6);
    // a darker stain halo around corrosion
    float halo = smoothstep(0.70 - wear * 0.3, 0.80 - wear * 0.3, r * 0.8 + drip * 0.25) * (1.0 - m);
    s.albedo *= 1.0 - halo * 0.35;
    s.albedo = mix(s.albedo, rc, m);
    s.metal = mix(s.metal, 0.0, m);
    s.rough = mix(s.rough, 0.88 + 0.08 * pit, m);
    s.h += m * (0.4 + 0.6 * pit) * 0.5;
    s.bump = max(s.bump, m * 0.0005);
}

void grime(inout Surf s, vec3 P, vec3 t, float wear) {
    float g = fbmAA(t + 3.3, 1.4);
    float low = smoothstep(0.7, 0.0, P.y);
    float k = clamp((g - 0.42) * 1.8 * wear + low * 0.4 * wear, 0.0, 0.85);
    s.albedo *= 1.0 - k * 0.6;
    s.rough = mix(s.rough, 0.92, k * 0.5);
    s.ao *= 1.0 - k * 0.25;
}

Surf material(int kind, vec3 P, vec3 N, vec3 t, vec3 base, float metal, float rough, float wear) {
    Surf s;
    s.albedo = toLinear(base);
    s.metal = metal;
    s.rough = rough;
    s.h = 0.0;
    s.bump = 0.0;
    s.emis = vec3(0.0);
    s.ao = 1.0;
    float seed = hash13(floor(t * 0.37) + 0.5) * 50.0;
    if (kind == 0) {
        // paint over steel: edge chips, scratches, faded patches, rust bloom
        float chip = fbmAA(t + seed, 6.0);
        float scr = nAA(vec3(t.x * 0.02, t.y, t.z * 0.02) + seed, 140.0);
        float fade = fbmAA(t + seed * 0.3, 0.9);
        float ch = smoothstep(0.69 - wear * 0.25, 0.71 - wear * 0.25, chip);
        ch = max(ch, smoothstep(0.92, 0.95, scr) * wear);
        vec3 steel = vec3(0.50, 0.51, 0.53);
        float fine = nAA(t, 220.0);
        s.albedo *= (0.9 + 0.2 * fine) * (1.0 - (fade - 0.5) * 0.35 * wear);
        s.rough = rough + 0.1 * (fine - 0.5) + (fade - 0.5) * 0.2 * wear;
        s.albedo = mix(s.albedo, steel, ch);
        s.metal = mix(metal, 1.0, ch);
        s.rough = mix(s.rough, 0.3, ch);
        s.h = -ch * 0.25 + fine * 0.03;
        s.bump = 0.00025;
        rustLayer(s, t, P, wear, seed);
        grime(s, P, t, wear);
    } else if (kind == 1) {
        float br = nAA(vec3(t.x * 0.01, t.y, t.z), 600.0);
        float sm = fbmAA(t + seed, 3.0);
        s.rough = clamp(rough + (br - 0.5) * 0.08 + (sm - 0.5) * 0.12 * (0.25 + wear), 0.03, 1.0);
        s.albedo *= 0.95 + 0.08 * br;
        s.h = br * 0.05;
        s.bump = 0.0003;
        rustLayer(s, t, P, wear * 0.6, seed);
        grime(s, P, t, wear * 0.6);
    } else if (kind == 2) {
        float n = nAA(t, 120.0);
        float d = fbmAA(t, 4.0);
        s.rough = 0.68 + 0.18 * n;
        s.albedo *= 0.8 + 0.3 * n;
        s.albedo *= 1.0 - (d - 0.5) * 0.3;
        s.h = n * 0.2;
        s.bump = 0.0004;
        grime(s, P, t, wear);
    } else if (kind == 3) {
        // wall panels: stains, dark run marks, a little rust, seams, rivets
        float st = fbmAA(t + 2.0, 0.7);
        float runs = fbmAA(vec3(t.x * 9.0, t.y * 0.25, t.z * 9.0), 1.0);
        float runMask = smoothstep(0.6, 0.78, runs) * smoothstep(5.2, 1.0, P.y + st);
        s.albedo *= 0.7 + 0.55 * st;
        s.albedo *= 1.0 - runMask * 0.55;
        s.rough = clamp(rough + (st - 0.5) * 0.3 + runMask * 0.15, 0.15, 0.95);
        float fine = nAA(t, 90.0);
        s.h = fine * 0.08 + runMask * 0.15;
        s.bump = 0.0006;
        vec3 an = abs(N);
        vec2 uv = (an.x > an.y && an.x > an.z) ? t.zy : ((an.y > an.z) ? t.xz : t.xy);
        vec2 dd = abs(fract(uv * 2.0 + 0.5) - 0.5) * 0.5;
        float seam = lineAA(min(dd.x, dd.y), 0.004);
        s.h -= seam * 0.9;
        s.albedo *= 1.0 - seam * 0.6;
        s.ao *= 1.0 - seam * 0.5;
        vec2 rc = fract(uv * 2.0) * 0.5;
        vec2 q = min(rc, 0.5 - rc);
        float rv = lineAA(length(q - 0.035), 0.008);
        s.h += rv * 0.8;
        s.albedo = mix(s.albedo, s.albedo * 1.35, rv);
        rustLayer(s, t, P, wear * 0.6, seed);
        grime(s, P, t, wear);
        s.ao *= 1.0 - smoothstep(0.4, 0.0, P.y) * 0.35;
    } else if (kind == 4) {
        // circuit board
        vec2 uv = t.xy * 55.0 + t.z * 13.0;
        float fwu = g_fw * 55.0;
        vec2 cell = floor(uv), f = fract(uv);
        float hsh = hash12(cell + seed);
        vec3 pcb = vec3(0.015, 0.09, 0.04);
        vec3 copper = vec3(0.75, 0.55, 0.25);
        float tw = 0.08 + fwu * 0.5;
        float trace = 0.0;
        if (hsh < 0.45) trace = 1.0 - smoothstep(tw - 0.03, tw, abs(f.y - 0.5));
        else if (hsh < 0.8) trace = 1.0 - smoothstep(tw - 0.03, tw, abs(f.x - 0.5));
        trace *= clamp(1.5 - fwu, 0.0, 1.0);
        float pad = (1.0 - smoothstep(0.14, 0.2, length(f - 0.5))) * step(0.85, hsh) * clamp(1.5 - fwu, 0.0, 1.0);
        vec2 big = floor(uv / 6.0);
        float chipH = hash12(big + 3.0 + seed);
        vec2 bf = fract(uv / 6.0);
        float chip = (chipH > 0.5 && all(greaterThan(bf, vec2(0.12))) && all(lessThan(bf, vec2(0.88)))) ? 1.0 : 0.0;
        s.albedo = mix(pcb, copper, max(trace, pad) * 0.9);
        s.metal = max(trace, pad) * 0.9;
        s.rough = mix(0.42, 0.28, trace);
        s.albedo = mix(s.albedo, vec3(0.012), chip);
        s.rough = mix(s.rough, 0.3, chip);
        s.metal = mix(s.metal, 0.0, chip);
        s.h = chip * 0.8 + trace * 0.15;
        s.bump = 0.0003;
        float led = hash12(cell * 1.7 + 9.0 + seed);
        if (led > 0.982 && length(f - 0.5) < 0.32) {
            float blink = step(0.5, fract(u_time * (0.5 + led * 3.0) + led * 10.0));
            vec3 lc = led > 0.995 ? vec3(1.0, 0.1, 0.05) : (led > 0.99 ? vec3(0.1, 1.0, 0.2) : vec3(1.0, 0.6, 0.1));
            s.emis = lc * (3.0 + 12.0 * blink) * (1.0 - wear * 0.6);
        }
        grime(s, P, t, wear * 0.6);
    } else if (kind == 5) {
        s.albedo = vec3(0.01);
        s.metal = 0.0;
        s.rough = 0.04 + 0.1 * wear * fbmAA(t, 20.0);
    } else if (kind == 6) {
        float sw = fract((t.x + t.y + t.z) * 5.0);
        float stripe = smoothstep(0.5 - g_fw * 5.0, 0.5 + g_fw * 5.0, sw);
        s.albedo = mix(toLinear(vec3(0.78, 0.55, 0.05)), vec3(0.015), stripe);
        s.rough = 0.55;
        s.metal = 0.0;
        float chip = fbmAA(t + seed, 8.0);
        float ch = smoothstep(0.64 - wear * 0.3, 0.67 - wear * 0.3, chip);
        s.albedo = mix(s.albedo, vec3(0.4, 0.41, 0.43), ch);
        s.metal = ch;
        s.h = -ch * 0.3;
        s.bump = 0.0005;
        rustLayer(s, t, P, wear * 0.7, seed);
        grime(s, P, t, wear);
    } else if (kind == 8) {
        // polished machined metal: clean, near-mirror, only a whisper of large-scale variation
        float sm = fbmAA(t * 0.5 + seed, 0.8);
        s.rough = clamp(rough + (sm - 0.5) * 0.04, 0.03, 1.0);
        s.albedo *= 0.97 + 0.06 * sm;
        s.h = 0.0;
        s.bump = 0.0;
    } else {
        float n = fbmAA(t + seed, 20.0);
        float c = fbmAA(t + seed, 2.0);
        s.albedo *= (0.75 + 0.45 * n) * (0.85 + 0.3 * c);
        s.rough = rough + (n - 0.5) * 0.25;
        s.h = n * 0.4;
        s.bump = 0.0004;
        rustLayer(s, t, P, wear, seed);
        grime(s, P, t, wear);
    }
    return s;
}

void main() {
    if (v_pos.y < -0.01) discard; // nothing exists under the deck (things sinking into the floor, and their reflections)
    if (v_mat.w > 0.0 || dot(v_nrm, v_nrm) < 0.01) {
        vec3 c = toLinear(v_col.rgb) * max(v_mat.w, 1.0) * u_emisGain * u_emisTint;
        o_col = vec4(c, u_alphaMode > 0.5 ? v_col.a : 1.0);
        return;
    }
    g_fw = max(length(dFdx(v_tex)), length(dFdy(v_tex)));
    int kind = int(v_col.a * 16.0 + 0.5);
    vec3 N = normalize(v_nrm);
    vec3 V = normalize(u_cam - v_pos);
    if (dot(N, V) < -0.2) N = -N;
    Surf s;
    if (u_cheap > 0.5) {
        s.albedo = toLinear(v_col.rgb);
        s.metal = v_mat.x;
        s.rough = v_mat.y;
        s.h = 0.0;
        s.bump = 0.0;
    } else {
        s = material(kind, v_pos, N, v_tex, v_col.rgb, v_mat.x, v_mat.y, v_mat.z);
    }
    float bfade = clamp(1.5 - g_fw * 400.0, 0.0, 1.0);
    if (s.bump > 0.0 && bfade > 0.0) N = bumpNormal(v_pos, N, s.h, s.bump * 600.0 * bfade);
    float rough = clamp(s.rough, 0.03, 1.0);
    // specular anti-aliasing: widen roughness where the normal varies within a pixel
    vec3 dn = fwidth(N);
    rough = clamp(sqrt(rough * rough + min(dot(dn, dn) * 2.0, 0.25)), 0.03, 1.0);
    vec3 c = shadeAll(v_pos, N, V, s.albedo, s.metal, rough);
    c += ambient(N, V, s.albedo, s.metal, rough, s.ao);
    c += s.emis;
    o_col = vec4(sane(c), 1.0);
}
