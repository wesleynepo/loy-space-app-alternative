# BLE bridge spec (what a Bend rewrite needs from native code)

Everything in `protocol.py` and `server.py` is pure bytes / HTTP / files except the Bluetooth calls below. Today they all go through `bleak`; a Bend rewrite needs a native library (CoreBluetooth on macOS) exposing the same small surface. Derived from the actual `bleak` calls in `server.py`, `panel.py`, `probe.py`, `scan.py`, `gatt.py`.

## Status

Spike done in `bend/ble/`: `loyble.m` implements open / write / recv / drain / close, `ble.c` exposes them to Bend as effects, and `bend/ble_hello.bend` completes the device-info exchange with the real panel. Not yet done: a generic (non-LOY) API, a replay/fake transport, and the upload/delete flows.

## Required for the server (`server.py`, `panel.py`, `probe.py`)

| # | Operation | Used for | Current behaviour to match |
|---|---|---|---|
| 1 | **Find peripheral by exact advertised name** `YS524A250818` | connect | scan up to 10 s, retry 3 times (a panel with a stale link is invisible for a few seconds). macOS gives UUIDs, not MAC addresses. |
| 2 | **Connect / disconnect / is_connected** | session | connect timeout 20 s. One connection at a time: the panel refuses a second. Must disconnect on SIGTERM/exit. |
| 3 | **Subscribe to notifications** on two characteristics | replies | `0000fff1-0000-1000-8000-00805f9b34fb` and `5833ff03-9b8b-5191-6142-22a4536ef123`. Both are subscribed, as the app does; replies arrive on FFF1. |
| 4 | **Write without response** to `0000fff2-0000-1000-8000-00805f9b34fb` | all commands | Chunks of at most 180 bytes; 20 ms pause between chunks of a larger frame. Frames reach ~1000 bytes (960-byte image chunk + header), so 6 writes per data frame. |
| 5 | **Receive notification chunks in order** | replies | Chunks are delivered raw; reassembly stays in Bend (`Reassembler` in `protocol.py`). A 214-byte reply arrives as two notifications. Needs a blocking `recv(timeout)` or a queue. |

Plus the behaviours around them:
- **Write flow control.** Write-without-response on CoreBluetooth must wait for `canSendWriteWithoutResponse` / `peripheralIsReady(toSendWriteWithoutResponse:)`. `bleak` hides this today; the bridge must not.
- **Threading.** CoreBluetooth needs its own dispatch queue / run loop. The bridge should run it on an internal thread and expose blocking calls, so Bend never handles callbacks.
- **Timeouts.** Per-frame ack wait is 3 s; a whole upload is about 16 acked frames (start, item, 13 chunks, finish), typically a few seconds.
- **No pairing / bonding / encryption** is used; a plain connect works today.
- **macOS Bluetooth permission.** The process (or the terminal that launches it) needs Bluetooth access. To verify for a non-`bleak` binary.

Proposed C surface (blocking, 7 calls):
```
ble_find(name, timeout_ms)            -> handle | error
ble_connect(handle, timeout_ms)       -> ok | error      (also discovers FFF1 / FFF2 / 5833ff03 and subscribes)
ble_write(frame_bytes)                -> ok | error      (splits into <=180 B writes, honours flow control)
ble_recv(timeout_ms)                  -> bytes | timeout
ble_is_connected()                    -> bool
ble_disconnect()                      -> ok
ble_drain()                           -> ()              (drop queued notifications before a send, as _send does)
```

## Only for the investigation tools (not needed by the server)

- `scan.py`: discover all peripherals with name, address, RSSI, advertised service UUIDs.
- `gatt.py`: list services/characteristics, read values, MTU size.

## Acceptance test for the bridge

1. find + connect + subscribe both characteristics.
2. Send `device_info(41)`; expect a reply with seq 41 whose body starts `830a` and contains `524A250818`.
3. Send `03 35 0a 00..09` (key table); expect a body of 214 bytes assembled from two notifications.
4. Upload then delete a picture in a free slot (6 or higher) and read the key table back, as `server.py` does.
5. Kill the process mid-session and confirm the panel is visible again within a few seconds.

## Other gaps for the rewrite (not BLE)

- MD5 for the slot key: unconfirmed in `bend-kit-crypto` (only `sha256` is named in its README); otherwise write it.
- How Bend 2 builds `effs/*.c`: whether it can compile Objective-C or link macOS frameworks is not documented anywhere I found.
