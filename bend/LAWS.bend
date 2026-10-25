import Base
import ./fixture.bend as X

# Laws the frame codec must keep. Written by a human; the proofs live in PROOF.bend.
# They are concrete fixtures: Bytes sits on arrays, so a universal claim over every body would need array lemmas
# that bend-kit does not have. The golden vectors in test.bend cover the same ground across many more frames.

# LAW: the delete frame the LOY SPACE app sent in a real capture (slot 0, seq 12) is built byte for byte.
law delete_frame_matches_capture:
  {X.Fixture.build(12, 193, "0208020000") == "aa55ffff0a000c00c10208020000e003" : String}

# LAW: parsing that frame gives back its seq, dir and body.
law delete_frame_parses:
  {X.Fixture.parse("aa55ffff0a000c00c10208020000e003") == "12 193 0208020000" : String}

# LAW: a frame whose checksum is wrong is refused.
law bad_checksum_refused:
  {X.Fixture.parse("aa55ffff0a000c00c10208020000e004") == "refused" : String}

# LAW: a frame with a byte missing or an extra byte is refused.
law wrong_length_refused:
  {Bool.and(String.eq(X.Fixture.parse("aa55ffff0a000c00c10208020000e0"), "refused"), String.eq(X.Fixture.parse("aa55ffff0a000c00c10208020000e00300"), "refused")) == True{} : Bool}
