package panel

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestMatchesPython(t *testing.T) {
	raw, err := os.ReadFile("testdata_vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	hx := func(b []byte) string { return hex.EncodeToString(b) }
	sh := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	check := func(name, got string) {
		if v[name] != got {
			t.Errorf("%s:\n got %s\nwant %v", name, got, v[name])
		}
	}
	rgb := make([]byte, 64*64*3)
	for i := range rgb {
		rgb[i] = byte((i*7 + i/3) % 256)
	}
	packed, err := packImage(rgb, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	check("device_info", hx(deviceInfo(41)))
	check("show_image", hx(showImage(11, 7)))
	check("query", hx(query(50, 0x2f)))
	check("power", hx(setPower(51, true)))
	check("rotation", hx(setRotation(52, 3)))
	check("brightness", hx(setBrightness(53, 9)))
	check("delete", hx(deleteSlot(54, 7)))
	check("packed", sh(packed))
	for _, slot := range []int{6, 9} {
		key := makeKey(slot+1, packed, 1700000000)
		check(fmt.Sprintf("key%d", slot), hx(key))
		want := v[fmt.Sprintf("upload%d", slot)].([]any)
		bodies := uploadBodies(slot, packed, key, 1700000000, 64, 64)
		if len(bodies) != len(want) {
			t.Fatalf("slot %d: %d frames, want %d", slot, len(bodies), len(want))
		}
		for i, b := range bodies {
			if got := sh(build(uint16(60+i), b)); got != want[i] {
				t.Errorf("slot %d frame %d differs", slot, i)
			}
		}
	}
}

func TestParseAndReassemble(t *testing.T) {
	f := build(9, []byte{0x83, 0x04, 0x01, 0x01})
	var r Reassembler
	var got [][]byte
	for _, c := range [][]byte{f[:5], f[5:8], append(append([]byte{}, f[8:]...), f...)} {
		got = append(got, r.Feed(c)...)
	}
	if len(got) != 2 || !bytes.Equal(got[0], f) || !bytes.Equal(got[1], f) {
		t.Fatalf("reassembly got %x", got)
	}
	p, err := parse(got[0])
	if err != nil || p.Seq != 9 || p.Dir != dirApp || !bytes.Equal(p.Body, []byte{0x83, 0x04, 0x01, 0x01}) {
		t.Fatalf("parse: %+v %v", p, err)
	}
	f[len(f)-1] ^= 1
	if _, err := parse(f); err == nil {
		t.Fatal("bad checksum accepted")
	}
}

// capturedText is the frame the app sent for the text "Wesley Text\n" (white on black, slot 0, 64x64 panel).
const capturedText = "aa55ffff9a001400c102080200000901010c01001c060000010000000d01001d090000000040004000000901010c01000d01000e01001403000903110a000100060100100040000901010c01000d01000e0100120701000000c00300131d03000003010000ffffff000000010100105765736c657920546578740a180602000000010035190010000000609ffbd5bafa79c7422f5fe7da5e166d3649de6a4d1a"

func TestTextMatchesCapture(t *testing.T) {
	want, _ := hex.DecodeString(capturedText)
	key, _ := hex.DecodeString("10000000609ffbd5bafa79c7422f5fe7da5e166d")
	body, err := textBody(0, "Wesley Text\n", 0xffffff, 0, key, 0x6ade4936, 64, 64)
	if err != nil {
		t.Fatal(err)
	}
	if got := build(0x14, body); !bytes.Equal(got, want) {
		t.Errorf("got  %x\nwant %x", got, want)
	}
	if _, err := textBody(0, "caf\u00e9", 0xffffff, 0, key, 0, 64, 64); err == nil {
		t.Error("non-ASCII accepted")
	}
}
