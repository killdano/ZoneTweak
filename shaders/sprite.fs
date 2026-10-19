in vec4 v_col;
in vec2 v_uv;
uniform float u_gain;
uniform float u_dark;
out vec4 o_col;
void main() {
    vec2 d = v_uv * 2.0 - 1.0;
    float r2 = dot(d, d);
    float a = 0.0;
    if (v_uv.x == 0.5) r2 = d.y * d.y;
    if (r2 < 1.0) a = 0.62 * (exp(-4.0 * r2) - 0.0183) / 0.9817 + 0.38 * exp(-26.0 * r2);
    if (u_dark > 1.5) {
        float soft = smoothstep(0.0, 0.6, a);
        o_col = vec4(pow(max(v_col.rgb, 0.0), vec3(2.2)) * u_gain, soft * v_col.a);
        return;
    }
    if (u_dark > 0.5) {
        o_col = vec4(0.0, 0.0, 0.0, a * v_col.a);
        return;
    }
    vec3 c = pow(max(v_col.rgb, 0.0), vec3(2.2));
    c = mix(c, vec3(1.0), smoothstep(0.55, 1.0, a) * 0.6);
    o_col = vec4(c * a * v_col.a * u_gain, 1.0);
}
