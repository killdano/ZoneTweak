# The radio speaker's recorded prompt: an automated voice, garbled like a worn tape on a
# cheap speaker, telling you to pick up the handset.
import os, sys, numpy as np, soundfile as sf
from scipy import signal
from kokoro_onnx import Kokoro
TTS = os.environ.get('KOKORO', 'tts')  # folder with kokoro-v1.0.onnx and voices-v1.0.bin
OUT = os.path.dirname(os.path.abspath(__file__))
voice = sys.argv[1] if len(sys.argv) > 1 else 'bf_emma'
L = {
    "n_pickup_1": "Incoming message. Pick up the phone.",
    "n_pickup_2": "You have a message. Pick up the handset.",
    "n_urgent": "Urgent message. Pick up the phone. Now.",
}
k = Kokoro(os.path.join(TTS, "kokoro-v1.0.onnx"), os.path.join(TTS, "voices-v1.0.bin"))
rng = np.random.default_rng(7)
SR = 16000
NOISE = 0.8  # how much static, hum and crackle (1 was too much)

def trim(a, sr):
    thr = 0.01 * np.max(np.abs(a))
    idx = np.nonzero(np.abs(a) > thr)[0]
    return a[max(0, idx[0] - int(0.02 * sr)): idx[-1] + int(0.08 * sr)]

def chime(sr):
    out = []
    for f in (880, 660):
        t = np.arange(int(0.16 * sr)) / sr
        env = np.minimum(1, t / 0.01) * np.exp(-t * 9)
        out.append(0.5 * env * (np.sin(2 * np.pi * f * t) + 0.3 * np.sin(4 * np.pi * f * t)))
        out.append(np.zeros(int(0.04 * sr)))
    out.append(np.zeros(int(0.12 * sr)))
    return np.concatenate(out)

def burst(sr, secs, lvl):
    # a squelch of radio static
    n = int(secs * sr)
    x = rng.standard_normal(n)
    env = np.sin(np.linspace(0, np.pi, n)) ** 0.5
    return NOISE * lvl * x * env

def garble(a, sr):
    # wow and flutter: a stretched old tape, read at a wobbling speed
    n = len(a)
    t = np.arange(n) / sr
    speed = 1 + 0.03 * np.sin(2 * np.pi * 0.6 * t + rng.uniform(0, 6)) + 0.01 * np.sin(2 * np.pi * 7.5 * t)
    pos = np.cumsum(speed)
    pos = pos[pos < n - 1]
    a = np.interp(pos, np.arange(n), a)
    # squelches of static at the start, the end, and between some words
    a = np.concatenate([burst(sr, 0.18, 0.5), a, burst(sr, 0.25, 0.6)])
    # through a bad telephone line: 8 kHz, band-passed hard
    a = signal.resample_poly(a, 1, 2)
    a = signal.resample_poly(a, 2, 1)
    b, aa = signal.butter(4, [450, 2600], btype='band', fs=sr)
    a = signal.lfilter(b, aa, a)
    a = a / (np.max(np.abs(a)) + 1e-9)
    t = np.arange(len(a)) / sr
    # mains hum riding on it, and a ring-mod wobble that makes it metallic
    a = a * (0.7 + 0.3 * np.sin(2 * np.pi * 47 * t))
    a = a + NOISE * 0.12 * np.sign(np.sin(2 * np.pi * 120 * t)) * (0.6 + 0.4 * np.sin(2 * np.pi * 0.7 * t))
    # drop-outs: the tape's worn through in patches
    env = np.ones(len(a))
    p = int(0.3 * sr)
    while p < len(a):
        w = int(rng.uniform(0.03, 0.12) * sr)
        lvl = rng.uniform(0.02, 0.25)
        ramp = np.linspace(1, lvl, max(2, min(60, w // 3)))
        seg = np.concatenate([ramp, np.full(max(0, w - 2 * len(ramp)), lvl), ramp[::-1]])
        e = env[p:p + len(seg)]
        env[p:p + len(seg)] = np.minimum(e, seg[:len(e)])
        if rng.random() < 0.5:
            # static fills the hole
            nb = min(len(seg), len(a) - p)
            a[p:p + nb] += burst(sr, nb / sr, 0.7)[:nb]
        p += int(rng.uniform(0.25, 0.6) * sr)
    a = a * env
    # an overdriven little speaker: clipped hard (lopsided), crushed and sampled down
    a = np.tanh(6 * a + 0.15) - np.tanh(0.15)
    a = a / (np.max(np.abs(a)) + 1e-9)
    q = 2 ** 4
    a = np.round(a * q) / q
    hold = 3  # about 5 kHz: gritty aliasing
    a = np.repeat(a[::hold], hold)[:len(a)]
    # a bed of static that swells and fades, and crackle
    swell = 0.5 + 0.5 * np.sin(2 * np.pi * 0.9 * np.arange(len(a)) / sr + 1) ** 2
    a = a + NOISE * 0.16 * swell * rng.standard_normal(len(a))
    cr = np.zeros(len(a))
    idx = rng.integers(0, len(a), size=int(len(a) / sr * 45))
    cr[idx] = NOISE * rng.uniform(-1, 1, size=len(idx))
    cr = signal.lfilter([1], [1, -0.7], cr)
    a = a + cr
    b, aa = signal.butter(2, [250, 3600], btype='band', fs=sr)
    a = signal.lfilter(b, aa, a)
    return 0.9 * a / (np.max(np.abs(a)) + 1e-9)

for key, text in L.items():
    v, sr = k.create(text, voice=voice, speed=0.98, lang="en-us")
    v = trim(v, sr)
    v = signal.resample_poly(v, SR, sr) if sr != SR else v
    v = v / np.max(np.abs(v))
    a = np.concatenate([chime(SR), v])
    a = garble(a, SR)
    sf.write(f'{key}.wav', a, SR)
    (np.clip(a, -1, 1) * 32000).astype('<i2').tofile(os.path.join(OUT, key + '.raw'))
    print(key, round(len(a) / SR, 2), 's')
