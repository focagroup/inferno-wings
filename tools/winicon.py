#!/usr/bin/env python3
"""Turns icon.png into rsrc_windows_amd64.syso, a COFF object holding a Windows icon resource.

Go links any *.syso in the package directory into the build, so the Windows .exe gets this icon
(Explorer, taskbar, Start menu). Linux/arm builds ignore it because of the _windows_amd64 suffix.

Usage: python3 tools/winicon.py [icon.png] [out.syso]      (needs Pillow)
"""
import io
import struct
import sys

from PIL import Image

SIZES = [16, 24, 32, 48, 64, 128, 256]
RT_ICON, RT_GROUP_ICON, LANG_EN_US = 3, 14, 0x0409


def icon_images(png_path):
    """Return [(width, height, bitcount, bytes)] for every size, each stored as PNG (Vista+)."""
    src = Image.open(png_path).convert('RGBA')
    out = []
    for s in SIZES:
        buf = io.BytesIO()
        src.resize((s, s), Image.LANCZOS).save(buf, 'PNG', optimize=True)
        out.append((s, s, 32, buf.getvalue()))
    return out


def build_rsrc(images):
    """Build the .rsrc section bytes plus the offsets of the RVA fields that need relocating."""
    n = len(images)
    grp = struct.pack('<HHH', 0, 1, n)
    for i, (w, h, bpp, data) in enumerate(images):
        grp += struct.pack('<BBBBHHIH', w & 0xFF, h & 0xFF, 0, 0, 1, bpp, len(data), i + 1)
    blobs = [d for (_, _, _, d) in images] + [grp]   # data for icon ids 1..n, then the group

    DIR, ENT, DATA = 16, 8, 16
    # layout: root dir | RT_ICON dir | RT_GROUP_ICON dir | n+1 language dirs | n+1 data entries | blobs
    root = 0
    icon_dir = root + DIR + 2 * ENT
    group_dir = icon_dir + DIR + n * ENT
    lang_dirs = group_dir + DIR + 1 * ENT
    data_entries = lang_dirs + (n + 1) * (DIR + ENT)
    blob_off = data_entries + (n + 1) * DATA
    offsets = []
    for b in blobs:
        blob_off = (blob_off + 7) & ~7
        offsets.append(blob_off)
        blob_off += len(b)
    total = blob_off

    sec = bytearray(total)
    relocs = []

    def put_dir(at, entries):
        struct.pack_into('<IIHHHH', sec, at, 0, 0, 0, 0, 0, len(entries))
        for k, (ident, target, is_dir) in enumerate(entries):
            struct.pack_into('<II', sec, at + DIR + k * ENT, ident, target | (0x80000000 if is_dir else 0))

    put_dir(root, [(RT_ICON, icon_dir, True), (RT_GROUP_ICON, group_dir, True)])
    put_dir(icon_dir, [(i + 1, lang_dirs + i * (DIR + ENT), True) for i in range(n)])
    put_dir(group_dir, [(1, lang_dirs + n * (DIR + ENT), True)])
    for i in range(n + 1):
        put_dir(lang_dirs + i * (DIR + ENT), [(LANG_EN_US, data_entries + i * DATA, False)])
        de = data_entries + i * DATA
        struct.pack_into('<IIII', sec, de, offsets[i], len(blobs[i]), 0, 0)
        relocs.append(de)            # OffsetToData is an RVA: the linker adds the section's address
        sec[offsets[i]:offsets[i] + len(blobs[i])] = blobs[i]
    return bytes(sec), relocs


def coff(section, relocs):
    IMAGE_FILE_MACHINE_AMD64, IMAGE_REL_AMD64_ADDR32NB = 0x8664, 3
    hdr, sh = 20, 40
    raw_at = hdr + sh
    rel_at = raw_at + len(section)
    sym_at = rel_at + 10 * len(relocs)
    out = struct.pack('<HHIIIHH', IMAGE_FILE_MACHINE_AMD64, 1, 0, sym_at, 1, 0, 0)
    out += struct.pack('<8sIIIIIIHHI', b'.rsrc', 0, 0, len(section), raw_at, rel_at, 0, len(relocs), 0,
                       0x40000040)  # INITIALIZED_DATA | MEM_READ
    out += section
    for r in relocs:
        out += struct.pack('<IIH', r, 0, IMAGE_REL_AMD64_ADDR32NB)
    out += struct.pack('<8sIhHBB', b'.rsrc', 0, 1, 0, 3, 0)   # symbol 0: the section, static
    out += struct.pack('<I', 4)                                 # empty string table
    return out


if __name__ == '__main__':
    png = sys.argv[1] if len(sys.argv) > 1 else 'icon.png'
    dst = sys.argv[2] if len(sys.argv) > 2 else 'rsrc_windows_amd64.syso'
    sec, rel = build_rsrc(icon_images(png))
    with open(dst, 'wb') as f:
        f.write(coff(sec, rel))
    print(f'{dst}: {len(SIZES)} icon sizes, {len(sec)} bytes of resources')
