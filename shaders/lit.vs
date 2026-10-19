layout(location = 0) in vec3 a_pos;
layout(location = 1) in vec3 a_nrm;
layout(location = 2) in vec4 a_col;
layout(location = 3) in vec4 a_mat;
layout(location = 4) in vec3 a_tex;
uniform mat4 u_vp;
out vec3 v_pos;
out vec3 v_nrm;
out vec4 v_col;
out vec4 v_mat;
out vec3 v_tex;
void main() {
    v_pos = a_pos;
    v_nrm = a_nrm;
    v_col = a_col;
    v_mat = a_mat;
    v_tex = a_tex;
    gl_Position = u_vp * vec4(a_pos, 1.0);
}
