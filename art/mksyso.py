import struct, sys
ico = open(sys.argv[1], 'rb').read()
_, typ, count = struct.unpack_from('<HHH', ico, 0)
imgs = []
for i in range(count):
    w, h, cc, res, planes, bpp, size, off = struct.unpack_from('<BBBBHHII', ico, 6 + 16 * i)
    imgs.append((w, h, cc, planes, bpp, ico[off:off + size]))
grp = struct.pack('<HHH', 0, 1, count) + b''.join(
    struct.pack('<BBBBHHIH', w, h, cc, 0, planes, bpp, len(d), i + 1) for i, (w, h, cc, planes, bpp, d) in enumerate(imgs))

def dirhdr(n): return struct.pack('<IIHHHH', 0, 0, 0, 0, 0, n)
# layout: root(2 types) -> type dirs -> name dirs(lang) -> data entries -> blobs
types = [(3, [(i + 1, d[5]) for i, d in enumerate(imgs)]), (14, [(1, grp)])]
root_sz = 16 + 8 * len(types)
off = root_sz
type_off = []
for tid, items in types:
    type_off.append(off); off += 16 + 8 * len(items)
name_off = []
for tid, items in types:
    lst = []
    for _ in items:
        lst.append(off); off += 16 + 8
    name_off.append(lst)
data_off = []
for tid, items in types:
    lst = []
    for _ in items:
        lst.append(off); off += 16
    data_off.append(lst)
blob_off = []
for tid, items in types:
    lst = []
    for _, b in items:
        off = (off + 7) & ~7
        lst.append(off); off += len(b)
    blob_off.append(lst)
total = (off + 7) & ~7
buf = bytearray(total)
buf[0:16] = dirhdr(len(types))
for k, (tid, items) in enumerate(types):
    struct.pack_into('<II', buf, 16 + 8 * k, tid, 0x80000000 | type_off[k])
    struct.pack_into('<IIHHHH', buf, type_off[k], 0, 0, 0, 0, 0, len(items))
    for j, (iid, b) in enumerate(items):
        struct.pack_into('<II', buf, type_off[k] + 16 + 8 * j, iid, 0x80000000 | name_off[k][j])
        struct.pack_into('<IIHHHH', buf, name_off[k][j], 0, 0, 0, 0, 0, 1)
        struct.pack_into('<II', buf, name_off[k][j] + 16, 0x0409, data_off[k][j])
        struct.pack_into('<IIII', buf, data_off[k][j], blob_off[k][j], len(b), 0, 0)
        buf[blob_off[k][j]:blob_off[k][j] + len(b)] = b
relocs = [data_off[k][j] for k in range(len(types)) for j in range(len(types[k][1]))]
hdr_sz = 20 + 40
raw_ptr = hdr_sz
rel_ptr = raw_ptr + len(buf)
sym_ptr = rel_ptr + 10 * len(relocs)
out = bytearray()
out += struct.pack('<HHIIIHH', 0x8664, 1, 0, sym_ptr, 1, 0, 0)
out += struct.pack('<8sIIIIIIHHI', b'.rsrc', 0, 0, len(buf), raw_ptr, rel_ptr, 0, len(relocs), 0, 0x40000040)
out += buf
for r in relocs:
    out += struct.pack('<IIH', r, 0, 3)
out += struct.pack('<8sIhHBB', b'.rsrc', 0, 1, 0, 3, 0)
out += struct.pack('<I', 4)
open(sys.argv[2], 'wb').write(out)
print('syso', len(out), 'bytes,', count, 'icons')
