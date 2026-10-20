layout(location = 0) in vec3 a_pos;
uniform mat4 u_vp;
invariant gl_Position; // the depth prepass and the shaded pass must land on identical depths
void main() { gl_Position = u_vp * vec4(a_pos, 1.0); }
