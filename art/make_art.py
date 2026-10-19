# Cover + icon in the in-game look: rusted industrial cell, green laser walls, generator posts.
# cover_render.png is a frame from the game's own renderer (preview harness).
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

def cover(W=1600, H=1000):
    base = Image.open('cover_render.png').convert('RGB').resize((W, H), Image.LANCZOS)
    img = base.convert('RGBA')
    # smoke the top-left corner for the title and the bottom edge for the feature line,
    # leaving the fire crew in the middle clear
    shade = Image.new('L', (W, H), 0)
    sd_ = ImageDraw.Draw(shade)
    sd_.ellipse([-W * 0.5, -H * 0.7, W * 0.64, H * 0.5], fill=215)
    sd_.rectangle([0, H * 0.88, W, H], fill=190)
    shade = shade.filter(ImageFilter.GaussianBlur(90))
    img = Image.composite(Image.new('RGBA', (W, H), (3, 7, 5, 255)), img, shade)
    vig = Image.new('L', (W, H), 0)
    ImageDraw.Draw(vig).ellipse([-W * 0.25, -H * 0.3, W * 1.25, H * 1.3], fill=255)
    vig = vig.filter(ImageFilter.GaussianBlur(160))
    img = Image.composite(img, Image.new('RGBA', (W, H), (0, 0, 0, 255)), vig)
    # title
    layer = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    d = ImageDraw.Draw(layer)
    ft = ImageFont.truetype(F_BLACK, 116)
    fs = ImageFont.truetype(F_MED, 30)
    x0, y0 = 78, 92
    x = tracked(d, (x0, y0), "ZONE", ft, (226, 232, 228, 255), 5)
    tracked(d, (x, y0), "TWEAK", ft, GREEN + (255,), 5)
    tw = text_width(d, "ZONETWEAK", ft, 5)
    ry = y0 + 148
    d.rectangle([x0 + 4, ry, x0 + tw, ry + 9], fill=(28, 26, 18, 255))
    for k in range(0, int(tw), 24):
        d.polygon([(x0 + 4 + k, ry + 9), (x0 + 4 + k + 12, ry), (x0 + 4 + k + 24, ry), (x0 + 4 + k + 12, ry + 9)], fill=(232, 178, 40, 255))
    a = ImageChops.multiply(layer.getchannel('A'), grunge_mask(W, H))
    layer.putalpha(a)
    glow = Image.new('RGBA', (W, H), (0, 0, 0, 0))
    tracked(ImageDraw.Draw(glow), (x, y0), "TWEAK", ft, GREEN + (170,), 5)
    img = Image.alpha_composite(img, glow.filter(ImageFilter.GaussianBlur(16)))
    img = Image.alpha_composite(img, layer)

    def clean_text(img, xy, text, font, fill, track, plate_pad=None):
        dd = ImageDraw.Draw(img)
        w = text_width(dd, text, font, track)
        x, y = xy
        if plate_pad:
            pl = Image.new('RGBA', (W, H), (0, 0, 0, 0))
            ImageDraw.Draw(pl).rectangle([x - plate_pad, y - 6, x + w + plate_pad, y + font.size + 14], fill=(2, 6, 4, 110))
            img = Image.alpha_composite(img, pl.filter(ImageFilter.GaussianBlur(12)))
        sh = Image.new('RGBA', (W, H), (0, 0, 0, 0))
        tracked(ImageDraw.Draw(sh), (x + 1, y + 2), text, font, (0, 0, 0, 160), track)
        img = Image.alpha_composite(img, sh.filter(ImageFilter.GaussianBlur(2)))
        tracked(ImageDraw.Draw(img), (x, y), text, font, fill, track)
        return img, w

    img, _ = clean_text(img, (x0 + 6, ry + 24), "PLAY-SPACE EDITOR FOR PIMAX LIGHTHOUSE", fs, (236, 244, 238, 255), 5, 14)

    # feature line along the bottom: what lives in the cell now
    ff = ImageFont.truetype(F_MED, 22)
    feats = ["TELESCOPING LASER EMITTERS", "ROBOT SALVAGE CREWS", "JOINT BASES", "FIRE INCIDENTS"]
    fx_, fy = x0 + 6, H - 74
    for i, f in enumerate(feats):
        if i:
            ImageDraw.Draw(img).polygon([(fx_ + 8, fy + 3), (fx_ + 20, fy + 3), (fx_ + 14, fy + 21), (fx_ + 2, fy + 21)], fill=(232, 178, 40, 255))
            fx_ += 40
        img, w = clean_text(img, (fx_, fy), f, ff, (210, 222, 214, 255), 4)
        fx_ += w + 18
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
