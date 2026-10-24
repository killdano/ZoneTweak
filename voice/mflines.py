# Contact lines for the misfire (build 12), processed like newlines.py (radio band, level).
import os, sys, numpy as np
from scipy import signal
from kokoro_onnx import Kokoro
TTS = os.environ.get('KOKORO', 'tts')
OUT = os.path.dirname(os.path.abspath(__file__))
k = Kokoro(os.path.join(TTS, "kokoro-v1.0.onnx"), os.path.join(TTS, "voices-v1.0.bin"))
voice = 'am_michael'
cols = ["red", "blue", "yellow", "green", "white"]
L = {
    "x_mf_start": "It didn't go off! Open the hatch on top, pull the handle up, and pick up again. Something in there's fried. I'll talk you through it.",
    "x_mf_up": "Okay, it's up. Good.",
    "x_mf_wrong": "No, no, no, not that one! Okay. Okay. We'll have to go another way.",
    "x_mf_patch": "Good, it's cut. Now the yellow patch cable, in the cart's bottom tray. Hold it over the cut, so it bridges both ends, and let go. It clamps on.",
    "x_mf_done": "Push the handle back down, start it up, and hit the button. That should do it.",
    "mf_orders": "Here's what you do.",
    "mf_also": "Then,",
    "mf_card": "Find the burnt circuit card, and swap in the spare from the cart's bottom shelf. It's hard to see. Use the flashlight.",
    "mf_cutters": "The wire cutters are on the cart's bottom shelf.",
    "x_mf_again": "It misfired again! Open it back up and pick up. There's more.",
    "mf_relay": "Find the relay with the cracked case, and swap in the spare from the shelf.",
    "mf_cap": "One of the capacitors has blown. Its top's bulged, just a little. Swap in the spare.",
    "mf_look": "Here's what to look for.",
    "x_mf_help": "Still stuck? Here's what to look for.",
    "mf_d_wire": "The thin wires run every which way between the boards. A striped one is mostly one colour, with short bands of the other. Put the cutters' jaws right on the wire, and squeeze the trigger.",
    "mf_d_cable": "The thick cables are the fat ones, as thick as your finger. Cut it with the cutters. Then take the yellow patch cable from the tray, hold it over the cut so it bridges both ends, and let go. It clamps on. The cut glows yellow till it's patched.",
    "mf_d_fuse": "A fuse is a little glass tube lying in a pair of metal clips, with a coloured cap on each end. In a blown one, the wire inside is snapped. Pull it out, and drop the spare with the same colour caps into the empty clips.",
    "mf_d_card": "Circuit cards are the thin green boards standing up in black slots. The burnt one has a dark, scorched corner, and it's hard to see. Shine the flashlight across them: the burnt spot glints, like embers. Pull it straight up, and set the spare card in the empty slot.",
    "mf_d_relay": "A relay is a little dark blue box, about the size of a sugar cube, with a clear top. The bad one has a fine crack across its top. Pull it off its socket, and put the spare relay from the tray in its place.",
    "mf_d_cap": "Capacitors are the little blue cans standing on end, with a shiny metal top. A good one's top is flat. The blown one's top is domed up, just a little. Pull it, and stand the spare in its place.",
    "mf_d_jumper": "Jumpers are tiny coloured blocks sitting on two little gold pins. Just pull the one I said straight off. You don't put anything back.",
    "h_d_need": "You'll need the screwdriver for this one.",
    "h_w_need": "You'll need the wrench for this one.",
}
# the tool and where it is first; that you need it, after
places = {"cart": "on the cart", "gen": "by one of the generators", "radio": "down by the radio", "console": "by the console",
          "turret": "by the harpoon turret", "middle": "on the floor, in the middle of the room",
          "north": "on the floor by the north wall", "east": "on the floor by the east wall",
          "south": "on the floor by the south wall", "west": "on the floor by the west wall"}
for _pk, _pw in places.items():
    L["h_d_at_" + _pk] = "The screwdriver's %s. You'll need it for this one." % _pw
    L["h_w_at_" + _pk] = "The wrench is %s. You'll need it for this one." % _pw
# where the prod is, said after the leech warning
for _d in ["north", "east", "south", "west"]:
    L["p_br_" + _d] = "The prod's in its bracket, on the generator by the %s wall." % _d
L["p_br_middle"] = "The prod's in its bracket, on one of the generators."
for _pk, _pw in places.items():
    L["p_at_" + _pk] = "The prod's %s." % _pw
# lines first recorded elsewhere, re-said with what matters up front
import re as _re
for _f, _keys in [("lines.py", ["x_leech", "x_start", "h_need", "x_genblown"]), ("newlines.py", ["h_wneed", "x_harp_super", "x_harp_tie", "x_tractor", "x_leads", "x_reel_mode"])]:
    _src = open(os.path.join(OUT, _f)).read()
    for _k in _keys:
        _m = _re.search(r'(?:"%s":\s*|L\["%s"\]\s*=\s*)"([^"]*)"' % (_k, _k), _src)
        if _m:
            L[_k] = _m.group(1)
for i, a in enumerate(cols):
    L["mf_w_" + a] = "Cut the %s wire." % a
    for b in cols[i + 1:]:
        L["mf_w_%s_%s" % (a, b)] = "Cut the %s and %s wire." % (a, b)
for c in ["orange", "grey", "black", "purple"]:
    L["mf_c_" + c] = "Cut the thick %s cable, and splice in the patch cable from the shelf." % c
for j in ["red", "blue", "green", "yellow"]:
    L["mf_j_" + j] = "Pull out the %s jumper." % j
for f in ["red", "blue", "green", "yellow", "white", "orange"]:
    L["mf_f_" + f] = "Replace the %s fuse. There's a spare on the cart's bottom shelf." % f
only = sys.argv[1:] if len(sys.argv) > 1 else list(L)
def process(a, sr):
    thr = 0.01 * np.max(np.abs(a))
    idx = np.nonzero(np.abs(a) > thr)[0]
    a = a[max(0, idx[0] - int(0.02 * sr)): idx[-1] + int(0.06 * sr)]
    a = signal.resample_poly(a, 2, 3) if sr == 24000 else a
    sr = 16000
    b, aa = signal.butter(4, [300, 3400], btype='band', fs=sr)
    a = signal.lfilter(b, aa, a)
    a = a / np.max(np.abs(a))
    a = np.tanh(1.6 * a) / np.tanh(1.6)
    a = a * (0.07 / np.sqrt((a ** 2).mean()))
    return np.clip(a, -0.95, 0.95)
for key in only:
    v, sr = k.create(L[key], voice=voice, speed=1.08, lang="en-us")
    a = process(v, sr)
    (a * 32768).astype('<i2').tofile(os.path.join(OUT, key + '.raw'))
    print(key, round(len(a) / 16000, 2), 's')
