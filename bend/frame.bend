import Base
import ./bytes.bend as B

# Frame codec for the LOY SPACE panel: aa55ffff | len u16le | seq u16le | dir | body | sum u16le
#   len = number of bytes after the len field (seq .. sum), so len = body + 5
#   sum = sum of every preceding byte of the frame, mod 65536

type Frame is Data:
  Frame{seq: U32, dir: U32, body: List<&2, U32>}

def Frame.magic() -> List<&2, U32>:
  [170, 85, 255, 255]

# Everything before the checksum: magic, len, seq, dir, body.
def Frame.head(+seq: U32, dir: U32, +body: List<&2, U32>) -> List<&2, U32>:
  +len = (U32.from_nat(List.length(&2, U32, body)) + 5 : U32)
  fixed = {[B.Bytes.lo(len), B.Bytes.hi(len), B.Bytes.lo(seq), B.Bytes.hi(seq), dir] : List<&2, U32>}
  List.append(&2, U32, Frame.magic(), List.append(&2, U32, fixed, body))

def Frame.checksum(+head: List<&2, U32>) -> U32:
  (B.Bytes.sum(head, 0) .&. 65535 : U32)

def Frame.build(seq: U32, dir: U32, +body: List<&2, U32>) -> List<&2, U32>:
  +head = Frame.head(seq, dir, body)
  +s = Frame.checksum(head)
  List.append(&2, U32, head, {[B.Bytes.lo(s), B.Bytes.hi(s)] : List<&2, U32>})

# --- parse: Some{Frame} only for a complete frame with the right length field and checksum ---

def Frame.parse.done(ok: Bool, seq: U32, dir: U32, body: List<&2, U32>) -> Maybe<&2, Frame>:
  match ok:
    case True{}:
      Some{Frame{seq, dir, body}}
    case False{}:
      None{}

def Frame.parse.tail(seq: U32, dir: U32, body: List<&2, U32>, tl: List<&2, U32>, want: U32) -> Maybe<&2, Frame>:
  match tl:
    case Con{c0, Con{c1, Nil{}}}:
      Frame.parse.done(U32.is_eq(want, B.Bytes.u16le(c0, c1)), seq, dir, body)
    case _:
      None{}

def Frame.parse.finish(ok: Bool, +n: Nat, +f: List<&2, U32>, seq: U32, dir: U32, +rest: List<&2, U32>) -> Maybe<&2, Frame>:
  match ok:
    case False{}:
      None{}
    case True{}:
      Frame.parse.tail(
        seq, dir,
        List.take(&2, U32, rest, Nat.sub(n, 11n)),
        List.drop(&2, U32, rest, Nat.sub(n, 11n)),
        Frame.checksum(List.take(&2, U32, f, Nat.sub(n, 2n))))

def Frame.parse.cut(+n: Nat, +f: List<&2, U32>, h: List<&2, U32>, rest: List<&2, U32>) -> Maybe<&2, Frame>:
  match h:
    case Con{m0, Con{m1, Con{m2, Con{m3, Con{l0, Con{l1, Con{s0, Con{s1, Con{d, Nil{}}}}}}}}}}:
      +len = B.Bytes.u16le(l0, l1)
      magic = Bool.and(Bool.and(U32.is_eq(m0, 170), U32.is_eq(m1, 85)), Bool.and(U32.is_eq(m2, 255), U32.is_eq(m3, 255)))
      sized = Nat.is_eq(Nat.add(6n, U32.to_nat(len)), n)
      Frame.parse.finish(Bool.and(magic, sized), n, f, B.Bytes.u16le(s0, s1), d, rest)
    case _:
      None{}

def Frame.parse(+f: List<&2, U32>) -> Maybe<&2, Frame>:
  +n = List.length(&2, U32, f)
  Frame.parse.cut(n, f, List.take(&2, U32, f, 9n), List.drop(&2, U32, f, 9n))

# --- stream reassembly: frames larger than a BLE notification arrive in several chunks ---

type Feed is Data:
  Feed{frames: List<&2, List<&2, U32>>, rest: List<&2, U32>}

def Bytes.eq(xs: List<&2, U32>, ys: List<&2, U32>) -> Bool:
  match xs ys:
    case Nil{} Nil{}:
      True{}
    case Con{a, as} Con{b, bs}:
      Bool.and(U32.is_eq(a, b), Bytes.eq(as, bs))
    case _ _:
      False{}

# Bytes 4..6 hold the length: the total frame size, or 0 while the header is incomplete.
def Frame.want.of(h: List<&2, U32>) -> Nat:
  match h:
    case Con{_, Con{_, Con{_, Con{_, Con{l0, Con{l1, Nil{}}}}}}}:
      Nat.add(6n, U32.to_nat(B.Bytes.u16le(l0, l1)))
    case _:
      0n

def Frame.want(+buf: List<&2, U32>) -> Nat:
  Frame.want.of(List.take(&2, U32, buf, 6n))

# One step of reassembly: the first whole frame in the buffer, or the buffer as it is.
type Cut is Data:
  Short{buf: List<&2, U32>}
  Whole{frame: List<&2, U32>, rest: List<&2, U32>}

def Frame.cut.pick(ready: Bool, +buf: List<&2, U32>, +want: Nat) -> Cut:
  match ready:
    case False{}:
      Short{buf}
    case True{}:
      Whole{List.take(&2, U32, buf, want), List.drop(&2, U32, buf, want)}

def Frame.cut.of(+buf: List<&2, U32>, +want: Nat) -> Cut:
  match want:
    case 0n:
      Short{buf}
    case 1n+_:
      Frame.cut.pick(Nat.is_ge(List.length(&2, U32, buf), want), buf, want)

def Frame.cut(+buf: List<&2, U32>) -> Cut:
  Frame.cut.of(buf, Frame.want(buf))

def Frame.split.stop(c: Cut, acc: List<&2, List<&2, U32>>) -> Feed:
  match c:
    case Short{buf}:
      Feed{List.reverse(&2, List<&2, U32>, acc), buf}
    case Whole{frame, rest}:
      Feed{List.reverse(&2, List<&2, U32>, frame <> acc), rest}

# Fuel is the buffer length: every frame takes at least 6 bytes, so it never runs out early.
def Frame.split(fuel: Nat, c: Cut, acc: List<&2, List<&2, U32>>) -> Feed:
  match fuel:
    case 0n:
      Frame.split.stop(c, acc)
    case 1n+p:
      match c:
        case Short{buf}:
          Feed{List.reverse(&2, List<&2, U32>, acc), buf}
        case Whole{frame, rest}:
          Frame.split(p, Frame.cut(rest), frame <> acc)

def Frame.feed.join(fresh: Bool, buf: List<&2, U32>, +chunk: List<&2, U32>) -> List<&2, U32>:
  match fresh:
    case True{}:
      chunk
    case False{}:
      List.append(&2, U32, buf, chunk)

# A chunk that starts with the magic restarts the buffer (drops a torn frame); otherwise it continues it.
def Frame.feed(buf: List<&2, U32>, +chunk: List<&2, U32>) -> Feed:
  +joined = Frame.feed.join(Bytes.eq(List.take(&2, U32, chunk, 4n), Frame.magic()), buf, chunk)
  Frame.split(List.length(&2, U32, joined), Frame.cut(joined), Nil{})
