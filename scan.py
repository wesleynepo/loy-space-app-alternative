"""Scan for nearby BLE devices and print name, address, RSSI, advertised services.

Usage: scan.py [seconds] [name-substring ...]
Devices whose name contains a substring (case-insensitive) are marked with ***.
"""
import asyncio
import sys

from bleak import BleakScanner

HINTS = ["ys", "loy", "h66", "led", "bt"]


async def main(seconds: float, hints: list[str]) -> None:
    found = await BleakScanner.discover(timeout=seconds, return_adv=True)
    rows = sorted(found.values(), key=lambda r: r[1].rssi, reverse=True)
    for dev, adv in rows:
        name = adv.local_name or dev.name
        hit = name and any(h in name.lower() for h in hints)
        print(f"{'***' if hit else '   '} {adv.rssi:>4} dBm  {dev.address}  name={name!r}")
        if adv.service_uuids:
            print(f"          services={adv.service_uuids}")
        if adv.manufacturer_data:
            for cid, data in adv.manufacturer_data.items():
                print(f"          mfr 0x{cid:04x}={data.hex()}")
    print(f"{len(rows)} devices")


args = sys.argv[1:]
secs = float(args[0]) if args else 15.0
asyncio.run(main(secs, [h.lower() for h in args[1:]] or HINTS))
