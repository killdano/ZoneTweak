in vec2 v_uv;
uniform sampler2D u_scene;
uniform sampler2D u_fog;
out vec4 o_col;
void main() {
    vec3 c = texture(u_scene, v_uv).rgb;
    vec4 f = texture(u_fog, v_uv);
    o_col = vec4(c * f.a + f.rgb, 1.0);
}
