# Frame codec for the LOY SPACE LED panel, on bend-kit's packed Bytes: build, parse, and stream reassembly.
import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes

# aa55ffff | len u16le | seq u16le | dir u8 | body | sum u16le
#   len = number of bytes after the len field (seq .. sum), so len = body + 5
#   sum = sum of every preceding byte of the frame, mod 65536

type Frame is Type:
  Frame{seq: U32, dir: U32, body: Bytes.Bytes}

# aa55ffff read as a big-endian u32.
def Frame.magic() -> U32:
  2857762815

# Bytes [i, end) summed, wrapping at 32 bits. r is the byte at i (None past the buffer or at end);
# fuel counts the bytes still to visit. Returns the buffer beside the sum.
def Frame.sum.at(b: Bytes.Bytes, +i: U32, +end: U32, ok: Bool) -> Bytes.Bytes & Maybe<&2, U32>:
  match ok:
    case True{}:
      Bytes.get(b, i)
    case False{}:
      (b, None{})

def Frame.sum.stop(r: Bytes.Bytes & Maybe<&2, U32>, acc: U32) -> Bytes.Bytes & U32:
  (b, _) = r
  (b, acc)

def Frame.sum(fuel: Nat, r: Bytes.Bytes & Maybe<&2, U32>, +i: U32, +end: U32, acc: U32) -> Bytes.Bytes & U32:
  match fuel:
    case 0n:
      Frame.sum.stop(r, acc)
    case 1n+p:
      (b, m) = r
      match m:
        case None{}:
          (b, acc)
        case Some{v}:
          Frame.sum(p, Frame.sum.at(b, (i + 1 : U32), end, U32.is_lt((i + 1 : U32), end)), (i + 1 : U32), end, (acc + v : U32))

def Frame.sum_to(b: Bytes.Bytes, +end: U32) -> Bytes.Bytes & U32:
  Frame.sum(U32.to_nat(end), Frame.sum.at(b, 0, end, U32.is_lt(0, end)), 0, end, 0)

# --- build ---

def Frame.head(len: U32, seq: U32, dir: U32) -> Bytes.Bytes:
  b = Bytes.new(9)
  b = Bytes.set.u32be(b, 0, Frame.magic())
  b = Bytes.set.u16le(b, 4, len)
  b = Bytes.set.u16le(b, 6, seq)
  Bytes.set(b, 8, dir)

def Frame.trailer(s: U32) -> Bytes.Bytes:
  Bytes.set.u16le(Bytes.new(2), 0, (s .&. 65535 : U32))

def Frame.build.fin(r: Bytes.Bytes & U32) -> Bytes.Bytes:
  (all, s) = r
  Bytes.append(all, Frame.trailer(s))

def Frame.build.sized(+seq: U32, dir: U32, r: Bytes.Bytes & U32) -> Bytes.Bytes:
  (body, +n) = r
  Frame.build.fin(Frame.sum_to(Bytes.append(Frame.head((n + 5 : U32), seq, dir), body), (n + 9 : U32)))

def Frame.build(seq: U32, dir: U32, body: Bytes.Bytes) -> Bytes.Bytes:
  Frame.build.sized(seq, dir, Bytes.length(body))

# --- parse: Some only for a complete frame with the right magic, length field and checksum ---

def Frame.parse.done(r: Bytes.Bytes & Bytes.Bytes, seq: U32, dir: U32) -> Maybe<&1, Frame>:
  (f, body) = r
  Some{Frame{seq, dir, body}}

def Frame.parse.dir(r: Bytes.Bytes & Maybe<&2, U32>, seq: U32, +n: U32) -> Maybe<&1, Frame>:
  (f, m) = r
  Frame.parse.done(Bytes.slice(f, 9, (n - 11 : U32)), seq, Maybe.default(&2, U32, m, 0))

def Frame.parse.seq(r: Bytes.Bytes & Maybe<&2, U32>, +n: U32) -> Maybe<&1, Frame>:
  (f, m) = r
  Frame.parse.dir(Bytes.get(f, 8), Maybe.default(&2, U32, m, 0), n)

def Frame.parse.sum.ok(ok: Bool, f: Bytes.Bytes, n: U32) -> Maybe<&1, Frame>:
  match ok:
    case True{}:
      Frame.parse.seq(Bytes.get.u16le(f, 6), n)
    case False{}:
      None{}

def Frame.parse.trailer(r: Bytes.Bytes & Maybe<&2, U32>, +s: U32, +n: U32) -> Maybe<&1, Frame>:
  (f, m) = r
  Frame.parse.sum.ok(U32.is_eq((s .&. 65535 : U32), Maybe.default(&2, U32, m, 65536)), f, n)

def Frame.parse.summed(r: Bytes.Bytes & U32, +n: U32) -> Maybe<&1, Frame>:
  (f, s) = r
  Frame.parse.trailer(Bytes.get.u16le(f, (n - 2 : U32)), s, n)

def Frame.parse.len.ok(ok: Bool, f: Bytes.Bytes, +n: U32) -> Maybe<&1, Frame>:
  match ok:
    case True{}:
      Frame.parse.summed(Frame.sum_to(f, (n - 2 : U32)), n)
    case False{}:
      None{}

def Frame.parse.len(r: Bytes.Bytes & Maybe<&2, U32>, +n: U32) -> Maybe<&1, Frame>:
  (f, m) = r
  Frame.parse.len.ok(U32.is_eq((Maybe.default(&2, U32, m, 0) + 6 : U32), n), f, n)

def Frame.parse.magic.ok(ok: Bool, f: Bytes.Bytes, n: U32) -> Maybe<&1, Frame>:
  match ok:
    case True{}:
      Frame.parse.len(Bytes.get.u16le(f, 4), n)
    case False{}:
      None{}

def Frame.parse.magic(r: Bytes.Bytes & Maybe<&2, U32>, n: U32) -> Maybe<&1, Frame>:
  (f, m) = r
  Frame.parse.magic.ok(U32.is_eq(Maybe.default(&2, U32, m, 0), Frame.magic()), f, n)

def Frame.parse.size.ok(ok: Bool, f: Bytes.Bytes, n: U32) -> Maybe<&1, Frame>:
  match ok:
    case True{}:
      Frame.parse.magic(Bytes.get.u32be(f, 0), n)
    case False{}:
      None{}

def Frame.parse.size(r: Bytes.Bytes & U32) -> Maybe<&1, Frame>:
  (f, +n) = r
  Frame.parse.size.ok(U32.is_ge(n, 11), f, n)

def Frame.parse(f: Bytes.Bytes) -> Maybe<&1, Frame>:
  Frame.parse.size(Bytes.length(f))

# --- stream reassembly: frames larger than a BLE notification arrive in several chunks ---

type Feed is Type:
  Feed{frames: List<&1, Bytes.Bytes>, rest: Bytes.Bytes}

# One step: the first whole frame in the buffer, or the buffer as it is.
type Cut is Type:
  Short{buf: Bytes.Bytes}
  Whole{frame: Bytes.Bytes, rest: Bytes.Bytes}

def Frame.cut.rest(r: Bytes.Bytes & Bytes.Bytes, frame: Bytes.Bytes) -> Cut:
  (b, rest) = r
  Whole{frame, rest}

def Frame.cut.frame(r: Bytes.Bytes & Bytes.Bytes, +n: U32, +total: U32) -> Cut:
  (b, frame) = r
  Frame.cut.rest(Bytes.slice(b, total, (n - total : U32)), frame)

def Frame.cut.whole(ok: Bool, b: Bytes.Bytes, +n: U32, +total: U32) -> Cut:
  match ok:
    case True{}:
      Frame.cut.frame(Bytes.slice(b, 0, total), n, total)
    case False{}:
      Short{b}

def Frame.cut.total(r: Bytes.Bytes & Maybe<&2, U32>, +n: U32) -> Cut:
  (b, m) = r
  +total = (Maybe.default(&2, U32, m, 0) + 6 : U32)
  Frame.cut.whole(U32.is_ge(n, total), b, n, total)

def Frame.cut.header(ok: Bool, b: Bytes.Bytes, n: U32) -> Cut:
  match ok:
    case True{}:
      Frame.cut.total(Bytes.get.u16le(b, 4), n)
    case False{}:
      Short{b}

def Frame.cut.size(r: Bytes.Bytes & U32) -> Cut:
  (b, +n) = r
  Frame.cut.header(U32.is_ge(n, 6), b, n)

def Frame.cut(buf: Bytes.Bytes) -> Cut:
  Frame.cut.size(Bytes.length(buf))

def Frame.split.stop(c: Cut, acc: List<&1, Bytes.Bytes>) -> Feed:
  match c:
    case Short{buf}:
      Feed{List.reverse(&1, Bytes.Bytes, acc), buf}
    case Whole{frame, rest}:
      Feed{List.reverse(&1, Bytes.Bytes, frame <> acc), rest}

# Fuel is the buffer length: every frame takes at least 6 bytes, so it never runs out early.
def Frame.split(fuel: Nat, c: Cut, acc: List<&1, Bytes.Bytes>) -> Feed:
  match fuel:
    case 0n:
      Frame.split.stop(c, acc)
    case 1n+p:
      match c:
        case Short{buf}:
          Feed{List.reverse(&1, Bytes.Bytes, acc), buf}
        case Whole{frame, rest}:
          Frame.split(p, Frame.cut(rest), frame <> acc)

def Frame.feed.go(r: Bytes.Bytes & U32) -> Feed:
  (joined, +n) = r
  Frame.split(U32.to_nat(n), Frame.cut(joined), Nil{})

def Frame.feed.join(fresh: Bool, buf: Bytes.Bytes, chunk: Bytes.Bytes) -> Bytes.Bytes:
  match fresh:
    case True{}:
      chunk
    case False{}:
      Bytes.append(buf, chunk)

def Frame.feed.start(r: Bytes.Bytes & Maybe<&2, U32>, buf: Bytes.Bytes) -> Feed:
  (chunk, m) = r
  Frame.feed.go(Bytes.length(Frame.feed.join(U32.is_eq(Maybe.default(&2, U32, m, 0), Frame.magic()), buf, chunk)))

# A chunk that starts with the magic restarts the buffer (drops a torn frame); otherwise it continues it.
def Frame.feed(buf: Bytes.Bytes, chunk: Bytes.Bytes) -> Feed:
  Frame.feed.start(Bytes.get.u32be(chunk, 0), buf)
