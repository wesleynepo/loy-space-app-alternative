package server

import "testing"

func TestWithPanelKeys(t *testing.T) {
	local := []slotInfo{{Slot: 6, Type: "upload", Key: "aa"}, {Slot: 7, Type: "upload", Key: "bb"}}
	got := withPanelKeys(local, map[int]string{0: "cc", 6: "aa", 7: "zz"})

	want := []struct {
		slot  int
		typ   string
		match bool
	}{{0, "unknown", true}, {6, "upload", true}, {7, "upload", false}}
	if len(got) != len(want) {
		t.Fatalf("got %d slots, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Slot != w.slot || got[i].Type != w.typ || *got[i].Match != w.match {
			t.Errorf("slot %d: got %+v, want %+v", i, got[i], w)
		}
	}
}
