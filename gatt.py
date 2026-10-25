"""Connect to a BLE device by address and dump its GATT table (read-only).

Usage: gatt.py <address-or-uuid>
"""
import asyncio
import sys

from bleak import BleakClient


async def main(address: str) -> None:
    async with BleakClient(address, timeout=20) as client:
        print(f"connected={client.is_connected} mtu={client.mtu_size}")
        for svc in client.services:
            print(f"\nservice {svc.uuid}  ({svc.description})")
            for ch in svc.characteristics:
                line = f"  char {ch.uuid}  props={','.join(ch.properties)}"
                if "read" in ch.properties:
                    try:
                        line += f"  value={bytes(await client.read_gatt_char(ch)).hex()}"
                    except Exception as e:
                        line += f"  read-error={e}"
                print(line)


asyncio.run(main(sys.argv[1]))
