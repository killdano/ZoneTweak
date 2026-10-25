# Cover + icon in the in-game look: rusted industrial cell, green laser walls, generator posts.
# cover_render.png (ZoneTweak) and cover_game.png (a round of Contraband) are frames from the
# app's own renderer (the preview harness).
from PIL import Image, ImageDraw, ImageFilter, ImageFont, ImageChops
import math, random

GREEN = (70, 255, 120)
F_BLACK = '/usr/share/fonts/opentype/inter/InterDisplay-Black.otf'
F_MED = '/usr/share/fonts/opentype/inter/InterDisplay-SemiBold.otf'
random.seed(7)

def noise_tex(w, h, scale, seed):
    random.seed(seed)
    small = Image.new('L', (max(2, w // scale), max(2, h // scale)))
    small.putdata([random.randint(0, 255) for _ in range(small.size[0] * small.size[1])])
    return small.resize((w, h), Image.BICUBIC)

def tracked(draw, xy, text, font, fill, track):
    x, y = xy
    for ch in text:
        draw.text((x, y), ch, font=font, fill=fill)
        x += draw.textlength(ch, font=font) + track
    return x

def text_width(draw, text, font, track):
    return sum(draw.textlength(c, font=font) + track for c in text) - track

def grunge_mask(w, h):
    # scratches + speckle eating into the lettering
    m = Image.new('L', (w, h), 255)
    d = ImageDraw.Draw(m)
    for _ in range(int(w * h / 900)):
        x, y = random.randint(0, w), random.randint(0, h)
        r = random.choice([0, 1, 1, 1, 2])
        d.ellipse([x - r, y - r, x + r, y + r], fill=random.randint(0, 120))
    for _ in range(60):
        x, y = random.randint(0, w), random.randint(0, h)
        a = random.uniform(-0.4, 0.4)
        l = random.randint(20, 140)
        d.line([(x, y), (x + math.cos(a) * l, y + math.sin(a) * l)], fill=random.randint(40, 150), width=1)
    n = noise_tex(w, h, 3, 3).point(lambda v: 255 if v > 40 else 190)
    return ImageChops.multiply(m, n)

RED = (255, 62, 48)
AMBER = (232, 178, 40)


def hazard_band(d, a, b, w, phase=0):
    """a hazard-striped band from point a to point b, w wide"""
    ax, ay = a
    bx, by = b
    L = math.hypot(bx - ax, by - ay)
    ux, uy = (bx - ax) / L, (by - ay) / L
    nx, ny = -uy * w / 2, ux * w / 2
    d.polygon([(ax + nx, ay + ny), (bx + nx, by + ny), (bx - nx, by - ny), (ax - nx, ay - ny)], fill=(22, 20, 14, 255))
    step = w * 1.4
    k = -step + phase
    while k < L + step:
        p0 = (ax + ux * k, ay + uy * k)
        p1 = (ax + ux * (k + step / 2), ay + uy * (k + step / 2))
        q0 = (p0[0] + ux * w * 0.6, p0[1] + uy * w * 0.6)
        q1 = (p1[0] + ux * w * 0.6, p1[1] + uy * w * 0.6)
        d.polygon([(p0[0] + nx, p0[1] + ny), (p1[0] + nx, p1[1] + ny), (q1[0] - nx, q1[1] - ny), (q0[0] - nx, q0[1] - ny)], fill=AMBER + (255,))
        k += step


def cover(W=1600, H=1000):
    """ZoneTweak, and the game that comes with it: the editor's cell on the left, a round of
    Contraband on the right, split on a hazard-striped diagonal."""
    def shifted(path, dx):
        # each frame slid so its subject sits in its own half (the robots, the cart)
        src = Image.open(path).convert('RGB').resize((W, H), Image.LANCZOS)
        out = Image.new('RGB', (W, H), (0, 0, 0))
        out.paste(src, (int(dx), 0))
        return out
    zone = shifted('cover_render.png', -W * 0.3)
    game = shifted('cover_game.png', W * 0.22)
    # the game side graded a little warmer and harder, so the two halves read apart
    r, g, b = game.split()
    game = Image.merge('RGB', (r.point(lambda v: min(255, int(v * 1.18 + 6))), g.point(lambda v: int(v * 0.9)), b.point(lambda v: int(v * 0.95))))
    top, bot = W * 0.645, W * 0.47  # where the seam meets the top and bottom edges
    m = Image.new('L', (W, H), 0)
    ImageDraw.Draw(m).polygon([(top, 0), (W, 0), (W, H), (bot, H)], fill=255)
    img = Image.composite(game, zone, m.filter(ImageFilter.GaussianBlur(1))).convert('RGBA')
    # smoke behind the title block (top left) and along the bottom for the tagline
    shade = Image.new('L', (W, H), 0)
    sd = ImageDraw.Draw(shade)
    sd.ellipse([-W * 0.45, -H * 0.8, W * 0.66, H * 0.62], fill=205)
    sd.rectangle([0, H * 0.86, W, H], fill=200)
    shade = shade.filter(ImageFilter.GaussianBlur(90))
    img = Image.composite(Image.new('RGBA', (W, H), (3, 6, 5, 255)), img, shade)
    vig = Image.new('L', (W, H), 0)
    ImageDraw.Draw(vig).ellipse([-W * 0.25, -H * 0.3, W * 1.25, H * 1.3], fill=255)
    vig = vig.filter(ImageFilter.GaussianBlur(160))
    img = Image.composite(img, Image.new('RGBA', (W, H), (0, 0, 0, 255)), vig)
    # the seam: a hazard band with a red glow on the game side
    sg = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    ImageDraw.Draw(sg).line([(top + 8, -10), (bot + 8, H + 10)], fill=RED + (150,), width=26)
    img = Image.alpha_composite(img, sg.filter(ImageFilter.GaussianBlur(22)))
    band = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    hazard_band(ImageDraw.Draw(band), (top, -20), (bot, H + 20), 16)
    img = Image.alpha_composite(img, band)

    def clean_text(img, xy, text, font, fill, track, plate_pad=None, glow=None):
        dd = ImageDraw.Draw(img)
        w = text_width(dd, text, font, track)
        x, y = xy
        if plate_pad:
            pl = Image.new('RGBA', (W, H), (0, 0, 0, 0))
            ImageDraw.Draw(pl).rectangle([x - plate_pad, y - 6, x + w + plate_pad, y + font.size + 14], fill=(2, 6, 4, 120))
            img = Image.alpha_composite(img, pl.filter(ImageFilter.GaussianBlur(12)))
        if glow:
            gl = Image.new('RGBA', (W, H), (0, 0, 0, 0))
            tracked(ImageDraw.Draw(gl), (x, y), text, font, glow, track)
            img = Image.alpha_composite(img, gl.filter(ImageFilter.GaussianBlur(14)))
        sh = Image.new('RGBA', (W, H), (0, 0, 0, 0))
        tracked(ImageDraw.Draw(sh), (x + 1, y + 2), text, font, (0, 0, 0, 170), track)
        img = Image.alpha_composite(img, sh.filter(ImageFilter.GaussianBlur(2)))
        tracked(ImageDraw.Draw(img), (x, y), text, font, fill, track)
        return img, w

    # ZONETWEAK, grunged, the TWEAK glowing green
    layer = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    ft = ImageFont.truetype(F_BLACK, 116)
    x0, y0 = 78, 70
    x = tracked(d, (x0, y0), "ZONE", ft, (226, 232, 228, 255), 5)
    tracked(d, (x, y0), "TWEAK", ft, GREEN + (255,), 5)
    layer.putalpha(ImageChops.multiply(layer.getchannel('A'), grunge_mask(W, H)))
    glow = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    tracked(ImageDraw.Draw(glow), (x, y0), "TWEAK", ft, GREEN + (170,), 5)
    img = Image.alpha_composite(img, glow.filter(ImageFilter.GaussianBlur(16)))
    img = Image.alpha_composite(img, layer)
    tw = text_width(ImageDraw.Draw(img), "ZONETWEAK", ft, 5)
    tl, tt, _, tb = ft.getbbox("Z")

    # "+ CONTRABAND" under it as the rest of the name: the + in the title's own white, the
    # game's name on a red plate that ends where ZONETWEAK does, its text dead centre on it
    line2 = y0 + 152
    plus = ImageFont.truetype(F_BLACK, 104)
    pd0 = ImageDraw.Draw(img)
    pl_, pt_, pr_, pb_ = plus.getbbox("+")
    plus_w = pr_ - pl_
    gap, pad_x, pad_y = 22, 26, 18
    plate_x0 = x0 + plus_w + gap
    plate_x1 = x0 + tw
    # the biggest CONTRABAND that fits the plate
    size = 100
    while size > 40:
        fc = ImageFont.truetype(F_BLACK, size)
        if text_width(pd0, "CONTRABAND", fc, 5) <= plate_x1 - plate_x0 - 2 * pad_x:
            break
        size -= 2
    cw = text_width(pd0, "CONTRABAND", fc, 5)
    cl_, ct_, _, cb_ = fc.getbbox("CONTRABAND")
    ch = cb_ - ct_
    plate_y0 = line2
    plate_y1 = line2 + ch + 2 * pad_y
    tx = plate_x0 + (plate_x1 - plate_x0 - cw) / 2 - cl_
    ty = plate_y0 + pad_y - ct_
    pl = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    pd = ImageDraw.Draw(pl)
    pd.rectangle([plate_x0, plate_y0, plate_x1, plate_y1], fill=(44, 6, 4, 215))
    pd.rectangle([plate_x0, plate_y0, plate_x1, plate_y1], outline=RED + (255,), width=3)
    img = Image.alpha_composite(img, pl)
    # the +, centred on the plate's height, grunged like the title
    pls = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    py = (plate_y0 + plate_y1) / 2 - (pt_ + pb_) / 2
    ImageDraw.Draw(pls).text((x0 - pl_, py), "+", font=plus, fill=(226, 232, 228, 255))
    pls.putalpha(ImageChops.multiply(pls.getchannel('A'), grunge_mask(W, H)))
    img = Image.alpha_composite(img, pls)
    cg = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    tracked(ImageDraw.Draw(cg), (tx, ty), "CONTRABAND", fc, RED + (190,), 5)
    img = Image.alpha_composite(img, cg.filter(ImageFilter.GaussianBlur(14)))
    cl = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    tracked(ImageDraw.Draw(cl), (tx, ty), "CONTRABAND", fc, (255, 92, 70, 255), 5)
    cl.putalpha(ImageChops.multiply(cl.getchannel('A'), grunge_mask(W, H)))
    img = Image.alpha_composite(img, cl)

    # what they are
    fs = ImageFont.truetype(F_MED, 27)
    sy = plate_y1 + 30
    img, _ = clean_text(img, (x0 + 6, sy), "PLAY-SPACE EDITOR FOR PIMAX & STEAMVR", fs, (236, 244, 238, 255), 5, 14)
    img, _ = clean_text(img, (x0 + 6, sy + 46), "A VR GAME IN YOUR OWN ROOM", fs, AMBER + (255,), 5, 14)

    # the tagline along the bottom, split across the seam: edit it on the left, defend it on the right
    fb = ImageFont.truetype(F_BLACK, 44)
    by = H - 92
    img, _ = clean_text(img, (x0 + 6, by), "TWEAK YOUR ZONE.", fb, (236, 244, 238, 255), 4, glow=GREEN + (120,))
    dd = ImageDraw.Draw(img)
    tw = text_width(dd, "THEN DEFEND IT.", fb, 4)
    img, _ = clean_text(img, (W - 78 - tw, by), "THEN DEFEND IT.", fb, (255, 214, 200, 255), 4, glow=RED + (150,))
    g = noise_tex(W, H, 1, 11).point(lambda v: int((v - 128) * 0.06 + 128))
    img = Image.blend(img.convert('RGB'), ImageChops.add(img.convert('RGB'), Image.merge('RGB', [g, g, g]), 1, -128), 0.5)
    return img


def icon(N=256):
    S = 4
    n = N * S
    img = Image.new('RGBA', (n, n), (0, 0, 0, 0))
    # rusted gunmetal plate
    plate = Image.new('RGBA', (n, n), (22, 26, 26, 255))
    rust = noise_tex(n, n, 40, 5)
    fine = noise_tex(n, n, 6, 6)
    rc = Image.new('RGBA', (n, n), (92, 44, 20, 255))
    rmask = rust.point(lambda v: max(0, min(110, (v - 160) * 2)))
    plate = Image.composite(rc, plate, rmask.filter(ImageFilter.GaussianBlur(10)))
    plate = Image.blend(plate, Image.merge('RGBA', [fine, fine, fine, Image.new('L', (n, n), 255)]), 0.07)
    pd = ImageDraw.Draw(plate)
    # hazard band at the bottom
    hy = int(n * 0.86)
    pd.rectangle([0, hy, n, n], fill=(30, 26, 14, 255))
    for k in range(-n, n * 2, int(n * 0.11)):
        w = int(n * 0.055)
        pd.polygon([(k, n), (k + w, n), (k + w + (n - hy), hy), (k + (n - hy), hy)], fill=(214, 160, 30, 255))
    # rivets
    for (rx, ry) in [(0.08, 0.08), (0.92, 0.08), (0.08, 0.78), (0.92, 0.78)]:
        cx, cy, r = rx * n, ry * n, n * 0.022
        pd.ellipse([cx - r, cy - r, cx + r, cy + r], fill=(70, 74, 76, 255))
        pd.ellipse([cx - r * 0.5, cy - r * 0.7, cx + r * 0.2, cy - r * 0.1], fill=(150, 156, 158, 255))
    img = plate
    # two laser wall planes meeting at a generator post (corner of the zone)
    cx, top, bot = n * 0.5, n * 0.14, n * 0.74
    walls = Image.new('RGBA', (n, n), (0, 0, 0, 0))
    wd = ImageDraw.Draw(walls)
    L = (n * 0.06, top + n * 0.10, n * 0.06, bot + n * 0.02)
    R = (n * 0.94, top + n * 0.10, n * 0.94, bot + n * 0.02)
    for (xa, ya, xb, yb) in [L, R]:
        wd.polygon([(cx, top), (xa, ya), (xb, yb), (cx, bot)], fill=GREEN + (52,))
        for k in range(1, 7):
            u = k / 7
            x = cx + (xa - cx) * u
            wd.line([(x, top + (ya - top) * u), (x, bot + (yb - bot) * u)], fill=GREEN + (90,), width=S * 2)
        wd.line([(cx, top), (xa, ya)], fill=GREEN + (255,), width=S * 5)
        wd.line([(cx, bot), (xb, yb)], fill=GREEN + (255,), width=S * 5)
    glow = walls.filter(ImageFilter.GaussianBlur(n * 0.02))
    img = Image.alpha_composite(img, glow)
    img = Image.alpha_composite(img, walls)
    # the generator post
    d = ImageDraw.Draw(img)
    pw = n * 0.055
    d.rectangle([cx - pw, top + n * 0.06, cx + pw, bot - n * 0.04], fill=(60, 64, 66, 255))
    d.rectangle([cx - pw * 0.45, top + n * 0.06, cx - pw * 0.15, bot - n * 0.04], fill=(150, 156, 160, 255))
    for k in range(4):
        y = top + n * 0.16 + k * n * 0.11
        d.rectangle([cx - pw * 1.25, y, cx + pw * 1.25, y + n * 0.018], fill=(36, 38, 40, 255))
    # base with legs
    d.ellipse([cx - n * 0.2, bot - n * 0.05, cx + n * 0.2, bot + n * 0.06], fill=(40, 42, 44, 255))
    d.ellipse([cx - n * 0.2, bot - n * 0.05, cx + n * 0.2, bot + n * 0.04], outline=(214, 160, 30, 255), width=S * 4)
    for s in (-1, 1):
        d.line([(cx + s * pw, bot - n * 0.12), (cx + s * n * 0.15, bot)], fill=(170, 176, 180, 255), width=S * 6)
    d.rectangle([cx - pw * 1.6, bot - n * 0.1, cx + pw * 1.6, bot - n * 0.04], fill=(70, 74, 76, 255))
    for k in range(-2, 3):
        x = cx + k * pw * 0.55
        d.line([(x, bot - n * 0.095), (x, bot - n * 0.045)], fill=GREEN + (255,), width=S * 3)
    # brass cap + green beacon
    d.rounded_rectangle([cx - pw * 1.05, top - n * 0.02, cx + pw * 1.05, top + n * 0.08], radius=pw, fill=(176, 140, 40, 255))
    b = Image.new('RGBA', (n, n), (0, 0, 0, 0))
    bd = ImageDraw.Draw(b)
    bd.ellipse([cx - n * 0.07, top - n * 0.11, cx + n * 0.07, top + n * 0.03], fill=GREEN + (255,))
    img = Image.alpha_composite(img, b.filter(ImageFilter.GaussianBlur(n * 0.03)))
    d = ImageDraw.Draw(img)
    d.ellipse([cx - n * 0.028, top - n * 0.068, cx + n * 0.028, top - n * 0.012], fill=(220, 255, 230, 255))
    # rounded tile + dark bevel
    mask = Image.new('L', (n, n), 0)
    ImageDraw.Draw(mask).rounded_rectangle([0, 0, n - 1, n - 1], radius=int(n * 0.2), fill=255)
    edge = Image.new('RGBA', (n, n), (0, 0, 0, 0))
    ImageDraw.Draw(edge).rounded_rectangle([S * 3, S * 3, n - S * 3, n - S * 3], radius=int(n * 0.19), outline=(0, 0, 0, 170), width=S * 10)
    img = Image.alpha_composite(img, edge.filter(ImageFilter.GaussianBlur(S * 4)))
    img.putalpha(mask)
    return img.resize((N, N), Image.LANCZOS)

cover().save('cover.png', optimize=True)
if '--icon' in __import__('sys').argv:
    ic = icon(256)
    ic.save('icon.png')
    ic.save('ZoneTweak.ico', sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)])
print('ok')
