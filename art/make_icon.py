# Contraband's app icon: your ring seen from above (green beams between generator posts),
# a magenta maze wall outside it, and the device glowing red in the middle.
from PIL import Image, ImageDraw, ImageFilter, ImageChops
import math

S = 1024
GREEN = (70, 255, 120)
MAG = (255, 60, 214)
RED = (255, 70, 50)

def glow_layer(draw_fn, blur):
    im = Image.new('RGBA', (S, S), (0, 0, 0, 0))
    d = ImageDraw.Draw(im)
    draw_fn(d)
    g = im.filter(ImageFilter.GaussianBlur(blur))
    return Image.alpha_composite(g, im)

bg = Image.new('RGBA', (S, S), (0, 0, 0, 0))
d = ImageDraw.Draw(bg)
d.rounded_rectangle([24, 24, S - 24, S - 24], radius=180, fill=(14, 18, 20, 255))
# floor grating
for k in range(80, S - 80, 64):
    d.line([(80, k), (S - 80, k)], fill=(24, 30, 32, 255), width=6)
# the ring: a slightly irregular room outline (rooms are never square)
ring = [(250, 240), (770, 215), (800, 560), (735, 800), (260, 790), (225, 520)]
def draw_ring(dd):
    dd.line(ring + [ring[0]], fill=GREEN + (255,), width=26, joint='curve')
img = Image.alpha_composite(bg, glow_layer(draw_ring, 30))
img = Image.alpha_composite(img, glow_layer(draw_ring, 10))
d = ImageDraw.Draw(img)
for (x, y) in ring:
    d.ellipse([x - 44, y - 44, x + 44, y + 44], fill=(40, 44, 46, 255), outline=(150, 160, 160, 255), width=8)
    d.ellipse([x - 20, y - 20, x + 20, y + 20], fill=GREEN + (255,))
# a magenta wall out in the hall
def draw_wall(dd):
    dd.line([(130, 930), (560, 930)], fill=MAG + (255,), width=26)
    dd.line([(880, 120), (880, 460)], fill=MAG + (255,), width=26)
img = Image.alpha_composite(img, glow_layer(draw_wall, 22))
# the device: a red core in the middle
def draw_dev(dd):
    dd.rounded_rectangle([420, 420, 600, 600], radius=26, fill=(60, 66, 70, 255), outline=(170, 175, 175, 255), width=10)
    dd.ellipse([470, 470, 550, 550], fill=RED + (255,))
img = Image.alpha_composite(img, glow_layer(draw_dev, 26))
img.save('contraband_icon.png')
sizes = [(256, 256), (128, 128), (64, 64), (48, 48), (32, 32), (24, 24), (16, 16)]
img.resize((256, 256), Image.LANCZOS).save('Contraband.ico', sizes=sizes)
print('ok')
