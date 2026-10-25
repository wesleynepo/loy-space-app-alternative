import Base
import ./bytes.bend as B
import ./frame.bend as F

# Laws the frame codec must keep. Written by a human; the proofs live in PROOF.bend.

# LAW: the checksum is a running sum, so it splits over a join.
law sum_append:
  for xs: List<&2, U32>
  for ys: List<&2, U32>
  for acc: U32
  {B.Bytes.sum(List.append(&2, U32, xs, ys), acc) == B.Bytes.sum(ys, B.Bytes.sum(xs, acc)) : U32}

# LAW: joining two lists adds their lengths.
law length_append:
  for xs: List<&2, U32>
  for ys: List<&2, U32>
  {List.length(&2, U32, List.append(&2, U32, xs, ys)) == Nat.add(List.length(&2, U32, xs), List.length(&2, U32, ys)) : Nat}

# LAW: appending two bytes adds two to the length.
law length_trailer:
  for xs: List<&2, U32>
  for a: U32
  for b: U32
  {Nat.add(2n, List.length(&2, U32, xs)) == List.length(&2, U32, List.append(&2, U32, xs, {[a, b] : List<&2, U32>})) : Nat}

# LAW: a frame is the body plus 11 bytes of framing (magic 4, len 2, seq 2, dir 1, sum 2).
law build_length:
  for +seq: U32
  for +dir: U32
  for +body: List<&2, U32>
  {List.length(&2, U32, F.Frame.build(seq, dir, body)) == Nat.add(11n, List.length(&2, U32, body)) : Nat}
