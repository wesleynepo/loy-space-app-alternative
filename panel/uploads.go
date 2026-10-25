package panel

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sync"
)

// Uploads is the on-disk record of the pictures we sent: a preview per slot and an index of their keys.
type Uploads struct {
	dir string
	mu  sync.Mutex
}

type UploadEntry struct {
	Key string `json:"key"`
	TS  int64  `json:"ts"`
}

// Load returns the index keyed by slot number.
func (u *Uploads) Load() map[string]UploadEntry {
	ix := map[string]UploadEntry{}
	if b, err := os.ReadFile(filepath.Join(u.dir, "index.json")); err == nil {
		json.Unmarshal(b, &ix)
	}
	return ix
}

// PreviewPath is where the picture sent to a slot is kept.
func (u *Uploads) PreviewPath(slot int) string {
	return filepath.Join(u.dir, fmt.Sprintf("slot%d.png", slot))
}

func (u *Uploads) update(fn func(map[string]UploadEntry)) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	ix := u.Load()
	fn(ix)
	b, _ := json.Marshal(ix)
	return os.WriteFile(filepath.Join(u.dir, "index.json"), b, 0o644)
}

func (u *Uploads) record(slot int, rgb []byte, key string, ts int64) error {
	if err := u.savePreview(slot, rgb); err != nil {
		return err
	}
	return u.update(func(ix map[string]UploadEntry) { ix[fmt.Sprint(slot)] = UploadEntry{key, ts} })
}

func (u *Uploads) forget(slot int) error {
	os.Remove(u.PreviewPath(slot))
	return u.update(func(ix map[string]UploadEntry) { delete(ix, fmt.Sprint(slot)) })
}

func (u *Uploads) savePreview(slot int, rgb []byte) error {
	if err := os.MkdirAll(u.dir, 0o755); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < 64*64; i++ {
		img.Set(i%64, i/64, color.RGBA{rgb[3*i], rgb[3*i+1], rgb[3*i+2], 255})
	}
	f, err := os.Create(u.PreviewPath(slot))
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
