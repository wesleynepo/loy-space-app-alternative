"""Send READ-ONLY queries (group 03) and print the raw replies.

Usage: probe.py <hex query body> [...]     e.g. probe.py 0304 00 ...  (each arg is a full body, group byte included)
"""
import asyncio
import sys

from bleak import BleakClient, BleakScanner

import protocol as P


async def main(bodies: list[bytes]) -> None:
    dev = await BleakScanner.find_device_by_name("YS524A250818", timeout=15)
    if dev is None:
        sys.exit("panel not found")
    replies: asyncio.Queue[dict] = asyncio.Queue()
    asm = P.Reassembler()
    on = lambda _, d: [replies.put_nowait(P.parse(f)) for f in asm.feed(bytes(d))]
    async with BleakClient(dev, timeout=20) as c:
        await c.start_notify(P.NOTIFY2_UUID, on)
        await c.start_notify(P.NOTIFY_UUID, on)
        await c.write_gatt_char(P.WRITE_UUID, P.device_info(41), response=False)
        await asyncio.wait_for(replies.get(), 5)
        for i, body in enumerate(bodies):
            assert body[0] == 0x03, "probe.py only sends group 03 (query) bodies"
            seq = 200 + i
            await c.write_gatt_char(P.WRITE_UUID, P.build(seq, body), response=False)
            try:
                r = await asyncio.wait_for(replies.get(), 3)
                print(body.hex(), "->", r["body"].hex())
            except asyncio.TimeoutError:
                print(body.hex(), "-> no reply")


asyncio.run(main([bytes.fromhex(a) for a in sys.argv[1:]]))
