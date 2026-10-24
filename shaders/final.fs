in vec2 v_uv;
uniform sampler2D u_scene;
uniform sampler2D u_bloom;
uniform float u_bloomAmt;
uniform float u_exposure;
uniform float u_frame;
uniform float u_fade; // 0 clear .. 1 black (switching between ZoneTweak and the game)
out vec4 o_col;
vec3 aces(vec3 x) {
    const mat3 i = mat3(0.59719, 0.07600, 0.02840, 0.35458, 0.90834, 0.13383, 0.04823, 0.01566, 0.83777);
    const mat3 o = mat3(1.60475, -0.10208, -0.00327, -0.53108, 1.10813, -0.07276, -0.07367, -0.00605, 1.07602);
    vec3 v = i * x;
    vec3 a = v * (v + 0.0245786) - 0.000090537;
    vec3 b = v * (0.983729 * v + 0.4329510) + 0.238081;
    return clamp(o * (a / b), 0.0, 1.0);
}
void main() {
    vec2 d = v_uv - 0.5;
    float r2 = dot(d, d);
    vec2 ca = d * r2 * 0.006;
    vec3 c;
    c.r = texture(u_scene, v_uv - ca).r;
    c.g = texture(u_scene, v_uv).g;
    c.b = texture(u_scene, v_uv + ca).b;
    vec3 b = texture(u_bloom, v_uv).rgb;
    c = mix(c, c + b, u_bloomAmt);
    c += b * u_bloomAmt * 0.6;
    c *= u_exposure;
    c = aces(c);
    c *= 1.0 - smoothstep(0.18, 0.55, r2) * 0.45;
    c = pow(c, vec3(1.0 / 2.2));
    float n = fract(sin(dot(gl_FragCoord.xy + u_frame * 13.1, vec2(12.9898, 78.233))) * 43758.5453);
    c += (n - 0.5) * 0.012;
    o_col = vec4(c * (1.0 - u_fade), 1.0);
}
