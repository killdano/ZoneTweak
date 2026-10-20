layout(location = 0) in vec3 a_pos;
layout(location = 1) in vec3 a_nrm;
layout(location = 2) in vec4 a_col;
layout(location = 3) in vec4 a_mat;
layout(location = 4) in vec3 a_tex;
uniform mat4 u_vp;
invariant gl_Position; // the depth prepass and the shaded pass must land on identical depths
out vec3 v_pos;
out vec3 v_nrm;
out vec4 v_col;
out vec4 v_mat;
out vec3 v_tex;
// a cell wall sliding back on its rails (see wallpush.go): the room is already built at its
// new size, and everything near the moving wall is drawn back toward its old place
uniform int u_slideOn;
uniform vec3 u_sO;
uniform vec3 u_sX;
uniform vec3 u_sZ;
uniform vec4 u_sOld; // walls x0, x1, z0, z1 in the room frame
uniform vec4 u_sNew;
uniform float u_sK;
float slideEdge(float u, float o, float n, float sg) {
    u *= sg; o *= sg; n *= sg;
    float d = n - o;
    if (d > 1e-4) {
        float a = o - 0.55, b = n - 0.55;
        if (u > a) u = (u < b) ? a + (u - a) * u_sK : u - d * (1.0 - u_sK);
    }
    return u * sg;
}
vec3 slideMap(vec3 p) {
    if (u_slideOn == 0) return p;
    vec3 r = p - u_sO;
    float x = dot(r, u_sX), z = dot(r, u_sZ);
    x = slideEdge(x, u_sOld.y, u_sNew.y, 1.0);
    x = slideEdge(x, u_sOld.x, u_sNew.x, -1.0);
    z = slideEdge(z, u_sOld.w, u_sNew.w, 1.0);
    z = slideEdge(z, u_sOld.z, u_sNew.z, -1.0);
    return vec3(u_sO.x, 0.0, u_sO.z) + u_sX * x + u_sZ * z + vec3(0.0, p.y, 0.0);
}
void main() {
    vec3 P = slideMap(a_pos);
    v_pos = P;
    v_nrm = a_nrm;
    v_col = a_col;
    v_mat = a_mat;
    v_tex = a_tex;
    gl_Position = u_vp * vec4(P, 1.0);
}
