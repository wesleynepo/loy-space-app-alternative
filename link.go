package main

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tinygo.org/x/bluetooth"
)

const (
	panelName  = "YS524A250818"
	writeMax   = 180 // largest BLE write the app uses (MTU 185 minus overhead)
	totalSlots = 60  // size of the panel's key table; the app refuses to send more programs than this
	// Slots 0-5 hold the app's content and are never overwritten.
	firstFreeSlot, lastSlot = 6, 9
)

// firstSeq is the first sequence number the link uses; the app's own frames sit below it (device info 41, show image 11).
const firstSeq = 50

var errNotConnected = errors.New("not connected")

// PanelLink is one persistent BLE connection. Every panel exchange holds mu, so frames never interleave.
type PanelLink struct {
	mu      sync.Mutex // serializes panel traffic
	adapter *bluetooth.Adapter
	device  bluetooth.Device
	write   bluetooth.DeviceCharacteristic
	up      atomic.Bool // written under mu; readable without it so status polling never waits on an upload
	infoVal atomic.Value
	asm     Reassembler
	replies chan Frame
	seq     atomic.Uint32

	logMu sync.Mutex
	log   []string
}

func newLink() *PanelLink {
	l := &PanelLink{replies: make(chan Frame, 64)}
	l.seq.Store(firstSeq - 1)
	return l
}

func (l *PanelLink) nextSeq() uint16 { return uint16(l.seq.Add(1)) }

func (l *PanelLink) note(format string, a ...any) {
	l.logMu.Lock()
	defer l.logMu.Unlock()
	l.log = append(l.log, time.Now().Format("15:04:05 ")+fmt.Sprintf(format, a...))
	if len(l.log) > 200 {
		l.log = l.log[len(l.log)-200:]
	}
}

func short(b []byte) string {
	s := hex.EncodeToString(b)
	return s[:min(len(s), 120)]
}

func (l *PanelLink) onNotify(data []byte) {
	for _, raw := range l.asm.Feed(data) {
		l.note("<- %s", short(raw))
		f, err := parse(raw)
		if err != nil {
			l.note("bad frame: %v", err)
			continue
		}
		select {
		case l.replies <- f:
		default:
		}
	}
}

func (l *PanelLink) Connected() bool { return l.up.Load() }

func (l *PanelLink) status() map[string]any {
	l.logMu.Lock()
	defer l.logMu.Unlock()
	info, _ := l.infoVal.Load().(string)
	return map[string]any{"connected": l.up.Load(), "info": info, "log": append([]string{}, l.log...)}
}

func (l *PanelLink) Connect() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.up.Load() {
		return nil
	}
	if l.adapter == nil {
		l.adapter = bluetooth.DefaultAdapter
		if err := l.adapter.Enable(); err != nil {
			l.adapter = nil
			return fmt.Errorf("bluetooth: %w", err)
		}
	}
	var addr bluetooth.Address
	found := false
	for attempt := 1; attempt <= 3 && !found; attempt++ {
		timer := time.AfterFunc(10*time.Second, func() { l.adapter.StopScan() })
		l.adapter.Scan(func(a *bluetooth.Adapter, r bluetooth.ScanResult) {
			if r.LocalName() == panelName {
				addr, found = r.Address, true
				a.StopScan()
			}
		})
		timer.Stop()
		if !found {
			l.note("panel not seen (attempt %d/3)", attempt)
		}
	}
	if !found {
		return fmt.Errorf("%s not found - close LOY SPACE / power-cycle the panel", panelName)
	}
	dev, err := l.adapter.Connect(addr, bluetooth.ConnectionParams{ConnectionTimeout: bluetooth.NewDuration(20 * time.Second)})
	if err != nil {
		return err
	}
	if err := l.attach(dev); err != nil {
		dev.Disconnect()
		return err
	}
	l.up.Store(true)
	reply, err := l.send(deviceInfo(41), 41)
	if err != nil {
		l.dropLocked()
		return err
	}
	if len(reply.Body) > 3 {
		parts := strings.Split(strings.Map(func(r rune) rune {
			if r < 0x80 {
				return r
			}
			return -1
		}, string(reply.Body[3:])), ",")
		l.infoVal.Store(strings.Join(parts[:min(3, len(parts))], ", "))
	}
	l.note("connected")
	return nil
}

func (l *PanelLink) attach(dev bluetooth.Device) error {
	l.device = dev
	services, err := dev.DiscoverServices(nil)
	if err != nil {
		return err
	}
	haveWrite := false
	for _, svc := range services {
		chars, err := svc.DiscoverCharacteristics(nil)
		if err != nil {
			continue
		}
		for _, c := range chars {
			switch u := c.UUID(); {
			case u.String() == writeUUID:
				l.write, haveWrite = c, true
			case u.String() == notify2UUID || u.String() == notifyUUID:
				if err := c.EnableNotifications(l.onNotify); err != nil {
					return fmt.Errorf("subscribe %s: %w", u, err)
				}
			}
		}
	}
	if !haveWrite {
		return errors.New("write characteristic not found")
	}
	return nil
}

func (l *PanelLink) dropLocked() {
	l.up.Store(false)
	l.device.Disconnect()
}

func (l *PanelLink) Disconnect() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.up.Load() {
		l.dropLocked()
	}
	l.note("disconnected")
	return nil
}

// send writes one frame and waits for the reply with the same sequence number. Caller holds mu.
func (l *PanelLink) send(frame []byte, seq uint16) (Frame, error) {
	for len(l.replies) > 0 {
		<-l.replies
	}
	l.note("-> %s", short(frame))
	for off := 0; off < len(frame); off += writeMax { // the app splits big frames into <=180-byte writes
		if _, err := l.write.WriteWithoutResponse(frame[off:min(off+writeMax, len(frame))]); err != nil {
			return Frame{}, err
		}
		if len(frame) > writeMax {
			time.Sleep(20 * time.Millisecond)
		}
	}
	deadline := time.After(3 * time.Second)
	for {
		select {
		case f := <-l.replies:
			if f.Seq == seq {
				return f, nil
			}
		case <-deadline:
			return Frame{}, errors.New("no reply from panel")
		}
	}
}

// exchange runs fn with the link locked and connected.
func (l *PanelLink) exchange(fn func() error) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.up.Load() {
		return errNotConnected
	}
	return fn()
}

func (l *PanelLink) Command(build func(seq uint16) []byte, seq uint16) error {
	return l.exchange(func() error { _, err := l.send(build(seq), seq); return err })
}

// ReadState reads power, brightness and rotation via read-only queries.
func (l *PanelLink) ReadState() (map[string]any, error) {
	var out [3]byte
	err := l.exchange(func() error {
		for i, cmd := range []byte{0x04, 0x06, 0x2F} {
			seq := l.nextSeq()
			r, err := l.send(query(seq, cmd), seq)
			if err != nil {
				return err
			}
			out[i] = r.Body[len(r.Body)-1]
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return map[string]any{"power": out[0] != 0, "brightness": int(out[1]), "rotation": int(out[2]) * 90}, nil
}

var ack = []byte{0x82, 0x00}

// Upload sends a 64x64 RGB picture to a slot (start, item info, chunks, finish), one acked frame at a time.
func (l *PanelLink) Upload(slot int, rgb []byte) (string, error) {
	packed, err := packImage(rgb, 64, 64)
	if err != nil {
		return "", err
	}
	ts := uint32(time.Now().Unix())
	key := makeKey(slot+1, packed, ts)
	var frames int
	err = l.exchange(func() error {
		bodies := uploadBodies(slot, packed, key, ts, 64, 64)
		frames = len(bodies)
		for i, body := range bodies {
			seq := l.nextSeq()
			r, err := l.send(build(seq, body), seq)
			if err != nil {
				return err
			}
			if string(r.Body) != string(ack) {
				return fmt.Errorf("panel rejected frame %d/%d: %x", i+1, len(bodies), r.Body)
			}
		}
		// The panel falls back to slot 0 after an upload; show the new slot, as the app does.
		_, err := l.send(showImage(11, slot), 11)
		return err
	})
	if err != nil {
		return "", err
	}
	l.note("uploaded slot %d (%d frames)", slot, frames)
	if err := savePreview(slot, rgb); err != nil {
		return "", err
	}
	k := hex.EncodeToString(key)
	return k, updateIndex(func(ix map[string]uploadEntry) { ix[fmt.Sprint(slot)] = uploadEntry{k, int64(ts)} })
}

// Delete removes one of our uploads from the panel and forgets its preview.
func (l *PanelLink) Delete(slot int) error {
	err := l.exchange(func() error {
		seq := l.nextSeq()
		r, err := l.send(deleteSlot(seq, slot), seq)
		if err == nil && string(r.Body) != string(ack) {
			err = fmt.Errorf("panel rejected delete: %x", r.Body)
		}
		return err
	})
	if err != nil {
		return err
	}
	l.note("deleted slot %d", slot)
	os.Remove(filepath.Join(uploadsDir, fmt.Sprintf("slot%d.png", slot)))
	return updateIndex(func(ix map[string]uploadEntry) { delete(ix, fmt.Sprint(slot)) })
}

// PanelState returns the current slot (03 36) and the key stored in each slot (03 35), as the app asks for them.
func (l *PanelLink) PanelState() (int, map[int]string, error) {
	var cur int
	var lists [][]byte
	err := l.exchange(func() error {
		seq := l.nextSeq()
		r, err := l.send(build(seq, []byte{0x03, 0x36, 0x00}), seq)
		if err != nil {
			return err
		}
		cur = int(r.Body[4])
		for first := 0; first < totalSlots; first += 10 { // the app reads the table ten slots at a time
			seq := l.nextSeq()
			body := []byte{0x03, 0x35, 0x0A}
			for i := 0; i < 10; i++ {
				body = append(body, byte(first+i))
			}
			r, err := l.send(build(seq, body), seq)
			if err != nil {
				return err
			}
			lists = append(lists, r.Body)
		}
		return nil
	})
	if err != nil {
		return 0, nil, err
	}
	keys := map[int]string{}
	for _, lst := range lists {
		for off := 4; off+21 <= len(lst) && off < 4+int(lst[3]); off += 21 { // entry = slot id + 20-byte key; zeros = empty
			if key := lst[off+1 : off+21]; strings.Trim(string(key), "\x00") != "" {
				keys[int(lst[off])] = hex.EncodeToString(key)
			}
		}
	}
	return cur, keys, nil
}

type uploadEntry struct {
	Key string `json:"key"`
	TS  int64  `json:"ts"`
}

var indexMu sync.Mutex

func loadUploads() map[string]uploadEntry {
	ix := map[string]uploadEntry{}
	if b, err := os.ReadFile(filepath.Join(uploadsDir, "index.json")); err == nil {
		json.Unmarshal(b, &ix)
	}
	return ix
}

func updateIndex(fn func(map[string]uploadEntry)) error {
	indexMu.Lock()
	defer indexMu.Unlock()
	ix := loadUploads()
	fn(ix)
	b, _ := json.Marshal(ix)
	return os.WriteFile(filepath.Join(uploadsDir, "index.json"), b, 0o644)
}
