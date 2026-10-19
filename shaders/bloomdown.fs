in vec2 v_uv;
uniform sampler2D u_src;
uniform int u_first;
out vec4 o_col;
vec3 s(vec2 o) { return texture(u_src, v_uv + o).rgb; }
float lum(vec3 c) { return dot(c, vec3(0.2126, 0.7152, 0.0722)); }
void main() {
    vec2 t = 1.0 / vec2(textureSize(u_src, 0));
    vec3 a = s(t * vec2(-2, 2)), b = s(t * vec2(0, 2)), c = s(t * vec2(2, 2));
    vec3 d = s(t * vec2(-2, 0)), e = s(vec2(0)), f = s(t * vec2(2, 0));
    vec3 g = s(t * vec2(-2, -2)), h = s(t * vec2(0, -2)), i = s(t * vec2(2, -2));
    vec3 j = s(t * vec2(-1, 1)), k = s(t * vec2(1, 1)), l = s(t * vec2(-1, -1)), m = s(t * vec2(1, -1));
    vec3 r;
    if (u_first == 1) {
        vec3 g0 = (a + b + d + e) * 0.25, g1 = (b + c + e + f) * 0.25, g2 = (d + e + g + h) * 0.25, g3 = (e + f + h + i) * 0.25, g4 = (j + k + l + m) * 0.25;
        float w0 = 1.0 / (1.0 + lum(g0)), w1 = 1.0 / (1.0 + lum(g1)), w2 = 1.0 / (1.0 + lum(g2)), w3 = 1.0 / (1.0 + lum(g3)), w4 = 1.0 / (1.0 + lum(g4));
        r = (g0 * w0 * 0.125 + g1 * w1 * 0.125 + g2 * w2 * 0.125 + g3 * w3 * 0.125 + g4 * w4 * 0.5) / (w0 * 0.125 + w1 * 0.125 + w2 * 0.125 + w3 * 0.125 + w4 * 0.5);
    } else {
        r = e * 0.125 + (a + c + g + i) * 0.03125 + (b + d + f + h) * 0.0625 + (j + k + l + m) * 0.125;
    }
    if (any(isnan(r)) || any(isinf(r))) r = vec3(0.0);
    o_col = vec4(min(max(r, 0.0), vec3(500.0)), 1.0);
}
