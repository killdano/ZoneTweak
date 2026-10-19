in vec4 v_col;
in float v_gain;
uniform float u_gain;
out vec4 o_col;
void main() {
    o_col = vec4(pow(max(v_col.rgb, 0.0), vec3(2.2)) * v_col.a * v_gain * u_gain, 1.0);
}
