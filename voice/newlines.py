# New contact lines for build 10, processed to sound like the old ones (radio band, level).
import os, sys, numpy as np, soundfile as sf
from scipy import signal
from kokoro_onnx import Kokoro
TTS = os.environ.get('KOKORO', 'tts')  # folder with kokoro-v1.0.onnx and voices-v1.0.bin
OUT = os.path.dirname(os.path.abspath(__file__))
k = Kokoro(os.path.join(TTS, "kokoro-v1.0.onnx"), os.path.join(TTS, "voices-v1.0.bin"))
voice = sys.argv[1] if len(sys.argv) > 1 else 'am_michael'
L = {
    "h_wneed": "You need a wrench for those bolts. There's one somewhere in your room.",
    "x_harp_super": "Hold A on the harpoon to supercharge it. A supercharged spear fries whatever it hits.",
    "x_harp_tie": "Tie a floor line to the cleat on the turret's post, and you can shoot again. Grab the line where it's stuck in the floor, and bring it to the cleat. It stays live.",
    "x_tractor": "Tractor beam the cart! Point the harpoon at it and pull the trigger. They dropped it.",
    "x_cart_spear": "If they take the cart, spear it. It sets off a little E M P. They'll be stumbling about while you wind it in.",
    "x_leads": "Plug the power leads back into the cart! It won't go off without them. They ripped them out.",
    "x_reel_mode": "Yellow harpoon light means reel spears only. They'll hook a robot, or the cart, but they won't make a live wire. Supercharge it if you need the wire.",
}
only = sys.argv[2:] if len(sys.argv) > 2 else list(L)
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
