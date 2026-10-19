in vec4 v_col;
in float v_gain;
in float v_y;
uniform float u_gain;
out vec4 o_col;
void main() {
    if (v_y < -0.01) discard;
    o_col = vec4(pow(max(v_col.rgb, 0.0), vec3(2.2)) * v_col.a * v_gain * u_gain, 1.0);
}
