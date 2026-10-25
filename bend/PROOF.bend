import Base
import ./LAWS.bend as L
import ./bytes.bend as B
import ./frame.bend as F

# Proofs for LAWS.bend. `bend PROOF.bend` must print ALL PROOFS CHECK before a commit.

# Induction on xs: the recursive call at acc + h is the step.
def L.sum_append(xs, ys, acc):
  match xs:
    case Nil{}:
      {==}
    case Con{h, t}:
      L.sum_append(t, ys, (acc + h : U32))

def L.length_append(xs, ys):
  match xs:
    case Nil{}:
      {==}
    case Con{h, t}:
      %L.length_append(t, ys) : {1n+List.length(&2, U32, List.append(&2, U32, t, ys)) == 1n+_ : Nat}
      {==}

def L.length_trailer(xs, a, b):
  match xs:
    case Nil{}:
      {==}
    case Con{h, t}:
      %L.length_trailer(t, a, b) : {Nat.add(2n, 1n+List.length(&2, U32, t)) == 1n+_ : Nat}
      {==}

# The 4 magic + 5 header bytes come before the body, the 2 checksum bytes after it.
def L.build_length(seq, dir, body):
  %L.length_trailer(
      body,
      B.Bytes.lo(F.Frame.checksum(F.Frame.head(seq, dir, body))),
      B.Bytes.hi(F.Frame.checksum(F.Frame.head(seq, dir, body))))
    : {9n+_ == Nat.add(11n, List.length(&2, U32, body)) : Nat}
  {==}
