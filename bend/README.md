# Bend port (work in progress)

A port of the panel protocol to [Bend 2](https://bend-lang.com) (tested with 2.0.39), built on [bend-kit](https://github.com/paymog/bend-kit) packages (`bytes`, `crypto`, `property`) and following its conventions. The Python server in the repo root is still the real implementation; this directory is a feasibility spike plus the first reusable pieces.

## What is here

| File | What |
|---|---|
| `frame.bend` | the `aa55ffff` frame codec on packed `Bytes`: `Frame.build`, `Frame.parse`, and `Frame.feed` (reassembly of frames split across BLE notifications) |
| `key.bend` | the 20-byte slot key: 4-byte prefix + MD5 (through `bend-kit-crypto`, so OpenSSL 3) |
| `hex.bend`, `fixture.bend` | hex helpers, and string-in string-out views of the codec for `LAWS.bend` |
| `LAWS.bend` / `PROOF.bend` | machine-checked laws (concrete fixtures, see below) |
| `gen_golden.py`, `golden.bend`, `fixtures/golden.json` | byte-exact vectors generated from `protocol.py`; the generator also asserts the reassembly cases against `protocol.Reassembler` |
| `test.bend`, `prop_test.bend`, `key_test.bend` | golden-vector tests, property tests (random bodies, shrinking), slot-key test |
| `ble/loyble.m` | native CoreBluetooth central with blocking calls: open, write, recv, drain, close |
| `ble/ble.bend`, `ble.c`, `ble.js` | Bend effects over that library: C for native builds, `bun:ffi` for the JS lane. Bytes cross as packed words, as in bend-kit's `wire` and `crypto`. `BLE.open.retry` rescans like `server.py` does |
| `ble_hello.bend` | opens the panel, sends the read-only device-info query, reassembles and parses the reply, prints the model |

## Checks

```
./check.sh               # proofs, 21 golden vectors, 2 properties x 200 random bodies, 4 slot keys
python3 bend/gen_golden.py   # after changing protocol.py (run from the repo root)
```

`LAWS.bend` states four fixtures: the delete frame from a real capture is built byte for byte and parses back, and a frame with a wrong checksum or a wrong length is refused. They are fixtures because `Bytes` sits on arrays, and bend-kit has no array lemmas for a universal claim; the golden vectors and the property tests cover the general case by sampling. I confirmed the gates fail when they should (a wrong expected byte, and a planted bug in a property).
Not run: `bend PROOF.bend --verdict` (needs Lean 4).

## Hardware

```
./build.sh
BEND_LIBLOYBLE=$PWD/ble/libloyble.dylib ./ble_hello       # native
BEND_LIBLOYBLE=$PWD/ble/libloyble.dylib bend ble_hello.bend   # JS lane, through bun:ffi
```
Close LOY SPACE and disconnect the Python server first: the panel takes one connection. The demo only sends the app's opening device-info query. Both lanes were run against the real panel. A panel that was just disconnected can be invisible for several seconds, hence the retry.

## Caveats

- Bend's C effects are not covered by proofs, and Bend promises no ABI for them: rebuild after a Bend update.
- macOS only (CoreBluetooth). The `bytes` import is pinned to `0.3.2.0` to match `property` and `http`; a different version is a different type.
- Everything is sequential. Frames are at most about 1 KB, so there is nothing balanced enough to gain from parallel calls.
- The slot key supports ids 1..255 (the panel has 60 slots); `Key.make` exits with code 22 outside that.
- Not done: image packing, upload/delete flows, the HTTP server, and a generic (non-LOY) BLE API.
- `BLE-BRIDGE-SPEC.md` (repo root) is the design for the generic bridge; `ble/` is panel-specific (fixed characteristics, one connection).
