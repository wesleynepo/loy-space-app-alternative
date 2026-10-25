"""Extract ATT traffic from an Apple PacketLogger .pklg capture.

Record layout: u32 LE length (of the rest), u32 LE seconds, u32 LE usecs, u8 type, payload.
Type 0x02 = ACL host->controller, 0x03 = ACL controller->host, 0xfc = text note.

Usage: parse_pklg.py <file.pklg> [--all]
"""
import struct
import sys

ATT_OPS = {
    0x01: "ErrorRsp", 0x02: "MtuReq", 0x03: "MtuRsp", 0x04: "FindInfoReq", 0x05: "FindInfoRsp",
    0x08: "ReadByTypeReq", 0x09: "ReadByTypeRsp", 0x0A: "ReadReq", 0x0B: "ReadRsp",
    0x10: "ReadByGroupReq", 0x11: "ReadByGroupRsp", 0x12: "WriteReq", 0x13: "WriteRsp",
    0x1B: "Notify", 0x1D: "Indicate", 0x52: "WriteCmd",
}


def records(data: bytes):
    pos = 0
    while pos + 4 <= len(data):
        (length,) = struct.unpack_from("<I", data, pos)
        body = data[pos + 4 : pos + 4 + length]
        pos += 4 + length
        if len(body) < 9:
            break
        sec, usec, rtype = struct.unpack_from("<IIB", body)
        yield sec + usec / 1e6, rtype, body[9:]


def main(path: str, show_all: bool) -> None:
    data = open(path, "rb").read()
    partial: dict[tuple[int, int], bytearray] = {}
    t0 = None
    for ts, rtype, payload in records(data):
        if rtype == 0xFC:
            print("note:", payload.decode(errors="replace"))
            continue
        if rtype not in (0x02, 0x03) or len(payload) < 4:
            continue
        t0 = t0 or ts
        hdr, _alen = struct.unpack_from("<HH", payload)
        handle, pb = hdr & 0x0FFF, (hdr >> 12) & 0x3
        frag = payload[4:]
        key = (rtype, handle)
        if pb == 0x1 and key in partial:  # continuation
            partial[key] += frag
        else:
            partial[key] = bytearray(frag)
        buf = partial[key]
        if len(buf) < 4:
            continue
        l2len, cid = struct.unpack_from("<HH", buf)
        if len(buf) < 4 + l2len:
            continue  # wait for more fragments
        att = bytes(buf[4 : 4 + l2len])
        del partial[key]
        if cid != 0x0004 or not att:
            continue
        op = att[0]
        name = ATT_OPS.get(op, f"op{op:02x}")
        if not show_all and op not in (0x12, 0x52, 0x1B, 0x1D, 0x13):
            continue
        direction = "app->panel" if rtype == 0x02 else "panel->app"
        h = f" handle=0x{att[1] | att[2] << 8:04x}" if len(att) >= 3 and op in (0x12, 0x52, 0x1B, 0x1D, 0x0A) else ""
        val = att[3:] if h else att[1:]
        print(f"+{ts - t0:8.3f}s {direction} {name}{h} len={len(val)} {val.hex()}")


if __name__ == "__main__":
    main(sys.argv[1], "--all" in sys.argv)
