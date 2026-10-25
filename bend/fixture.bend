# Small views over the codec for LAWS.bend: strings in, strings out, so a law is a plain equality.
import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes
import ./hex.bend as H
import ./frame.bend as F

def Fixture.build(seq: U32, dir: U32, body_hex: String) -> String:
  H.Hex.show(F.Frame.build(seq, dir, H.Hex.bytes(body_hex)))

def Fixture.parsed.of(m: Maybe<&1, F.Frame>) -> String:
  match m:
    case Some{F.Frame{seq, dir, body}}:
      U32.show(seq) ++ " " ++ U32.show(dir) ++ " " ++ H.Hex.show(body)
    case None{}:
      "refused"

# "seq dir body" of a frame, or "refused".
def Fixture.parse(frame_hex: String) -> String:
  Fixture.parsed.of(F.Frame.parse(H.Hex.bytes(frame_hex)))
