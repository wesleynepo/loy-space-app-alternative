import Base
import ./bytes.bend as B
import ./frame.bend as F
import ./golden.bend as G

# Golden-vector tests: every frame must match the bytes protocol.py produces.

def Test.same.frame(f: F.Frame, seq: U32, dir: U32, body: List<&2, U32>) -> Bool:
  match f:
    case F.Frame{s, d, b}:
      Bool.and(Bool.and(U32.is_eq(s, seq), U32.is_eq(d, dir)), F.Bytes.eq(b, body))

def Test.same.parsed(m: Maybe<&2, F.Frame>, seq: U32, dir: U32, body: List<&2, U32>) -> Bool:
  match m:
    case Some{f}:
      Test.same.frame(f, seq, dir, body)
    case None{}:
      False{}

def Test.is_none(m: Maybe<&2, F.Frame>) -> Bool:
  match m:
    case Some{_}:
      False{}
    case None{}:
      True{}

# build, parse round trip, and two malformed inputs that must be refused.
def Test.case(c: G.Case) -> Bool:
  match c:
    case G.Case{_, +seq, +dir, +body, +hex}:
      +frame = F.Frame.build(seq, dir, body)
      Bool.and(
        Bool.and(String.eq(B.Bytes.hex(frame), hex), Test.same.parsed(F.Frame.parse(frame), seq, dir, body)),
        Bool.and(
          Test.is_none(F.Frame.parse(List.append(&2, U32, frame, {[0] : List<&2, U32>}))),
          Test.is_none(F.Frame.parse(List.take(&2, U32, frame, Nat.sub(List.length(&2, U32, frame), 1n))))))

def Test.eq_frames(xs: List<&2, List<&2, U32>>, ys: List<&2, List<&2, U32>>) -> Bool:
  match xs ys:
    case Nil{} Nil{}:
      True{}
    case Con{a, as} Con{b, bs}:
      Bool.and(F.Bytes.eq(a, b), Test.eq_frames(as, bs))
    case _ _:
      False{}

def Test.feed_all(chunks: List<&2, List<&2, U32>>, st: F.Feed, acc: List<&2, List<&2, U32>>) -> F.Feed:
  match chunks st:
    case Nil{} F.Feed{fr, buf}:
      F.Feed{List.append(&2, List<&2, U32>, acc, fr), buf}
    case Con{c, rest} F.Feed{fr, buf}:
      Test.feed_all(rest, F.Frame.feed(buf, c), List.append(&2, List<&2, U32>, acc, fr))

def Test.split.done(r: F.Feed, frames: List<&2, List<&2, U32>>) -> Bool:
  match r:
    case F.Feed{got, _}:
      Test.eq_frames(got, frames)

def Test.split(s: G.Split) -> Bool:
  match s:
    case G.Split{_, chunks, frames}:
      Test.split.done(Test.feed_all(chunks, F.Feed{Nil{}, Nil{}}, Nil{}), frames)

def Test.case.name(c: G.Case) -> String:
  match c:
    case G.Case{n, _, _, _, _}:
      n

def Test.split.name(s: G.Split) -> String:
  match s:
    case G.Split{n, _, _}:
      n

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
