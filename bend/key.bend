# Slot key for the LOY panel's finish frame: 20 bytes = 4-byte program prefix + md5(image data + timestamp).
import Base
import bend-kit-bytes@0.3.2.0/bytes.bend as Bytes
import bend-kit-crypto@0.2.2.1/crypto.bend as Crypto

# protocol.make_key: the prefix is the first 8 hex digits of (hex(id_pro) + "0000000"). That is id_pro << 4 in the
# first byte for 1..15, and id_pro itself for 16..255. Larger ids would need a second byte and are not supported.
def Key.prefix(+id_pro: U32) -> Bytes.Bytes:
  Bytes.set(Bytes.new(4), 0, Bool.pick(U32, U32.is_lt(id_pro, 16), (id_pro << 4n : U32), id_pro))

def Key.input(packed: Bytes.Bytes, ts: U32) -> Bytes.Bytes:
  Bytes.append(packed, Bytes.set.u32le(Bytes.new(4), 0, ts))

def Key.wrap(r: U32 & Array<U32>) -> IO(Bytes.Bytes):
  (n, words) = r
  IO.pure(Bytes.Bytes, Bytes.Bytes{n, words})

# md5 through OpenSSL libcrypto (set BEND_LIBCRYPTO if libcrypto.3 is not on the default path).
def Key.md5(b: Bytes.Bytes) -> IO(Bytes.Bytes):
  Bytes.Bytes{len, buf} = b
  do IO<Bytes.Bytes>:
    r : U32 & Array<U32> <- IO.try(U32 & Array<U32>, Crypto.digest.words("MD5", len, buf))
    Key.wrap(r)

def Key.join(prefix: Bytes.Bytes, md5: Bytes.Bytes) -> IO(Bytes.Bytes):
  IO.pure(Bytes.Bytes, Bytes.append(prefix, md5))

def Key.make.go(id_pro: U32, packed: Bytes.Bytes, ts: U32) -> IO(Bytes.Bytes):
  do IO<Bytes.Bytes>:
    md5 : Bytes.Bytes <- Key.md5(Key.input(packed, ts))
    Key.join(Key.prefix(id_pro), md5)

# Exits with code 22 for an id_pro outside 1..255 (the panel has 60 slots, so real ids are 1..60).
def Key.make.check(ok: Bool, id_pro: U32, packed: Bytes.Bytes, ts: U32) -> IO(Bytes.Bytes):
  match ok:
    case True{}:
      Key.make.go(id_pro, packed, ts)
    case False{}:
      IO.die(Bytes.Bytes, 22, "slot key: id_pro must be 1..255")

def Key.make(+id_pro: U32, packed: Bytes.Bytes, ts: U32) -> IO(Bytes.Bytes):
  Key.make.check(Bool.and(U32.is_ge(id_pro, 1), U32.is_le(id_pro, 255)), id_pro, packed, ts)
