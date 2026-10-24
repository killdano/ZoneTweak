import moderngl, numpy as np, json, os, sys, math
from PIL import Image

SH = sys.argv[1]
OUT = sys.argv[2]
W, H = int(sys.argv[3]) if len(sys.argv) > 3 else 1280, int(sys.argv[4]) if len(sys.argv) > 4 else 960
FOV = float(sys.argv[5]) if len(sys.argv) > 5 else 80
Q = dict(fogDiv=3, fogSteps=40, fogLights=20, samples=4, shadow=2048, bloomLevels=7, fogDensity=0.035, bloomAmt=0.07, exposure=1.35)

ctx = moderngl.create_standalone_context(backend='egl')
def src(name, common=False):
    s = "#version 330 core\n"
    if common: s += open(os.path.join(SH, 'common.glsl')).read() + "\n"
    return s + open(os.path.join(SH, name)).read()
def prog(vs, fs, common=False):
    return ctx.program(vertex_shader=src(vs), fragment_shader=src(fs, common))
P = dict(lit=prog('lit.vs', 'lit.fs', True), floor=prog('lit.vs', 'floor.fs', True), glow=prog('glow.vs', 'glow.fs'),
         sprite=prog('sprite.vs', 'sprite.fs'), smoke=prog('smoke.vs', 'smoke.fs', True), flame=prog('smoke.vs', 'flame.fs', True), shadow=prog('shadow.vs', 'shadow.fs'), fog=prog('post.vs', 'fog.fs', True),
         fogcomp=prog('post.vs', 'fogcomp.fs'), bdown=prog('post.vs', 'bloomdown.fs'), bup=prog('post.vs', 'bloomup.fs'), final=prog('post.vs', 'final.fs'))

meta = json.load(open('dump/meta.json'))
def load(n):
    return np.fromfile('dump/%s.bin' % n, dtype='f4')
B = {n: load(n) for n in ['static', 'gen', 'env', 'floorq', 'floors', 'glowtri', 'glowline', 'glowwide', 'floor', 'envl', 'soft', 'shadow', 'smoke', 'fire', 'opaque', 'lines', 'trans', 'floorl', 'staticl']}
VB = {n: (ctx.buffer(v.tobytes()) if v.size else None) for n, v in B.items()}
TEX = {'soft', 'shadow', 'smoke', 'fire'}

def attrs(p, tex):
    names = [n for n in p]
    if tex:
        fmt, an = [], []
        for nm, f in (('a_pos', '3f'), ('a_uv', '2f'), ('a_col', '4f')):
            if nm in names: fmt.append(f); an.append(nm)
            else: fmt.append({'3f': '12x', '2f': '8x', '4f': '16x'}[f])
        return ' '.join(fmt), an
    fmt, an = [], []
    for nm, f, sz in (('a_pos', '3f', 12), ('a_nrm', '3f', 12), ('a_col', '4f', 16), ('a_mat', '4f', 16), ('a_tex', '3f', 12)):
        if nm in names: fmt.append(f); an.append(nm)
        else: fmt.append('%dx' % sz)
    return ' '.join(fmt), an

VAOS = {}
def draw(pn, bn, mode=moderngl.TRIANGLES):
    if VB.get(bn) is None: return
    key = (pn, bn)
    if key not in VAOS:
        f, an = attrs(P[pn], bn in TEX)
        VAOS[key] = ctx.vertex_array(P[pn], [(VB[bn], f, *an)])
    VAOS[key].render(mode)

def setu(p, name, val):
    if name in p:
        u = p[name]
        if isinstance(val, (list, tuple, np.ndarray)):
            u.write(np.array(val, dtype='f4').tobytes())
        else:
            u.value = val

def m4(m): return np.array(m, dtype='f4').reshape(4, 4)  # column-major rows = columns
def mul(a, b):  # column-major lists
    A = np.array(a, 'f4').reshape(4, 4).T; Bm = np.array(b, 'f4').reshape(4, 4).T
    return (A @ Bm).T.flatten()
def persp(fovy, aspect, n, f):
    t = 1 / math.tan(fovy / 2)
    m = [0.0] * 16
    m[0] = t / aspect; m[5] = t; m[10] = -(f + n) / (f - n); m[11] = -1; m[14] = -2 * f * n / (f - n)
    return m

view = list(meta['view'])
mirror = list(view)
for r in range(3): mirror[4 + r] = -mirror[4 + r]
proj = persp(math.radians(FOV), W / H, 0.05, 60)
vp = mul(proj, view)
vpm = mul(proj, mirror)
ivp = np.linalg.inv(np.array(vp, 'f4').reshape(4, 4).T).T.flatten()
eye = meta['eye']
lights = meta['lights']; nl = meta['nl']; spots = meta['spots'] or []
tm = meta['time']

# shadow maps
sres = Q['shadow']
stex = []
for i in range(2):
    d = ctx.depth_texture((sres, sres))
    d.compare_func = '<='
    d.filter = (moderngl.LINEAR, moderngl.LINEAR)
    stex.append(d)
    if i < len(spots):
        fb = ctx.framebuffer(depth_attachment=d)
        fb.use(); fb.clear(depth=1.0)
        ctx.enable(moderngl.DEPTH_TEST)
        setu(P['shadow'], 'u_vp', spots[i]['vp'])
        sl = meta.get('slide')
        if sl:
            Sp = P['shadow']; setu(Sp, 'u_slideOn', 1); setu(Sp, 'u_sO', sl['o']); setu(Sp, 'u_sX', sl['x']); setu(Sp, 'u_sZ', sl['z']); setu(Sp, 'u_sOld', sl['old']); setu(Sp, 'u_sNew', sl['new']); setu(Sp, 'u_sK', sl['k'])
        draw('shadow', 'static'); draw('shadow', 'gen'); draw('shadow', 'env')

def scene(p):
    setu(p, 'u_time', tm); setu(p, 'u_cam', eye); setu(p, 'u_cheap', float(os.environ.get('CHEAP', 0)))
    setu(p, 'u_L', lights); setu(p, 'u_nL', nl)
    ns = min(2, len(spots)); setu(p, 'u_nS', ns)
    sp, sd, sc, sm = [], [], [], []
    for i in range(2):
        s = spots[i] if i < ns else None
        if s:
            sp += s['pos'] + [s['cosOut']]; sd += s['dir'] + [s['cosIn']]; sc += list(s['col']) + [s['rng']]; sm += list(s['vp'])
        else:
            sp += [0] * 4; sd += [0] * 4; sc += [0] * 4; sm += [0] * 16
    setu(p, 'u_Spos', sp); setu(p, 'u_Sdir', sd); setu(p, 'u_Scol', sc); setu(p, 'u_SM', sm)
    setu(p, 'u_shadowTexel', 1 / sres)
    stex[0].use(1); stex[1].use(2)
    setu(p, 'u_shadow0', 1); setu(p, 'u_shadow1', 2)
    sl = meta.get('slide')
    if sl:
        setu(p, 'u_slideOn', 1); setu(p, 'u_sO', sl['o']); setu(p, 'u_sX', sl['x']); setu(p, 'u_sZ', sl['z'])
        setu(p, 'u_sOld', sl['old']); setu(p, 'u_sNew', sl['new']); setu(p, 'u_sK', sl['k'])

ms_col = ctx.renderbuffer((W, H), 4, samples=Q['samples'], dtype='f2')
ms_dep = ctx.depth_renderbuffer((W, H), samples=Q['samples'])
fb_ms = ctx.framebuffer([ms_col], ms_dep)
res_col = ctx.texture((W, H), 4, dtype='f2'); res_col.filter = (moderngl.LINEAR, moderngl.LINEAR)
res_dep = ctx.depth_texture((W, H))
fb_res = ctx.framebuffer([res_col], res_dep)

fb_ms.use(); fb_ms.clear(0, 0, 0, 0, depth=1.0)
ctx.enable(moderngl.DEPTH_TEST); ctx.disable(moderngl.BLEND)

def glow(v, wide):
    ctx.enable(moderngl.BLEND); ctx.blend_func = (moderngl.ONE, moderngl.ONE)
    fb_ms.depth_mask = False
    setu(P['glow'], 'u_vp', v); setu(P['glow'], 'u_gain', 3.0)
    draw('glow', 'glowtri'); ctx.line_width = 1.5; draw('glow', 'glowline', moderngl.LINES)
    if wide: ctx.line_width = 5; draw('glow', 'glowwide', moderngl.LINES)
    setu(P['sprite'], 'u_vp', v); setu(P['sprite'], 'u_gain', 3.5); setu(P['sprite'], 'u_dark', 0)
    draw('sprite', 'soft')
    fb_ms.depth_mask = True
    ctx.blend_func = (moderngl.SRC_ALPHA, moderngl.ONE_MINUS_SRC_ALPHA)

TINT = meta.get('emisTint', [1.0, 1.0, 1.0])
def draw_static():
    setu(P['lit'], 'u_emisTint', TINT); draw('lit', 'static'); setu(P['lit'], 'u_emisTint', [1.0, 1.0, 1.0])
L = P['lit']; scene(L); setu(L, 'u_emisGain', 4.0); setu(L, 'u_alphaMode', 0.0); setu(L, 'u_vp', vpm)
draw_static(); draw('lit', 'gen'); draw('lit', 'env')
glow(vpm, False)
ctx.enable(moderngl.BLEND); ctx.blend_func = (moderngl.SRC_ALPHA, moderngl.ONE_MINUS_SRC_ALPHA)
F = P['floor']; scene(F); setu(F, 'u_vp', vp); setu(F, 'u_fo', meta['fo']); setu(F, 'u_fx', meta['fx']); setu(F, 'u_fz', meta['fz']); setu(F, 'u_reflScale', 1.0)
draw('floor', 'floorq')
setu(L, 'u_vp', vp); setu(L, 'u_alphaMode', 1.0)
fb_ms.depth_mask = False
draw('lit', 'floors'); draw('lit', 'floor')
setu(P['sprite'], 'u_vp', vp); setu(P['sprite'], 'u_dark', 1)
draw('sprite', 'shadow')
fb_ms.depth_mask = True
ctx.disable(moderngl.BLEND)
setu(L, 'u_alphaMode', 0.0)
draw_static(); draw('lit', 'gen'); draw('lit', 'env')
setu(L, 'u_emisGain', 1.3); draw('lit', 'opaque'); setu(L, 'u_emisGain', 4.0)
ctx.line_width = 1; draw('lit', 'envl', moderngl.LINES); draw('lit', 'staticl', moderngl.LINES)
ctx.enable(moderngl.BLEND); ctx.blend_func = (moderngl.SRC_ALPHA, moderngl.ONE_MINUS_SRC_ALPHA)
setu(L, 'u_alphaMode', 1.0); ctx.line_width = 2; draw('lit', 'lines', moderngl.LINES); draw('lit', 'floorl', moderngl.LINES); setu(L, 'u_alphaMode', 0.0)
fb_ms.depth_mask = False
scene(P['smoke']); setu(P['smoke'], 'u_vp', vp); setu(P['smoke'], 'u_gain', 1.0)
if not os.environ.get('NOSMOKE'): ctx.blend_func = (moderngl.ONE, moderngl.ONE_MINUS_SRC_ALPHA); draw('smoke', 'smoke'); ctx.blend_func = (moderngl.SRC_ALPHA, moderngl.ONE_MINUS_SRC_ALPHA)
setu(P['flame'], 'u_time', tm); setu(P['flame'], 'u_vp', vp)
ctx.blend_func = (moderngl.ONE, moderngl.ONE); draw('flame', 'fire'); ctx.blend_func = (moderngl.SRC_ALPHA, moderngl.ONE_MINUS_SRC_ALPHA)
fb_ms.depth_mask = True
L = P['lit']; scene(L); setu(L, 'u_vp', vp); setu(L, 'u_alphaMode', 1.0); setu(L, 'u_emisGain', 1.0); draw('lit', 'trans'); setu(L, 'u_emisGain', 4.0); setu(L, 'u_alphaMode', 0.0)
glow(vp, True)
ctx.disable(moderngl.BLEND)

ctx.copy_framebuffer(fb_res, fb_ms)
# NANCHECK
_a=np.frombuffer(res_col.read(),dtype='f2').reshape(H,W,4).astype('f4')
_bad=~np.isfinite(_a).all(axis=2)
print('nan pixels',int(_bad.sum()), 'neg', int((_a[...,:3]<0).any(axis=2).sum()))
if _bad.sum(): ys,xs=np.nonzero(_bad); print('at', list(zip(xs[:5], (H-1-ys)[:5])))
ctx.disable(moderngl.DEPTH_TEST)

empty = ctx.vertex_array(P['fog'], [])
def full(pn):
    ctx.vertex_array(P[pn], []).render(moderngl.TRIANGLES, vertices=3)

fw, fh = W // Q['fogDiv'], H // Q['fogDiv']
fog_t = ctx.texture((fw, fh), 4, dtype='f2'); fog_t.filter = (moderngl.LINEAR, moderngl.LINEAR)
fb_fog = ctx.framebuffer([fog_t]); fb_fog.use()
Fg = P['fog']; scene(Fg)
res_dep.use(0); setu(Fg, 'u_depth', 0)
setu(Fg, 'u_ivp', ivp); setu(Fg, 'u_density', Q['fogDensity']); setu(Fg, 'u_steps', Q['fogSteps']); setu(Fg, 'u_fogLights', Q['fogLights']); setu(Fg, 'u_maxDist', 14.0); setu(Fg, 'u_frame', 1.0)
full('fog')

hdr2 = ctx.texture((W, H), 4, dtype='f2'); hdr2.filter = (moderngl.LINEAR, moderngl.LINEAR)
hdr2.repeat_x = hdr2.repeat_y = False
fb_h2 = ctx.framebuffer([hdr2]); fb_h2.use()
res_col.use(0); fog_t.use(3)
setu(P['fogcomp'], 'u_scene', 0); setu(P['fogcomp'], 'u_fog', 3)
full('fogcomp')

bl = []
bw, bh = W // 2, H // 2
for i in range(Q['bloomLevels']):
    if bw <= 8 or bh <= 8: break
    t = ctx.texture((bw, bh), 4, dtype='f2'); t.filter = (moderngl.LINEAR, moderngl.LINEAR); t.repeat_x = t.repeat_y = False
    bl.append((t, ctx.framebuffer([t]))); bw //= 2; bh //= 2
src_t = hdr2
for i, (t, fb) in enumerate(bl):
    fb.use(); src_t.use(0); setu(P['bdown'], 'u_src', 0); setu(P['bdown'], 'u_first', 1 if i == 0 else 0); full('bdown'); src_t = t
ctx.enable(moderngl.BLEND); ctx.blend_func = (moderngl.ONE, moderngl.ONE)
for i in range(len(bl) - 1, 0, -1):
    bl[i - 1][1].use(); bl[i][0].use(0); setu(P['bup'], 'u_src', 0); setu(P['bup'], 'u_radius', 1.0); full('bup')
ctx.disable(moderngl.BLEND)

out = ctx.texture((W, H), 4); fb_out = ctx.framebuffer([out]); fb_out.use()
hdr2.use(0); bl[0][0].use(4)
setu(P['final'], 'u_scene', 0); setu(P['final'], 'u_bloom', 4); setu(P['final'], 'u_bloomAmt', Q['bloomAmt']); setu(P['final'], 'u_exposure', Q['exposure']); setu(P['final'], 'u_frame', 1.0)
full('final')
img = Image.frombytes('RGBA', (W, H), fb_out.read(components=4)).transpose(Image.FLIP_TOP_BOTTOM)
img.convert('RGB').save(OUT)
print('saved', OUT, 'lights', nl, 'spots', len(spots), 'verts', {k: v.size // (9 if k in TEX else 17) for k, v in B.items()})
