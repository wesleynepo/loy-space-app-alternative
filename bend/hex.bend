# Hex text to Bytes and back, for fixtures and tests.
import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes

# Empty for odd or non-hex text; tests compare the hex back, so a bad fixture shows up as a mismatch.
def Hex.bytes(s: String) -> Bytes.Bytes:
  Maybe.default(&1, Bytes.Bytes, Bytes.from_hex(s), Bytes.new(0))

def Hex.show(b: Bytes.Bytes) -> String:
  Bytes.to_hex(b)
