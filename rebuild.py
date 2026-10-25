"""Rebuild the image uploaded in a capture (02 09 chunks, YSTP01 header, 64x64 RGB) into PNGs.

Usage: rebuild.py <att.txt from parse_pklg.py> <out-prefix>
"""
import re
import struct
import sys

import protocol as P
from pngutil import write_png as png


asm, chunks = P.Reassembler(), {}
for line in open(sys.argv[1]):
    m = re.search(r"app->panel WriteCmd .* len=\d+ ([0-9a-f]+)", line)
    for frame in asm.feed(bytes.fromhex(m.group(1))) if m else []:
        body = P.parse(frame)["body"]
        if body[:2] == b"\x02\x09" and body[13:15] == b"\x12\x07":
            idx = body[17] | body[18] << 8
            chunks[idx] = body[26:]  # 2 cmd + 24 TLV header bytes precede the data
data = b"".join(chunks[i] for i in sorted(chunks))
print(len(chunks), "chunks,", len(data), "bytes, magic", data[:6], "w,h", struct.unpack_from("<HH", data, 6), "bpp", data[14])
pix = data[15:15 + 64 * 64 * 3]
png(sys.argv[2] + "_rgb.png", 64, 64, pix)
png(sys.argv[2] + "_bgr.png", 64, 64, b"".join(pix[i + 2:i + 3] + pix[i + 1:i + 2] + pix[i:i + 1] for i in range(0, len(pix), 3)))
