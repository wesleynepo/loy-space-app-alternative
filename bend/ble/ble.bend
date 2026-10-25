# Native BLE central for the LOY panel (macOS CoreBluetooth through libloyble), one connection per process.
import Base

# Bytes travel as (len, words): len octets, four to a U32 word, low byte first, as bytes/bytes.bend packs them
# (build a Bytes from them with `Bytes.Bytes{len, words}`). Every call has a timeout and fails with a code and text.
# The library loads from $BEND_LIBLOYBLE, else ./libloyble.dylib, ./ble/libloyble.dylib (see ble/loyble.m).
# Native builds use ble.c; `bend file.bend` (the JS lane) uses ble.js through bun:ffi on the same library.

# Find the panel by its advertised name, connect, discover the characteristics and subscribe.
def BLE.open(name: String, scan_ms: U32, connect_ms: U32) -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

# Write one frame. The library splits it into pieces of at most 180 bytes and honours the radio's flow control.
def BLE.write.words(len: U32, words: Array<U32>) -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

# The next notification chunk; fails with a timeout code when none arrives in time.
def BLE.recv.words(timeout_ms: U32) -> IO(Result<&1, &1, U32 & String, U32 & Array<U32>>):
  import "./ble.c"
  import "./ble.js"

# Drop notifications that arrived but were never read.
def BLE.drain() -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

# Disconnect, so the panel is free for the next client. Also runs when the process exits.
def BLE.close() -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

# A panel that was just disconnected can be invisible for a few seconds, so server.py scans up to 3 times.
# `last` is the previous attempt's result: start with a Fail, and the first attempt always runs.
def BLE.open.retry(fuel: Nat, last: Result<&1, &1, U32 & String, U32>, +name: String, +scan_ms: U32, +connect_ms: U32) ->
  IO(Result<&1, &1, U32 & String, U32>):
  match fuel:
    case 0n:
      IO.pure(Result<&1, &1, U32 & String, U32>, last)
    case 1n+p:
      match last:
        case Done{v}:
          IO.pure(Result<&1, &1, U32 & String, U32>, Done{v})
        case Fail{e}:
          do IO<Result<&1, &1, U32 & String, U32>>:
            r : Result<&1, &1, U32 & String, U32> <- BLE.open(name, scan_ms, connect_ms)
            BLE.open.retry(p, r, name, scan_ms, connect_ms)
