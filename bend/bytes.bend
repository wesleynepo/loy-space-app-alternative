import Base

# Bytes are U32 values 0..255 held in a list.

def Bytes.lo(x: U32) -> U32:
  (x .&. 255 : U32)

def Bytes.hi(x: U32) -> U32:
  ((x >> 8n : U32) .&. 255 : U32)

# Two bytes, little-endian, as a number.
def Bytes.u16le(lo: U32, hi: U32) -> U32:
  (lo + (hi << 8n : U32) : U32)

# Sum of every byte, wrapping at 32 bits (callers mask to 16).
def Bytes.sum(xs: List<&2, U32>, acc: U32) -> U32:
  match xs:
    case Nil{}:
      acc
    case Con{h, t}:
      Bytes.sum(t, (acc + h : U32))

def Bytes.digit.pick(n: U32, low: Bool) -> Char:
  match low:
    case True{}:
      Char.from_u32((48 + n : U32))
    case False{}:
      Char.from_u32((87 + n : U32))

def Bytes.digit(+n: U32) -> Char:
  Bytes.digit.pick(n, U32.is_lt(n, 10))

def Bytes.hex_byte(+b: U32) -> String:
  String.from_list({[Bytes.digit((b >> 4n : U32)), Bytes.digit((b .&. 15 : U32))] : List<&2, Char>})

def Bytes.hex(xs: List<&2, U32>) -> String:
  match xs:
    case Nil{}:
      ""
    case Con{h, t}:
      Bytes.hex_byte(h) ++ Bytes.hex(t)

# Hex text back to bytes; None on an odd length or a non-hex character.
def Bytes.nibble.pick(c: U32, digit: Bool, lower: Bool) -> Maybe<&2, U32>:
  match digit:
    case True{}:
      Some{(c - 48 : U32)}
    case False{}:
      match lower:
        case True{}:
          Some{(c - 87 : U32)}
        case False{}:
          None{}

def Bytes.nibble(+c: U32) -> Maybe<&2, U32>:
  Bytes.nibble.pick(c, Bool.and(U32.is_ge(c, 48), U32.is_le(c, 57)), Bool.and(U32.is_ge(c, 97), U32.is_le(c, 102)))

def Bytes.unhex.join(hi: Maybe<&2, U32>, lo: Maybe<&2, U32>, rest: Maybe<&2, List<&2, U32>>) -> Maybe<&2, List<&2, U32>>:
  match hi lo rest:
    case Some{h} Some{l} Some{r}:
      Some{(((h << 4n : U32) + l : U32)) <> r}
    case _ _ _:
      None{}

def Bytes.unhex.go(cs: List<&2, Char>) -> Maybe<&2, List<&2, U32>>:
  match cs:
    case Nil{}:
      Some{Nil{}}
    case Con{a, Con{b, rest}}:
      Bytes.unhex.join(Bytes.nibble(Char.to_u32(a)), Bytes.nibble(Char.to_u32(b)), Bytes.unhex.go(rest))
    case _:
      None{}

def Bytes.unhex(s: String) -> Maybe<&2, List<&2, U32>>:
  Bytes.unhex.go(String.to_list(s))

# Bytes as ASCII text.
def Bytes.text(xs: List<&2, U32>) -> String:
  match xs:
    case Nil{}:
      ""
    case Con{h, t}:
      String.from_list({[Char.from_u32(h)] : List<&2, Char>}) ++ Bytes.text(t)
