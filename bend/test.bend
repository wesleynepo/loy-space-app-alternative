import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes
import ./frame.bend as F
import ./hex.bend as H
import ./golden.bend as G

# Golden-vector tests: every frame must match the bytes protocol.py produces.

def Test.from_hex(s: String) -> Bytes.Bytes:
  H.Hex.bytes(s)

# A string with its last hex digit changed, so the frame's checksum no longer matches.
def Test.flip.last(c: Maybe<&2, Char>) -> String:
  match c:
    case Some{ch}:
      Bool.pick(String, Char.is_eq(ch, '0'), "1", "0")
    case None{}:
      ""

def Test.flip(+s: String) -> String:
  String.take(s, Nat.sub(String.length(s), 1n)) ++ Test.flip.last(String.get(s, Nat.sub(String.length(s), 1n)))

def Test.is_none(m: Maybe<&1, F.Frame>) -> Bool:
  match m:
    case Some{_}:
      False{}
    case None{}:
      True{}

def Test.same.frame(f: F.Frame, seq: U32, dir: U32, body: String) -> Bool:
  match f:
    case F.Frame{s, d, b}:
      Bool.and(Bool.and(U32.is_eq(s, seq), U32.is_eq(d, dir)), String.eq(Bytes.to_hex(b), body))

def Test.same.parsed(m: Maybe<&1, F.Frame>, seq: U32, dir: U32, body: String) -> Bool:
  match m:
    case Some{f}:
      Test.same.frame(f, seq, dir, body)
    case None{}:
      False{}

def Test.case.name(c: G.Case) -> String:
  match c:
    case G.Case{n, _, _, _, _}:
      n

# build, parse round trip, and three malformed inputs that must be refused.
def Test.case(c: G.Case) -> Bool:
  match c:
    case G.Case{_, +seq, +dir, +body, +frame}:
      Bool.and(
        Bool.and(
          String.eq(Bytes.to_hex(F.Frame.build(seq, dir, Test.from_hex(body))), frame),
          Test.same.parsed(F.Frame.parse(Test.from_hex(frame)), seq, dir, body)),
        Bool.and(
          Test.is_none(F.Frame.parse(Test.from_hex(frame ++ "00"))),
          Bool.and(
            Test.is_none(F.Frame.parse(Test.from_hex(String.take(frame, Nat.sub(String.length(frame), 2n))))),
            Test.is_none(F.Frame.parse(Test.from_hex(Test.flip(frame)))))))

# --- reassembly ---

def Test.hexes(fs: List<&1, Bytes.Bytes>) -> List<&2, String>:
  match fs:
    case Nil{}:
      Nil{}
    case Con{f, rest}:
      Bytes.to_hex(f) <> Test.hexes(rest)

def Test.eq_strings(xs: List<&2, String>, ys: List<&2, String>) -> Bool:
  match xs ys:
    case Nil{} Nil{}:
      True{}
    case Con{a, as} Con{b, bs}:
      Bool.and(String.eq(a, b), Test.eq_strings(as, bs))
    case _ _:
      False{}

# Feed every chunk in turn and collect the frames that came out, as hex.
def Test.feed_all(chunks: List<&2, String>, st: F.Feed, acc: List<&2, String>) -> List<&2, String>:
  match chunks st:
    case Nil{} F.Feed{fr, _}:
      List.append(&2, String, acc, Test.hexes(fr))
    case Con{c, rest} F.Feed{fr, buf}:
      Test.feed_all(rest, F.Frame.feed(buf, Test.from_hex(c)), List.append(&2, String, acc, Test.hexes(fr)))

def Test.split.name(s: G.Split) -> String:
  match s:
    case G.Split{n, _, _}:
      n

def Test.split(s: G.Split) -> Bool:
  match s:
    case G.Split{_, chunks, frames}:
      Test.eq_strings(Test.feed_all(chunks, F.Feed{Nil{}, Bytes.new(0)}, Nil{}), frames)

# --- report ---

def Test.line(ok: Bool, name: String) -> String:
  match ok:
    case True{}:
      "ok   " ++ name ++ "\n"
    case False{}:
      "FAIL " ++ name ++ "\n"

def Test.fail(ok: Bool) -> Nat:
  match ok:
    case True{}:
      0n
    case False{}:
      1n

def Test.report.cases(cs: List<&2, G.Case>) -> String:
  match cs:
    case Nil{}:
      ""
    case Con{+c, rest}:
      Test.line(Test.case(c), Test.case.name(c)) ++ Test.report.cases(rest)

def Test.count.cases(cs: List<&2, G.Case>) -> Nat:
  match cs:
    case Nil{}:
      0n
    case Con{+c, rest}:
      Nat.add(Test.fail(Test.case(c)), Test.count.cases(rest))

def Test.report.splits(ss: List<&2, G.Split>) -> String:
  match ss:
    case Nil{}:
      ""
    case Con{+s, rest}:
      Test.line(Test.split(s), Test.split.name(s)) ++ Test.report.splits(rest)

def Test.count.splits(ss: List<&2, G.Split>) -> Nat:
  match ss:
    case Nil{}:
      0n
    case Con{+s, rest}:
      Nat.add(Test.fail(Test.split(s)), Test.count.splits(rest))

def Test.finish(failed: Nat) -> IO(Unit):
  match failed:
    case 0n:
      IO.print("all golden vectors match")
    case 1n+_:
      IO.die(Unit, 1, "golden vectors failed: " ++ Nat.show(failed))

def main() -> IO(Unit):
  do IO<Unit>:
    IO.write(Test.report.cases(G.Golden.cases()))
    IO.write(Test.report.splits(G.Golden.splits()))
    Test.finish(Nat.add(Test.count.cases(G.Golden.cases()), Test.count.splits(G.Golden.splits())))
