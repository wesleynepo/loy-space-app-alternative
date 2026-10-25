# Bend port (work in progress)

A port of the panel protocol to [Bend 2](https://bend-lang.com) (tested with 2.0.39). The Python server in the repo root is still the real implementation; this directory is a feasibility spike plus the first reusable pieces.

## What is here

| File | What |
|---|---|
| `bytes.bend` | byte helpers: little-endian u16, running sum, hex in and out, ASCII |
| `frame.bend` | the `aa55ffff` frame codec: `Frame.build`, `Frame.parse`, and `Frame.feed` (reassembly of frames split across BLE notifications) |
| `LAWS.bend` / `PROOF.bend` | machine-checked laws about the codec (see below) |
| `golden.bend`, `test.bend` | byte-exact vectors generated from `protocol.py`, and the runner |
| `ble/loyble.m` | native CoreBluetooth central with blocking calls: open, write, recv, drain, close |
| `ble/ble.c`, `ble.js`, `ble.bend` | Bend effects over that library, loaded at run time from `$BEND_LIBLOYBLE` |
| `ble_hello.bend` | opens the panel, sends the read-only device-info query, reassembles and parses the reply |
| `gen_golden.py` | regenerates `fixtures/golden.json` and `golden.bend` from `protocol.py` |

## Checks

```
bend PROOF.bend          # ALL PROOFS CHECK
bend test.bend           # 19 golden vectors: build, parse round trip, bad input refused, chunked reassembly
python3 bend/gen_golden.py   # after changing protocol.py (run from the repo root)
```

Proven laws: the checksum splits over a join (`sum_append`), appending lists adds lengths (`length_append`, `length_trailer`), and a built frame is exactly `11 + length(body)` bytes (`build_length`).
Not proven: that `parse(build(x)) == x` for all `x` (needs bit-level reasoning on `U32`); it is covered by the golden vectors instead. `bend PROOF.bend --verdict` needs Lean 4 and was not run.

## Hardware spike

```
./build.sh
BEND_LIBLOYBLE=$PWD/ble/libloyble.dylib ./ble_hello
```
Close LOY SPACE and disconnect the Python server first: the panel takes one connection. The demo only sends the app's opening device-info query.

## Limits and caveats

- Bend's C effects are not covered by proofs, and Bend promises no ABI for them: rebuild after a Bend update.
- BLE works in native builds only (`-o`); the JS lane reports that instead.
- Everything here is sequential. Frames are at most about 1 KB, so there is nothing balanced enough to gain from parallel calls.
- `BLE-BRIDGE-SPEC.md` (repo root) is the design for the generic version of the bridge; this one is panel-specific (fixed characteristics, one connection).
