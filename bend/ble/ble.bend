import Base

# Native BLE central for the LOY panel (see loyble.m). One connection per process; every call has a timeout.
# Build programs that use it with `bend file.bend -o file` and keep libloyble.dylib beside the binary
# (or set BEND_LIBLOYBLE). Frames travel as hex text, so the effect layer stays tiny.

# Find the panel by its advertised name, connect, discover the characteristics and subscribe.
def BLE.open(name: String, scan_ms: U32, connect_ms: U32) -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

# Write one frame (hex text). The library splits it into pieces of at most 180 bytes.
def BLE.write(hex: String) -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

# The next notification chunk as hex text; fails with a timeout code when none arrives in time.
def BLE.recv(timeout_ms: U32) -> IO(Result<&1, &1, U32 & String, String>):
  import "./ble.c"
  import "./ble.js"

# Drop notifications that arrived but were never read.
def BLE.drain() -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"

def BLE.close() -> IO(Result<&1, &1, U32 & String, U32>):
  import "./ble.c"
  import "./ble.js"
