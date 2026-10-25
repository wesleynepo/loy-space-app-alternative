package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type Uploads struct {
	dir string
	mu  sync.Mutex
}

type UploadEntry struct {
	Key string `json:"key"`
	TS  int64  `json:"ts"`
}

func (u *Uploads) Load() map[string]UploadEntry {
	ix, _ := u.load()
	return ix
}

// load treats a missing index as empty but reports an unreadable or corrupt one, so update never rewrites it from nothing.
func (u *Uploads) load() (map[string]UploadEntry, error) {
	ix := map[string]UploadEntry{}
	b, err := os.ReadFile(filepath.Join(u.dir, "index.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return ix, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &ix); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", filepath.Join(u.dir, "index.json"), err)
	}
	return ix, nil
}

func (u *Uploads) PreviewPath(slot int) string {
	return filepath.Join(u.dir, fmt.Sprintf("slot%d.png", slot))
}

func (u *Uploads) update(fn func(map[string]UploadEntry)) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	ix, err := u.load()
	if err != nil {
		return err
	}
	fn(ix)
	b, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(u.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(u.dir, "index-*.json")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(u.dir, "index.json"))
}

func (u *Uploads) record(slot int, rgb []byte, key string, ts int64) error {
	if err := u.savePreview(slot, rgb); err != nil {
		return err
	}
	return u.update(func(ix map[string]UploadEntry) { ix[fmt.Sprint(slot)] = UploadEntry{key, ts} })
}

func (u *Uploads) forget(slot int) error {
	if err := os.Remove(u.PreviewPath(slot)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
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
	err = png.Encode(f, img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
