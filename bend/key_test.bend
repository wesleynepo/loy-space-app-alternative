import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes
import ./key.bend as K
import ./hex.bend as H
import ./golden.bend as G

# Checks K.Key.make against protocol.make_key. Needs libcrypto: build with `bend key_test.bend -o key_test`.

def KeyTest.from_hex(s: String) -> Bytes.Bytes:
  H.Hex.bytes(s)

def KeyTest.one(c: G.KeyCase) -> IO(Bool):
  match c:
    case G.KeyCase{id_pro, packed, ts, key}:
      do IO<Bool>:
        k : Bytes.Bytes <- K.Key.make(id_pro, KeyTest.from_hex(packed), ts)
        IO.pure(Bool, String.eq(Bytes.to_hex(k), key))

def KeyTest.bad(ok: Bool) -> Nat:
  Bool.pick(Nat, ok, 0n, 1n)

def KeyTest.run(cs: List<&2, G.KeyCase>, bad: Nat) -> IO(Nat):
  match cs:
    case Nil{}:
      IO.pure(Nat, bad)
    case Con{c, rest}:
      do IO<Nat>:
        ok : Bool <- KeyTest.one(c)
        KeyTest.run(rest, Nat.add(bad, KeyTest.bad(ok)))

def KeyTest.finish(bad: Nat) -> IO(Unit):
  match bad:
    case 0n:
      IO.print("all slot keys match")
    case 1n+_:
      IO.die(Unit, 1, "slot keys failed: " ++ Nat.show(bad))

def main() -> IO(Unit):
  do IO<Unit>:
    bad : Nat <- KeyTest.run(G.Golden.keys(), 0n)
    KeyTest.finish(bad)
