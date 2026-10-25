import Base
import ./bytes.bend as B
import ./frame.bend as F
import ./ble/ble.bend as Ble

# Opens the panel, sends the app's opening query (device info, read-only), reassembles the reply, prints the model.
# Build: bend ble_hello.bend -o ble_hello   (libloyble.dylib must sit beside the binary)

def Hello.frames.some(m: Maybe<&2, List<&2, U32>>) -> List<&2, U32>:
  Maybe.default(&2, List<&2, U32>, m, Nil{})

# Read chunks until a whole frame has been reassembled, with fuel as the bound on chunks.
def Hello.read(fuel: Nat, st: F.Feed) -> IO(Maybe<&2, List<&2, U32>>):
  match fuel st:
    case 0n _:
      IO.pure(Maybe<&2, List<&2, U32>>, None{})
    case 1n+p F.Feed{frames, buf}:
      match frames:
        case Con{f, _}:
          IO.pure(Maybe<&2, List<&2, U32>>, Some{f})
        case Nil{}:
          do IO<Maybe<&2, List<&2, U32>>>:
            chunk : String <- IO.try(String, Ble.BLE.recv(3000))
            Hello.read(p, F.Frame.feed(buf, Hello.frames.some(B.Bytes.unhex(chunk))))

# Reply body is 83 0a <len> <text>.
def Hello.model(body: List<&2, U32>) -> String:
  B.Bytes.text(List.drop(&2, U32, body, 3n))

def Hello.show(m: Maybe<&2, F.Frame>) -> String:
  match m:
    case Some{F.Frame{seq, dir, body}}:
      "reply seq=" ++ U32.show(seq) ++ " dir=" ++ U32.show(dir) ++ " device info: " ++ Hello.model(body)
    case None{}:
      "reply did not parse"

def Hello.done(frame: Maybe<&2, List<&2, U32>>) -> IO(Unit):
  match frame:
    case Some{f}:
      IO.print(Hello.show(F.Frame.parse(f)))
    case None{}:
      IO.print("no reply")

def main() -> IO(Unit):
  do IO<Unit>:
    _ : U32 <- IO.try(U32, Ble.BLE.open("YS524A250818", 10000, 20000))
    IO.print("connected")
    _ : U32 <- IO.try(U32, Ble.BLE.drain())
    _ : U32 <- IO.try(U32, Ble.BLE.write(B.Bytes.hex(F.Frame.build(41, 193, {[3, 10, 0, 27, 0, 4, 0, 6, 0] : List<&2, U32>}))))
    frame : Maybe<&2, List<&2, U32>> <- Hello.read(8n, F.Feed{Nil{}, Nil{}})
    _ : U32 <- IO.try(U32, Ble.BLE.close())
    Hello.done(frame)
