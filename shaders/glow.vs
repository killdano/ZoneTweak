layout(location = 0) in vec3 a_pos;
layout(location = 2) in vec4 a_col;
layout(location = 3) in vec4 a_mat;
uniform mat4 u_vp;
out vec4 v_col;
out float v_gain;
out float v_y;
void main() {
    v_y = a_pos.y;
    v_col = a_col;
    v_gain = max(a_mat.w, 1.0);
    gl_Position = u_vp * vec4(a_pos, 1.0);
}
