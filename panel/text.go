package panel

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Text programs (LOY SPACE "text" item), decoded from trace-2.pklg and the app's `Q`/`F`/`ie` builders.
// The whole program fits one frame, so unlike an image there are no separate start/item/chunk frames:
//
//	02 | 08 02 00 slot | 09 01 01 | 0c 01 slot | 1c 06 00 00 01 00 00 00 | 0d 01 00 | 1d 09 0,0,w,h,0   start + region
//	   | 09 01 01 | 0c 01 slot | 0d 01 00 | 0e 01 00
//	   | 14 03 ani speed stay                                                                          animation
//	   | 11 0a 00 01 00 06 code font size 00 interval(u16)                                              text style
//	   | 09 01 01 | 0c 01 slot | 0d 01 00 | 0e 01 00 | 12 07 01 00 00 00 c0 03 00 | 13 len data     one chunk
//	   | 18 06 02 slot 00 00 01 00                                                                      show now
//	   | 35 19 slot key(20) ts(u32le)                                                                   finish
//
// data = 03 00 page | 03 01 line | 00 fg(3) bg(3) | 01 code font size | text bytes.
// The captured text ended in 0a (a newline); whether the app adds it or the text box had one is unknown.

// textFrameMax is the app's single-frame limit (`b` = 960 + 37); longer programs are split into frames.
const textFrameMax = 997

const (
	fontCode = 1  // `code` in the app's style defaults
	fontSize = 16 // pixel height of the glyphs
)

// textBody returns the body of the one frame that uploads text to slot and shows it. fg and bg are 24-bit colours written
// as three little-endian bytes (0xffffff is white). text must be ASCII; the app converts other text to GB.
func textBody(slot int, text string, fg, bg uint32, key []byte, ts uint32, w, h int) ([]byte, error) {
	for _, r := range text {
		if r > 0x7f {
			return nil, fmt.Errorf("text %q: only ASCII is supported", text)
		}
	}
	if len(key) != 20 {
		return nil, errors.New("key must be 20 bytes")
	}
	s := byte(slot)
	rgb := func(c uint32) []byte { return []byte{byte(c), byte(c >> 8), byte(c >> 16)} }
	data := cat(
		[]byte{0x03, 0x00, 0x00, 0x03, 0x01, 0x00}, // page and line alignment: top, left
		[]byte{0x00}, rgb(fg), rgb(bg),
		[]byte{0x01, fontCode, 0x00, fontSize},
		[]byte(text),
	)
	hdr := cat(tlv(0x09, 1), tlv(0x0C, s), tlv(0x0D, 0))
	body := cat([]byte{0x02},
		tlv(0x08, 0, s), tlv(0x09, 1), tlv(0x0C, s),
		tlv(0x1C, 0, 0, 1, 0, 0, 0),
		tlv(0x0D, 0), tlv(0x1D, append([]byte{0, 0, 0, 0}, append(u16(w, h), 0)...)...),
		hdr, tlv(0x0E, 0),
		tlv(0x14, 0, 9, 3), // animation type 0, speed 9, stays 3
		tlv(0x11, 0, 1, 0, 6, fontCode, 0, fontSize, 0, byte(w), byte(w>>8)),
		hdr, tlv(0x0E, 0),
		[]byte{0x12, 7}, u16(1, 0, chunkSize), []byte{0x00},
		[]byte{0x13}, lenTLV(len(data)), data,
		tlv(0x18, 2, s, 0, 0, 1, 0),
		[]byte{0x35, 0x19, s}, key, binary.LittleEndian.AppendUint32(nil, ts),
	)
	if len(body) > textFrameMax {
		return nil, fmt.Errorf("text program is %d bytes; multi-frame text is not implemented", len(body))
	}
	return body, nil
}
