// LOY SPACE / YS-series BLE LED panel framing (reverse-engineered from a PacketLogger capture).
//
// Frame: aa55ffff | len u16le | seq u16le | dir u8 | body... | sum u16le
//
//	len = number of bytes after the len field (seq .. sum)
//	sum = sum of every preceding byte of the frame, mod 65536
//	dir = 0xc1 app->panel, 0x81 panel->app
//
// App bodies start with (group, cmd), e.g. 02 06 02 00 09. Replies echo the sequence number.
package main

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var magic = []byte{0xaa, 0x55, 0xff, 0xff}

const (
	dirApp   = 0xC1
	dirPanel = 0x81
)

// GATT: write commands to FFF2 (write-without-response), replies arrive as notifications on FFF1.
// The app subscribes to a second notify characteristic as well.
const (
	writeUUID   = "0000fff2-0000-1000-8000-00805f9b34fb"
	notifyUUID  = "0000fff1-0000-1000-8000-00805f9b34fb"
	notify2UUID = "5833ff03-9b8b-5191-6142-22a4536ef123"
)

func sum16(b []byte) uint16 {
	var s uint32
	for _, c := range b {
		s += uint32(c)
	}
	return uint16(s)
}

func build(seq uint16, body []byte) []byte {
	f := append([]byte{}, magic...)
	f = binary.LittleEndian.AppendUint16(f, uint16(len(body)+5))
	f = binary.LittleEndian.AppendUint16(f, seq)
	f = append(f, dirApp)
	f = append(f, body...)
	return binary.LittleEndian.AppendUint16(f, sum16(f))
}

type Frame struct {
	Seq  uint16
	Dir  byte
	Body []byte
}

func parse(frame []byte) (Frame, error) {
	if len(frame) < 11 || !bytes.Equal(frame[:4], magic) {
		return Frame{}, errors.New("bad magic")
	}
	length := int(binary.LittleEndian.Uint16(frame[4:]))
	if len(frame) != 6+length {
		return Frame{}, fmt.Errorf("length mismatch: header says %d, got %d", length, len(frame)-6)
	}
	if binary.LittleEndian.Uint16(frame[len(frame)-2:]) != sum16(frame[:len(frame)-2]) {
		return Frame{}, errors.New("bad checksum")
	}
	return Frame{binary.LittleEndian.Uint16(frame[6:]), frame[8], frame[9 : len(frame)-2]}, nil
}

// Reassembler: frames larger than the BLE write/notify size arrive in several chunks.
type Reassembler struct{ buf []byte }

func (r *Reassembler) Feed(chunk []byte) [][]byte {
	if bytes.HasPrefix(chunk, magic) {
		r.buf = append([]byte{}, chunk...)
	} else {
		r.buf = append(r.buf, chunk...)
	}
	var out [][]byte
	for len(r.buf) >= 6 {
		total := 6 + int(binary.LittleEndian.Uint16(r.buf[4:]))
		if len(r.buf) < total {
			break
		}
		out = append(out, r.buf[:total:total])
		r.buf = r.buf[total:]
	}
	return out
}

// deviceInfo is group 03, cmd 0a: the app's first command; the reply carries model, firmware and panel size.
func deviceInfo(seq uint16) []byte {
	return build(seq, []byte{0x03, 0x0a, 0x00, 0x1b, 0x00, 0x04, 0x00, 0x06, 0x00})
}

// showImage is group 02, cmd 36: payload 01 <index> 00 followed by the fixed list 01..3b the app always sends.
func showImage(seq uint16, index int) []byte {
	body := []byte{0x02, 0x36, 0x3E, 0x01, byte(index), 0x00}
	for i := 1; i < 0x3C; i++ {
		body = append(body, byte(i))
	}
	return build(seq, body)
}

// query is a group 03 read-only query. Replies are 83 <cmd> <len> <payload>; the value is the last byte.
// Seen: 04 power (1=on), 06 brightness, 2f rotation (0-3), 1b size (w,h u16le at payload 1..4).
func query(seq uint16, cmd byte) []byte { return build(seq, []byte{0x03, cmd, 0x00}) }

// setPower is group 02, cmd 04: payload 00 <1|0>.
func setPower(seq uint16, on bool) []byte {
	v := byte(0)
	if on {
		v = 1
	}
	return build(seq, []byte{0x02, 0x04, 0x02, 0x00, v})
}

// setRotation is group 02, cmd 2f: one byte, 0/1/2/3 for 0/90/180/270 degrees (app code, `setLedRotate`).
func setRotation(seq uint16, quarterTurns int) []byte {
	return build(seq, []byte{0x02, 0x2F, 0x01, byte(quarterTurns & 3)})
}

// setBrightness is group 02, cmd 06, payload 00 <level>.
func setBrightness(seq uint16, level int) []byte {
	return build(seq, []byte{0x02, 0x06, 0x02, 0x00, byte(level)})
}

// deleteSlot is group 02, cmd 08, payload 00 <slot>. The panel acks, then pushes an unsolicited `06 36 02 01 NN` frame.
func deleteSlot(seq uint16, slot int) []byte {
	return build(seq, []byte{0x02, 0x08, 0x02, 0x00, byte(slot)})
}

const (
	chunkSize = 960
	gamma     = 1.6 // fitted from the app's own upload; the app builds a ((a+.5)/256)^g lookup table
)

// packImage makes a YSTP01 image: 15-byte header + raw 24-bit RGB rows, top-down.
func packImage(rgb []byte, w, h int) ([]byte, error) {
	if len(rgb) != w*h*3 {
		return nil, fmt.Errorf("expected %d RGB bytes, got %d", w*h*3, len(rgb))
	}
	var table [256]byte
	for a := range table {
		table[a] = byte(int(256*math.Pow((float64(a)+0.5)/256, gamma) - 0.5))
	}
	out := []byte("YSTP01")
	for _, v := range []int{w, h, 1, w} {
		out = binary.LittleEndian.AppendUint16(out, uint16(v))
	}
	out = append(out, 24)
	for _, c := range rgb {
		out = append(out, table[c])
	}
	return out, nil
}

func tlv(tag byte, value ...byte) []byte { return append([]byte{tag, byte(len(value))}, value...) }

func u16(vs ...int) []byte {
	var b []byte
	for _, v := range vs {
		b = binary.LittleEndian.AppendUint16(b, uint16(v))
	}
	return b
}

func lenTLV(n int) []byte {
	if n < 128 {
		return []byte{byte(n)}
	}
	return append([]byte{0x82}, u16(n)...)
}

func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// makeKey makes the 20-byte slot key: (idPro + '0000000')[:8] as hex bytes + 16 bytes. The app uses md5 of its
// program JSON; we can't reproduce that, so ours is md5(data + timestamp), which only needs to be unique per upload.
func makeKey(idPro int, packed []byte, ts uint32) []byte {
	var prefix [4]byte
	s := fmt.Sprintf("%x0000000", idPro)[:8]
	for i := range prefix {
		fmt.Sscanf(s[2*i:2*i+2], "%02x", &prefix[i])
	}
	sum := md5.Sum(binary.LittleEndian.AppendUint32(append([]byte{}, packed...), ts))
	return append(prefix[:], sum[:]...)
}

// uploadBodies returns bodies (seq assigned by the caller) for: start, item info, data chunks, finish.
// Layout from the app's code (tags 08 start, 1c play info, 1d region rect, 12/13 chunk, 35 finish);
// the item-info frame (tags 14 and 11) is copied from the capture and only verified for 64x64 24-bit.
func uploadBodies(slot int, packed, key []byte, ts uint32, w, h int) [][]byte {
	s := byte(slot)
	hdr := cat(tlv(0x09, 1), tlv(0x0C, s), tlv(0x0D, 0))
	start := cat([]byte{0x02}, tlv(0x08, 0, s), tlv(0x09, 1), tlv(0x0C, s),
		tlv(0x1C, 0, 0, 1, 0, 3, 0),                                                     // type_bg, loop mode, count 1, time_sync 3
		tlv(0x0D, 0), tlv(0x1D, append([]byte{0, 0, 0, 0}, append(u16(w, h), 0)...)...)) // region 0, rect x y w h bg
	item := cat([]byte{0x02}, hdr, tlv(0x0E, 0), tlv(0x14, 1, 0x0D, 3), tlv(0x11, 0, 1, 0, 10))
	total := (len(packed) + chunkSize - 1) / chunkSize
	out := [][]byte{start, item}
	for i := 0; i < total; i++ {
		data := packed[i*chunkSize : min((i+1)*chunkSize, len(packed))]
		out = append(out, cat([]byte{0x02}, hdr, tlv(0x0E, 0),
			[]byte{0x12, 7}, u16(total, i, chunkSize), []byte{0x00},
			[]byte{0x13}, lenTLV(len(data)), data))
	}
	finish := cat([]byte{0x02, 0x35, 0x19, s}, key, binary.LittleEndian.AppendUint32(nil, ts))
	return append(out, finish)
}
