in vec4 v_col;
in vec2 v_uv;
in vec3 v_pos;
out vec4 o_col;

// procedural flame on a vertical billboard: turbulent, domain-warped noise tears a wavering
// teardrop apart; gusts change its reach and width from moment to moment, tongues break off
// and burn out above it. Colour runs white-yellow at the root, orange body, dark red tips.
void main() {
    float seed = v_col.r * 37.0;
    float inten = v_col.a;
    float x = (v_uv.x - 0.5) * 2.0;
    float y = v_uv.y;
    float t = u_time;
    // gusts: slow and fast pulsing of the whole flame's reach and width
    float gust = vnoise(vec3(t * 0.9, seed, 1.0)) * 0.6 + vnoise(vec3(t * 3.7, seed, 5.0)) * 0.4;
    float reach = 0.55 + 0.6 * gust;
    float yy = y / reach;
    vec3 q = vec3(x * 1.8, y * 2.6 - t * 3.1, seed);
    vec2 w = vec2(vnoise(q * 1.4), vnoise(q * 1.4 + 4.7)) - 0.5;
    float n = vnoise(vec3(x * 3.0 + w.x * 2.2, y * 4.0 - t * 4.2 + w.y * 2.2, seed * 1.3)) * 0.6
            + vnoise(vec3(x * 7.0 + w.y, y * 9.0 - t * 7.5, seed + 9.0)) * 0.4;
    float sway = (vnoise(vec3(y * 1.8 - t * 2.4, seed, 2.0)) - 0.5) * 0.8 * y
               + (vnoise(vec3(t * 1.3, seed, 7.0)) - 0.5) * 0.35 * y;
    float yc = clamp(yy, 0.0, 1.0);
    float widCamp = mix(0.8, 0.1, pow(yc, 0.7));
    // electrical: pinched at the source, ballooning through the middle, thin at the top
    float widElec = 0.12 + 0.85 * smoothstep(0.0, 0.38, yc) * (1.0 - smoothstep(0.4, 1.0, yc) * 0.9);
    float wid = mix(widCamp, widElec, step(0.5, v_col.g)) * (0.8 + 0.4 * gust);
    float env = 1.0 - smoothstep(wid * 0.45, wid, abs(x - sway));
    env *= smoothstep(0.0, 0.06, y) * (1.0 - smoothstep(0.5, 1.0, yy));
    // detached tongues licking up past the main body
    float lick = smoothstep(0.62, 0.9, vnoise(vec3(x * 4.0, y * 3.0 - t * 5.5, seed + 3.0))) * (1.0 - smoothstep(0.6, 1.0, y)) * smoothstep(0.3, 0.6, y);
    float f = clamp(env * 1.6 - n * 1.05 - yy * 0.4, 0.0, 1.0);
    f = max(f, lick * (1.0 - abs(x) * 1.2) * 0.6);
    if (f <= 0.0) discard;
    vec3 c = mix(vec3(0.85, 0.1, 0.02), vec3(1.0, 0.42, 0.07), smoothstep(0.05, 0.35, f));
    c = mix(c, vec3(1.0, 0.82, 0.42), smoothstep(0.35, 0.75, f));
    c = mix(c, vec3(1.0, 0.97, 0.85), smoothstep(0.78, 1.0, f) * (1.0 - y));
    float flick = 0.8 + 0.4 * vnoise(vec3(t * 9.0, seed, 3.0));
    o_col = vec4(c * f * inten * 2.3 * flick, 0.0);
}
