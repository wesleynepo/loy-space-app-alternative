"""Send a command to the panel over BLE.

Usage: panel.py image <index> | brightness <level>
"""
import asyncio
import sys

from bleak import BleakClient, BleakScanner

import protocol as P

NAME = "YS524A250818"


async def main(cmd: str, arg: int) -> None:
    dev = await BleakScanner.find_device_by_name(NAME, timeout=15)
    if dev is None:
        sys.exit(f"{NAME} not found (is LOY SPACE closed?)")
    replies: asyncio.Queue[bytes] = asyncio.Queue()
    asm = P.Reassembler()

    def on_notify(_, data: bytearray) -> None:
        for frame in asm.feed(bytes(data)):
            replies.put_nowait(frame)

    async with BleakClient(dev, timeout=20) as client:
        await client.start_notify(P.NOTIFY2_UUID, on_notify)
        await client.start_notify(P.NOTIFY_UUID, on_notify)
        # Mirror the app's opening move: ask for device info before any other command.
        await client.write_gatt_char(P.WRITE_UUID, P.device_info(41), response=False)
        info = P.parse(await asyncio.wait_for(replies.get(), 5))
        print("info ", info["body"][3:60])
        frame = P.show_image(11, arg) if cmd == "image" else P.set_brightness(11, arg)
        print("send ", frame.hex())
        await client.write_gatt_char(P.WRITE_UUID, frame, response=False)
        try:
            reply = P.parse(await asyncio.wait_for(replies.get(), 3))
            print("reply", reply["body"].hex())
        except asyncio.TimeoutError:
            print("no reply within 3s")


asyncio.run(main(sys.argv[1], int(sys.argv[2])))
