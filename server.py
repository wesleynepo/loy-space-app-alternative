"""Local web UI for the LED panel. Serves web/index.html and a small JSON API on 127.0.0.1.

Usage: server.py [port]     (default 8765; open http://127.0.0.1:8765)
"""
import asyncio
import base64
import hashlib
import itertools
import json
import mimetypes
import os
import signal
import sys
import threading
import time
import urllib.request
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
from urllib.parse import urlsplit

from bleak import BleakClient, BleakScanner

import protocol as P
from pngutil import write_png

NAME = "YS524A250818"
WEB = Path(__file__).parent / "web"
CACHE = Path(__file__).parent / ".cache"
# LOY SPACE (iOS app on Apple silicon) keeps its per-panel slot list and local images here.
APP_DOC = Path(os.environ.get("LOY_DOC", "~/Library/Containers/com.yskd.loywf/Data/Library/Pandora/apps/__UNI__F2139F6/doc")).expanduser()
PROGRAM = APP_DOC / "524A250818_program_2_64_64.json"
UPLOADS = Path(__file__).parent / ".uploads"  # our own uploads: previews + slot keys
UPLOAD_INDEX = UPLOADS / "index.json"
WRITE_MAX = 180  # largest BLE write the app uses (MTU 185 minus overhead)
TOTAL_SLOTS = 60  # size of the panel's key table; the app refuses to send more programs than this
FIRST_FREE_SLOT, LAST_SLOT = 6, 9  # slots 0-5 hold the app's content and are never overwritten


class PanelLink:
    """One persistent BLE connection, driven from a private asyncio loop thread."""

    def __init__(self) -> None:
        self.loop = asyncio.new_event_loop()
        threading.Thread(target=self.loop.run_forever, daemon=True).start()
        self.client: BleakClient | None = None
        self.info = ""
        self.log: list[str] = []
        self.seq = itertools.count(50)
        self.replies: asyncio.Queue[dict] = asyncio.Queue()
        self.asm = P.Reassembler()
        self.lock = asyncio.Lock()

    def call(self, coro, timeout: float = 30):
        return asyncio.run_coroutine_threadsafe(coro, self.loop).result(timeout)

    def note(self, line: str) -> None:
        self.log = (self.log + [f"{time.strftime('%H:%M:%S')} {line}"])[-200:]

    def _on_notify(self, _, data: bytearray) -> None:
        for frame in self.asm.feed(bytes(data)):
            self.note(f"<- {frame.hex()[:120]}")
            self.replies.put_nowait(P.parse(frame))

    @property
    def connected(self) -> bool:
        return bool(self.client and self.client.is_connected)

    async def connect(self) -> None:
        async with self.lock:
            if self.connected:
                return
            # A panel still holding a stale link (e.g. after a killed server) is invisible for a few seconds.
            for attempt in range(3):
                dev = await BleakScanner.find_device_by_name(NAME, timeout=10)
                if dev:
                    break
                self.note(f"panel not seen (attempt {attempt + 1}/3)")
            else:
                raise RuntimeError(f"{NAME} not found - close LOY SPACE / power-cycle the panel")
            self.client = BleakClient(dev, timeout=20)
            await self.client.connect()
            await self.client.start_notify(P.NOTIFY2_UUID, self._on_notify)
            await self.client.start_notify(P.NOTIFY_UUID, self._on_notify)
            reply = await self._send(P.device_info(41), 41)  # the app's opening move
            text = bytes(reply["body"][3:]).decode("ascii", errors="ignore")
            self.info = ", ".join(text.split(",")[:3])
            self.note("connected")

    async def disconnect(self) -> None:
        async with self.lock:
            if self.client:
                await self.client.disconnect()
            self.note("disconnected")

    async def _send(self, frame: bytes, seq: int) -> dict:
        while not self.replies.empty():
            self.replies.get_nowait()
        self.note(f"-> {frame.hex()[:120]}")
        for off in range(0, len(frame), WRITE_MAX):  # the app splits big frames into <=180-byte writes
            await self.client.write_gatt_char(P.WRITE_UUID, frame[off : off + WRITE_MAX], response=False)
            if len(frame) > WRITE_MAX:
                await asyncio.sleep(0.02)
        deadline = time.monotonic() + 3
        while (left := deadline - time.monotonic()) > 0:
            reply = await asyncio.wait_for(self.replies.get(), left)
            if reply["seq"] == seq:
                return reply
        raise RuntimeError("no reply from panel")

    async def command(self, build, seq: int) -> str:
        async with self.lock:
            if not self.connected:
                raise RuntimeError("not connected")
            reply = await self._send(build(seq), seq)
            return reply["body"].hex()

    async def read_state(self) -> dict:
        """Power, brightness and rotation via read-only queries."""
        async with self.lock:
            if not self.connected:
                raise RuntimeError("not connected")
            out = {}
            for name, cmd in (("power", 0x04), ("brightness", 0x06), ("rotation", 0x2F)):
                seq = next(self.seq)
                out[name] = (await self._send(P.query(seq, cmd), seq))["body"][-1]
        return {"power": bool(out["power"]), "brightness": out["brightness"], "rotation": out["rotation"] * 90}

    async def upload(self, slot: int, rgb: bytes) -> str:
        """Send a 64x64 RGB picture to a slot (start, item info, chunks, finish), one acked frame at a time."""
        packed = P.pack_image(rgb)
        ts = int(time.time())
        key = P.make_key(slot + 1, packed, ts)
        async with self.lock:
            if not self.connected:
                raise RuntimeError("not connected")
            bodies = P.upload_bodies(slot, packed, key, ts)
            for i, body in enumerate(bodies):
                seq = next(self.seq)
                reply = await self._send(P.build(seq, body), seq)
                if reply["body"] != b"\x82\x00":
                    raise RuntimeError(f"panel rejected frame {i + 1}/{len(bodies)}: {reply['body'].hex()}")
            # The panel falls back to slot 0 after an upload; show the new slot, as the app does.
            await self._send(P.show_image(11, slot), 11)
        self.note(f"uploaded slot {slot} ({len(bodies)} frames)")
        UPLOADS.mkdir(exist_ok=True)
        write_png(str(UPLOADS / f"slot{slot}.png"), 64, 64, rgb)
        index = json.loads(UPLOAD_INDEX.read_text()) if UPLOAD_INDEX.exists() else {}
        index[str(slot)] = {"key": key.hex(), "ts": ts}
        UPLOAD_INDEX.write_text(json.dumps(index))
        return key.hex()

    async def delete(self, slot: int) -> None:
        """Delete one of our uploads from the panel and forget its preview."""
        async with self.lock:
            if not self.connected:
                raise RuntimeError("not connected")
            seq = next(self.seq)
            reply = await self._send(P.delete_slot(seq, slot), seq)
            if reply["body"] != b"\x82\x00":
                raise RuntimeError(f"panel rejected delete: {reply['body'].hex()}")
        self.note(f"deleted slot {slot}")
        (UPLOADS / f"slot{slot}.png").unlink(missing_ok=True)
        if UPLOAD_INDEX.exists():
            index = json.loads(UPLOAD_INDEX.read_text())
            index.pop(str(slot), None)
            UPLOAD_INDEX.write_text(json.dumps(index))

    async def panel_state(self) -> tuple[int, dict[int, str]]:
        """Current slot (03 36) and the key stored in each slot (03 35), as the app asks for them."""
        async with self.lock:
            if not self.connected:
                raise RuntimeError("not connected")
            seq = next(self.seq)
            cur = (await self._send(P.build(seq, bytes([0x03, 0x36, 0x00])), seq))["body"]
            lists = []
            for first in range(0, TOTAL_SLOTS, 10):  # the app reads the table ten slots at a time
                seq = next(self.seq)
                body = bytes([0x03, 0x35, 0x0A]) + bytes(range(first, first + 10))
                lists.append((await self._send(P.build(seq, body), seq))["body"])
        keys = {}
        for lst in lists:
            for off in range(4, 4 + lst[3], 21):  # entry = slot id + 20-byte key; all zeros means empty
                key = lst[off + 1 : off + 21]
                if any(key):
                    keys[lst[off]] = key.hex()
        return cur[4], keys

    def status(self) -> dict:
        return {"connected": self.connected, "info": self.info, "log": self.log}


link = PanelLink()


def load_program() -> list[dict]:
    return json.loads(PROGRAM.read_text()) if PROGRAM.exists() else []


def load_uploads() -> dict[int, dict]:
    return {int(k): v for k, v in json.loads(UPLOAD_INDEX.read_text()).items()} if UPLOAD_INDEX.exists() else {}


def all_slots() -> list[dict]:
    """The app's program list, plus our own uploads in slots the app doesn't know about."""
    slots = [{"slot": i, "type": e.get("type"), "key": e.get("key")} for i, e in enumerate(load_program())]
    uploads = load_uploads()
    for slot in sorted(uploads):
        if slot >= len(slots):
            slots.append({"slot": slot, "type": "upload", "key": uploads[slot]["key"]})
    return slots


def slot_image(slot: int) -> tuple[bytes, str]:
    """Bytes and content type for a slot's picture: CDN download (cached) or a file in the app's folder."""
    program = load_program()
    if slot >= len(program):
        return (UPLOADS / f"slot{slot}.png").read_bytes(), "image/png"
    path = program[slot]["path"]
    if path.startswith("http"):
        url = urlsplit(path)
        if url.scheme != "https" or not (url.hostname or "").endswith(".popled.cn"):
            raise ValueError("unexpected image host")
        CACHE.mkdir(exist_ok=True)
        cached = CACHE / (hashlib.sha1(path.encode()).hexdigest() + Path(url.path).suffix)
        if not cached.exists():
            with urllib.request.urlopen(path, timeout=10) as r:
                cached.write_bytes(r.read())
    else:
        cached = (APP_DOC / path.removeprefix("_doc/")).resolve()
        if APP_DOC.resolve() not in cached.parents:
            raise ValueError("path outside the app folder")
    return cached.read_bytes(), mimetypes.guess_type(cached.name)[0] or "application/octet-stream"


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *_) -> None:
        pass

    def _local_only(self) -> bool:
        # Blocks DNS-rebinding and cross-site requests from other web pages.
        host = (self.headers.get("Host") or "").split(":")[0]
        origin = self.headers.get("Origin")
        return host in ("127.0.0.1", "localhost") and (
            origin is None or urlsplit(origin).hostname in ("127.0.0.1", "localhost")
        )

    def _json(self, code: int, obj: dict) -> None:
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self) -> None:
        if not self._local_only():
            return self._json(403, {"error": "forbidden"})
        path = urlsplit(self.path).path
        if path == "/api/status":
            return self._json(200, link.status())
        if path == "/api/state":
            try:
                return self._json(200, link.call(link.read_state()))
            except Exception as e:
                return self._json(400, {"error": str(e) or type(e).__name__})
        if path == "/api/slots":
            slots = [{**s, "src": f"/img/{s['slot']}?k={s['key'][-8:]}", "match": None} for s in all_slots()]
            current, error, used = None, None, None
            if link.connected:
                try:
                    current, keys = link.call(link.panel_state())
                    used = len(keys)
                    for s in slots:
                        s["match"] = keys.get(s["slot"]) == s["key"]
                except Exception as e:
                    error = str(e) or type(e).__name__
            return self._json(200, {"slots": slots, "current": current, "error": error, "used": used, "total": TOTAL_SLOTS})
        if path.startswith("/img/"):
            try:
                body, ctype = slot_image(int(path[5:]))
            except Exception as e:
                return self._json(404, {"error": str(e)})
            self.send_response(200)
            self.send_header("Content-Type", ctype)
            self.send_header("Content-Length", str(len(body)))
            self.send_header("Cache-Control", "max-age=3600")
            self.end_headers()
            self.wfile.write(body)
            return
        if path == "/":
            body = (WEB / "index.html").read_bytes()
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
            return
        self._json(404, {"error": "not found"})

    def do_POST(self) -> None:
        if not self._local_only() or self.headers.get("Content-Type") != "application/json":
            return self._json(403, {"error": "forbidden"})
        try:
            args = json.loads(self.rfile.read(int(self.headers.get("Content-Length", 0))) or b"{}")
            path = urlsplit(self.path).path
            if path == "/api/connect":
                link.call(link.connect(), timeout=60)
            elif path == "/api/disconnect":
                link.call(link.disconnect())
            elif path == "/api/image":
                index = int(args["index"])
                if not 0 <= index <= 59:
                    raise ValueError("index out of range")
                link.call(link.command(lambda s: P.show_image(11, index), 11))  # the app always used seq 11
            elif path == "/api/upload":
                if args.get("slot") is None:  # pick the first slot the panel reports as empty
                    taken = link.call(link.panel_state())[1]
                    slot = next((s for s in range(FIRST_FREE_SLOT, LAST_SLOT + 1) if s not in taken), None)
                    if slot is None:
                        raise ValueError(f"slots {FIRST_FREE_SLOT}-{LAST_SLOT} are all in use - delete one first")
                else:
                    slot = int(args["slot"])
                if not FIRST_FREE_SLOT <= slot <= LAST_SLOT:
                    raise ValueError(f"uploads go to slots {FIRST_FREE_SLOT}-{LAST_SLOT} only")
                rgb = base64.b64decode(args["rgb"], validate=True)
                if len(rgb) != 64 * 64 * 3:
                    raise ValueError("expected 64x64 RGB (12288 bytes)")
                link.call(link.upload(slot, rgb), timeout=90)
                return self._json(200, {**link.status(), "slot": slot})
            elif path == "/api/delete":
                slot = int(args["slot"])
                if not FIRST_FREE_SLOT <= slot <= LAST_SLOT:
                    raise ValueError(f"deletes are limited to slots {FIRST_FREE_SLOT}-{LAST_SLOT}")
                link.call(link.delete(slot))
            elif path == "/api/power":
                on = bool(args["on"])
                link.call(link.command(lambda s: P.set_power(s, on), next(link.seq)))
            elif path == "/api/rotation":
                degrees = int(args["degrees"])
                if degrees not in (0, 90, 180, 270):
                    raise ValueError("degrees must be 0, 90, 180 or 270")
                link.call(link.command(lambda s: P.set_rotation(s, degrees // 90), next(link.seq)))
            elif path == "/api/brightness":
                level = int(args["level"])
                if not 1 <= level <= 15:
                    raise ValueError("level must be 1-15 (0 is avoided: the app maps it to 15)")
                link.call(link.command(lambda s: P.set_brightness(s, level), next(link.seq)))
            else:
                return self._json(404, {"error": "not found"})
            self._json(200, link.status())
        except Exception as e:  # surfaced in the UI
            link.note(f"error: {e}")
            self._json(400, {"error": str(e) or type(e).__name__, **link.status()})


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8765
    print(f"http://127.0.0.1:{port}")
    signal.signal(signal.SIGTERM, lambda *_: sys.exit(0))  # so `pkill` still releases the panel
    try:
        ThreadingHTTPServer(("127.0.0.1", port), Handler).serve_forever()
    finally:
        if link.connected:
            link.call(link.disconnect(), timeout=5)
