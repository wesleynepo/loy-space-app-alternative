"""LOY SPACE / YS-series BLE LED panel framing (reverse-engineered from a PacketLogger capture).

Frame: aa55ffff | len u16le | seq u16le | dir u8 | body... | sum u16le
  len  = number of bytes after the len field (seq .. sum)
  sum  = sum of every preceding byte of the frame, mod 65536
  dir  = 0xc1 app->panel, 0x81 panel->app
App bodies start with (group, cmd), e.g. 02 06 02 00 09. Replies echo the sequence number.
"""
import struct

MAGIC = bytes.fromhex("aa55ffff")
DIR_APP, DIR_PANEL = 0xC1, 0x81

# GATT: write commands to FFF2 (write-without-response), replies arrive as notifications on FFF1.
WRITE_UUID = "0000fff2-0000-1000-8000-00805f9b34fb"
NOTIFY_UUID = "0000fff1-0000-1000-8000-00805f9b34fb"


def build(seq: int, body: bytes, direction: int = DIR_APP) -> bytes:
    head = MAGIC + struct.pack("<HHB", len(body) + 5, seq, direction) + body
    return head + struct.pack("<H", sum(head) & 0xFFFF)


def parse(frame: bytes) -> dict:
    if frame[:4] != MAGIC:
        raise ValueError("bad magic")
    length, seq, direction = struct.unpack_from("<HHB", frame, 4)
    if len(frame) != 6 + length:
        raise ValueError(f"length mismatch: header says {length}, got {len(frame) - 6}")
    (got,) = struct.unpack_from("<H", frame, len(frame) - 2)
    if got != sum(frame[:-2]) & 0xFFFF:
        raise ValueError("bad checksum")
    return {"seq": seq, "dir": direction, "body": frame[9:-2]}


class Reassembler:
    """Frames larger than the BLE write/notify size arrive in several chunks."""

    def __init__(self) -> None:
        self.buf = b""

    def feed(self, chunk: bytes) -> list[bytes]:
        self.buf = chunk if chunk[:4] == MAGIC else self.buf + chunk
        out = []
        while len(self.buf) >= 6:
            total = 6 + struct.unpack_from("<H", self.buf, 4)[0]
            if len(self.buf) < total:
                break
            out.append(self.buf[:total])
            self.buf = self.buf[total:]
        return out


# --- commands seen in the capture ---

NOTIFY2_UUID = "5833ff03-9b8b-5191-6142-22a4536ef123"  # second notify char; the app subscribes to both


def device_info(seq: int) -> bytes:
    """group 03, cmd 0a: the app's first command; the reply carries model, firmware and panel size."""
    return build(seq, bytes.fromhex("030a001b0004000600"))


def show_image(seq: int, index: int) -> bytes:
    """group 02, cmd 36: payload 01 <index> 00 followed by the fixed list 01..3b the app always sends."""
    return build(seq, bytes([0x02, 0x36, 0x3E, 0x01, index, 0x00]) + bytes(range(1, 0x3C)))


def query(seq: int, cmd: int) -> bytes:
    """group 03 read-only query. Replies are 83 <cmd> <len> <payload>; the value is the last byte.
    Seen: 04 power (1=on), 06 brightness, 2f rotation (0-3), 1b size (w,h u16le at payload 1..4)."""
    return build(seq, bytes([0x03, cmd, 0x00]))


def set_power(seq: int, on: bool) -> bytes:
    """group 02, cmd 04: payload 00 <1|0>. From the app's code; confirmed on the panel (see ROADMAP)."""
    return build(seq, bytes([0x02, 0x04, 0x02, 0x00, int(on)]))


def set_rotation(seq: int, quarter_turns: int) -> bytes:
    """group 02, cmd 2f: one byte, 0/1/2/3 for 0/90/180/270 degrees (app code, `setLedRotate`)."""
    return build(seq, bytes([0x02, 0x2F, 0x01, quarter_turns & 3]))


def set_brightness(seq: int, level: int) -> bytes:
    """group 02, cmd 06, payload 00 <level>. Level values seen: 1..9, then 2,3,6,12,14."""
    return build(seq, bytes([0x02, 0x06, 0x02, 0x00, level]))


def delete_slot(seq: int, slot: int) -> bytes:
    """group 02, cmd 08, payload 00 <slot>. What the app sends per program (seen in a capture: slots 0-5, one frame
    each). The panel acks, then pushes an unsolicited `06 36 02 01 NN` frame."""
    return build(seq, bytes([0x02, 0x08, 0x02, 0x00, slot]))


# --- image upload (group 02: start 08, item info, data chunks 09, finish 35) ---

CHUNK = 960
GAMMA = 1.6  # fitted from the app's own upload; the app builds a ((a+.5)/256)^g lookup table


def pack_image(rgb: bytes, w: int = 64, h: int = 64, gamma: float | None = GAMMA) -> bytes:
    """YSTP01 image: 15-byte header + raw 24-bit RGB rows, top-down. Verified against a captured upload."""
    if len(rgb) != w * h * 3:
        raise ValueError(f"expected {w * h * 3} RGB bytes, got {len(rgb)}")
    if gamma:
        table = bytes(int(256 * ((a + 0.5) / 256) ** gamma - 0.5) for a in range(256))
        rgb = rgb.translate(table)
    return b"YSTP01" + struct.pack("<HHHH", w, h, 1, w) + bytes([24]) + rgb


def _tlv(tag: int, *value: int) -> bytes:
    return bytes([tag, len(value), *value])


def _len_tlv(n: int) -> bytes:
    return bytes([n]) if n < 128 else bytes([0x82]) + struct.pack("<H", n)


def make_key(id_pro: int, packed: bytes, ts: int) -> bytes:
    """20-byte slot key: (id_pro + '0000000')[:8] as hex bytes + 16 bytes. The app uses md5 of its program JSON;
    we can't reproduce that, so ours is md5(data + timestamp), which only needs to be unique per upload."""
    import hashlib
    prefix = bytes.fromhex((f"{id_pro:x}" + "0000000")[:8])
    return prefix + hashlib.md5(packed + struct.pack("<I", ts)).digest()


def upload_bodies(slot: int, packed: bytes, key: bytes, ts: int, w: int = 64, h: int = 64) -> list[bytes]:
    """Bodies (seq assigned by the caller) for: start, item info, data chunks, finish.
    Layout from the app's code (tags 08 start, 1c play info, 1d region rect, 12/13 chunk, 35 finish);
    the item-info frame (tags 14 and 11) is copied from the capture and only verified for 64x64 24-bit."""
    hdr = _tlv(0x09, 1) + _tlv(0x0C, slot) + _tlv(0x0D, 0)
    start = (bytes([0x02]) + _tlv(0x08, 0, slot) + _tlv(0x09, 1) + _tlv(0x0C, slot)
             + _tlv(0x1C, 0, 0, 1, 0, 3, 0)  # type_bg, loop mode, count 1, time_sync 3
             + _tlv(0x0D, 0) + _tlv(0x1D, 0, 0, 0, 0, *struct.pack("<HH", w, h), 0))  # region 0, rect x y w h bg
    item = bytes([0x02]) + hdr + _tlv(0x0E, 0) + _tlv(0x14, 1, 0x0D, 3) + _tlv(0x11, 0, 1, 0, 10)
    total = -(-len(packed) // CHUNK)
    chunks = []
    for i in range(total):
        data = packed[i * CHUNK : (i + 1) * CHUNK]
        chunks.append(bytes([0x02]) + hdr + _tlv(0x0E, 0)
                      + bytes([0x12, 7]) + struct.pack("<HHH", total, i, CHUNK) + b"\x00"
                      + bytes([0x13]) + _len_tlv(len(data)) + data)
    finish = bytes([0x02, 0x35, 0x19, slot]) + key + struct.pack("<I", ts)
    return [start, item, *chunks, finish]
