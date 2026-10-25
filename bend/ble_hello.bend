import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes
import ./frame.bend as F
import ./hex.bend as H
import ./ble/ble.bend as Ble

# Opens the panel, sends the app's opening query (device info, read-only), reassembles the reply and prints the model.
# Native: bend ble_hello.bend -o ble_hello      JS lane: bend ble_hello.bend   (BEND_LIBLOYBLE=ble/libloyble.dylib)

def Hello.send(b: Bytes.Bytes) -> IO(Unit):
  Bytes.Bytes{len, words} = b
  do IO<Unit>:
    _ : U32 <- IO.try(U32, Ble.BLE.write.words(len, words))
    IO.pure(Unit, Unit{})

def Hello.chunk(r: U32 & Array<U32>) -> Bytes.Bytes:
  (n, words) = r
  Bytes.Bytes{n, words}

# Read chunks until a whole frame has been reassembled; fuel bounds the number of chunks.
def Hello.read(fuel: Nat, st: F.Feed) -> IO(Maybe<&1, Bytes.Bytes>):
  match fuel st:
    case 0n _:
      IO.pure(Maybe<&1, Bytes.Bytes>, None{})
    case 1n+p F.Feed{frames, buf}:
      match frames:
        case Con{f, _}:
          IO.pure(Maybe<&1, Bytes.Bytes>, Some{f})
        case Nil{}:
          do IO<Maybe<&1, Bytes.Bytes>>:
            r : U32 & Array<U32> <- IO.try(U32 & Array<U32>, Ble.BLE.recv.words(3000))
            Hello.read(p, F.Frame.feed(buf, Hello.chunk(r)))

# The reply body is 83 0a <len> <comma-separated text>; the first field is the model.
def Hello.first(parts: List<&2, String>) -> String:
  match parts:
    case Con{h, _}:
      h
    case Nil{}:
      ""

def Hello.model(r: Bytes.Bytes & Bytes.Bytes) -> String:
  (body, text) = r
  Hello.first(String.split(Bytes.to_string(text), ','))

def Hello.model.of(r: Bytes.Bytes & U32) -> String:
  (body, +n) = r
  Hello.model(Bytes.slice(body, 3, (n - 3 : U32)))

def Hello.show(m: Maybe<&1, F.Frame>) -> String:
  match m:
    case Some{F.Frame{seq, dir, body}}:
      "reply seq=" ++ U32.show(seq) ++ " dir=" ++ U32.show(dir) ++ " model=" ++ Hello.model.of(Bytes.length(body))
    case None{}:
      "reply did not parse"

def Hello.done(frame: Maybe<&1, Bytes.Bytes>) -> IO(Unit):
  match frame:
    case Some{f}:
      IO.print(Hello.show(F.Frame.parse(f)))
    case None{}:
      IO.print("no reply")

def main() -> IO(Unit):
  do IO<Unit>:
    opened : Result<&1, &1, U32 & String, U32> <- Ble.BLE.open.retry(3n, Fail{(0, "not tried")}, "YS524A250818", 10000, 20000)
    _ : U32 <- IO.pass(U32, opened)
    IO.print("connected")
    _ : U32 <- IO.try(U32, Ble.BLE.drain())
    Hello.send(F.Frame.build(41, 193, H.Hex.bytes("030a001b0004000600")))
    frame : Maybe<&1, Bytes.Bytes> <- Hello.read(8n, F.Feed{Nil{}, Bytes.new(0)})
    _ : U32 <- IO.try(U32, Ble.BLE.close())
    Hello.done(frame)
