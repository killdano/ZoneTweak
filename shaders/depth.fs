in vec3 v_pos;
out vec4 o_col;
// depth prepass: same under-deck cut as the shaded pass
void main() {
    if (v_pos.y < -0.01) discard;
    o_col = vec4(0.0);
}
