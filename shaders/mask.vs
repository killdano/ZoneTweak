layout(location = 0) in vec3 a_pos;
uniform mat4 u_proj;
// hidden-area mesh: view-space points on z = -1, pushed to the near plane so nothing behind
// them is ever shaded
void main() {
    vec4 p = u_proj * vec4(a_pos, 1.0);
    gl_Position = vec4(p.xy, -p.w * 0.9999, p.w);
}
