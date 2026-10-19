in vec2 v_uv;
uniform sampler2D u_src;
uniform float u_radius;
out vec4 o_col;
void main() {
    vec2 t = u_radius / vec2(textureSize(u_src, 0));
    vec3 c = texture(u_src, v_uv).rgb * 4.0;
    c += (texture(u_src, v_uv + vec2(-t.x, 0)).rgb + texture(u_src, v_uv + vec2(t.x, 0)).rgb + texture(u_src, v_uv + vec2(0, -t.y)).rgb + texture(u_src, v_uv + vec2(0, t.y)).rgb) * 2.0;
    c += texture(u_src, v_uv + vec2(-t.x, -t.y)).rgb + texture(u_src, v_uv + vec2(t.x, -t.y)).rgb + texture(u_src, v_uv + vec2(-t.x, t.y)).rgb + texture(u_src, v_uv + vec2(t.x, t.y)).rgb;
    o_col = vec4(c / 16.0, 1.0);
}
