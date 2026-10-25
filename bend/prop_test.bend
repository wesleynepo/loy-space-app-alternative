import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes
import bend-kit-property@0.3.0.0/property.bend as Prop
import bend-kit-property@0.3.0.0/generate.bend as Gen
import bend-kit-property@0.3.0.0/shrink.bend as Shrink
import ./frame.bend as F

# Property tests over random bodies (seeded, shrinking): the properties the golden vectors can only sample.

# bytes@0.3.2.0 has no clone: a copy is a slice of the whole buffer, as bend-kit's own property check does.
def PT.clone.pair(r: Bytes.Bytes & U32) -> Bytes.Bytes & Bytes.Bytes:
  (b, len) = r
  Bytes.slice(b, 0, len)

def PT.clone(b: Bytes.Bytes) -> Bytes.Bytes & Bytes.Bytes:
  PT.clone.pair(Bytes.length(b))

# --- build then parse gives the body back ---

def PT.rt.eq(r: Bytes.Bytes & Bytes.Bytes & Bool, +s: U32, +d: U32) -> Bytes.Bytes & Bool:
  (original, other, ok) = r
  (original, Bool.and(ok, Bool.and(U32.is_eq(s, 7), U32.is_eq(d, 129))))

def PT.rt.check(m: Maybe<&1, F.Frame>, original: Bytes.Bytes) -> Bytes.Bytes & Bool:
  match m:
    case Some{F.Frame{s, d, body}}:
      PT.rt.eq(Bytes.eq(original, body), s, d)
    case None{}:
      (original, False{})

def PT.rt.go(r: Bytes.Bytes & Bytes.Bytes) -> Bytes.Bytes & Bool:
  (original, probe) = r
  PT.rt.check(F.Frame.parse(F.Frame.build(7, 129, probe)), original)

def PT.rt(b: Bytes.Bytes) -> Bytes.Bytes & Bool:
  PT.rt.go(PT.clone(b))

# --- a built frame cut in two anywhere reassembles into itself ---

def PT.is_nil(fs: List<&1, Bytes.Bytes>) -> Bool:
  match fs:
    case Nil{}:
      True{}
    case Con{_, _}:
      False{}

def PT.same.eq(r: Bytes.Bytes & Bytes.Bytes & Bool) -> Bool:
  (a, b, ok) = r
  ok

def PT.same(f: Bytes.Bytes, expect: Bytes.Bytes) -> Bool:
  PT.same.eq(Bytes.eq(f, expect))

def PT.one(fs: List<&1, Bytes.Bytes>, expect: Bytes.Bytes) -> Bool:
  match fs:
    case Con{f, Nil{}}:
      PT.same(f, expect)
    case _:
      False{}

def PT.empty(r: Bytes.Bytes & U32) -> Bool:
  (b, n) = r
  U32.is_eq(n, 0)

def PT.split.verdict(fr1: List<&1, Bytes.Bytes>, st2: F.Feed, expect: Bytes.Bytes) -> Bool:
  match st2:
    case F.Feed{fr2, rest}:
      Bool.and(PT.is_nil(fr1), Bool.and(PT.one(fr2, expect), PT.empty(Bytes.length(rest))))

def PT.split.second(st: F.Feed, tail: Bytes.Bytes, expect: Bytes.Bytes) -> Bool:
  match st:
    case F.Feed{fr1, buf}:
      PT.split.verdict(fr1, F.Frame.feed(buf, tail), expect)

def PT.split.tail(orig: Bytes.Bytes, expect: Bytes.Bytes, head: Bytes.Bytes, r: Bytes.Bytes & Bytes.Bytes) -> Bytes.Bytes & Bool:
  (frame, tail) = r
  (orig, PT.split.second(F.Frame.feed(Bytes.new(0), head), tail, expect))

def PT.split.head(orig: Bytes.Bytes, expect: Bytes.Bytes, r: Bytes.Bytes & Bytes.Bytes, +n: U32) -> Bytes.Bytes & Bool:
  (frame, head) = r
  PT.split.tail(orig, expect, head, Bytes.slice(frame, (n / 2 : U32), (n - (n / 2 : U32) : U32)))

def PT.split.len(orig: Bytes.Bytes, expect: Bytes.Bytes, r: Bytes.Bytes & U32) -> Bytes.Bytes & Bool:
  (frame, +n) = r
  PT.split.head(orig, expect, Bytes.slice(frame, 0, (n / 2 : U32)), n)

def PT.split.built(orig: Bytes.Bytes, r: Bytes.Bytes & Bytes.Bytes) -> Bytes.Bytes & Bool:
  (frame, expect) = r
  PT.split.len(orig, expect, Bytes.length(frame))

def PT.split.go(r: Bytes.Bytes & Bytes.Bytes) -> Bytes.Bytes & Bool:
  (orig, probe) = r
  PT.split.built(orig, PT.clone(F.Frame.build(7, 129, probe)))

def PT.split(b: Bytes.Bytes) -> Bytes.Bytes & Bool:
  PT.split.go(PT.clone(b))

def PT.require(ok: Bool, msg: String) -> IO(Unit):
  match ok:
    case True{}:
      IO.pure(Unit, Unit{})
    case False{}:
      IO.die(Unit, 1, msg)

def main() -> IO(Unit):
  do IO<Unit>:
    rt : Bool <- Prop.run(~Bytes.Bytes, ~(r => Gen.bytes(300, r)), ~PT.clone, ~Shrink.bytes, ~PT.rt, ~Bytes.to_hex, 42, 200n)
    PT.require(rt, "build/parse round trip failed")
    sp : Bool <- Prop.run(~Bytes.Bytes, ~(r => Gen.bytes(300, r)), ~PT.clone, ~Shrink.bytes, ~PT.split, ~Bytes.to_hex, 7, 200n)
    PT.require(sp, "split reassembly failed")
    IO.print("properties hold: round trip and split reassembly, 200 random bodies each")
