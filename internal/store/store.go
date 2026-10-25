// Package store keeps what this app has uploaded to the panel: an index of slot keys and a PNG preview per slot.
package store

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

// Slot is a panel program slot. It keys the index, which encoding/json writes as the decimal string.
type Slot int

type Entry struct {
	Key string `json:"key"`
	TS  int64  `json:"ts"`
}

type Store struct {
	dir string
	mu  sync.Mutex
}

func New(dir string) *Store { return &Store{dir: dir} }

func (s *Store) indexPath() string { return filepath.Join(s.dir, "index.json") }

func (s *Store) previewPath(slot Slot) string {
	return filepath.Join(s.dir, fmt.Sprintf("slot%d.png", slot))
}

// Entries returns every recorded upload. A missing index is empty. An unreadable or corrupt one is an error.
func (s *Store) Entries() (map[Slot]Entry, error) {
	ix := map[Slot]Entry{}
	b, err := os.ReadFile(s.indexPath())
	if errors.Is(err, fs.ErrNotExist) {
		return ix, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &ix); err != nil {
		return nil, fmt.Errorf("corrupt %s: %w", s.indexPath(), err)
	}
	return ix, nil
}

// Preview returns the PNG saved for the slot.
func (s *Store) Preview(slot Slot) ([]byte, error) { return os.ReadFile(s.previewPath(slot)) }

// Record saves a 64x64 RGB preview and indexes the slot.
func (s *Store) Record(slot Slot, rgb []byte, e Entry) error {
	if err := s.savePreview(slot, rgb); err != nil {
		return err
	}
	return s.update(func(ix map[Slot]Entry) { ix[slot] = e })
}

// Forget removes the slot's preview and index entry. Forgetting an unknown slot is not an error.
func (s *Store) Forget(slot Slot) error {
	if err := os.Remove(s.previewPath(slot)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return s.update(func(ix map[Slot]Entry) { delete(ix, slot) })
}

func (s *Store) update(fn func(map[Slot]Entry)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	ix, err := s.Entries()
	if err != nil {
		return err
	}
	fn(ix)
	b, err := json.Marshal(ix)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "index-*.json")
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
	return os.Rename(tmp.Name(), s.indexPath())
}

func (s *Store) savePreview(slot Slot, rgb []byte) error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < 64*64; i++ {
		img.Set(i%64, i/64, color.RGBA{rgb[3*i], rgb[3*i+1], rgb[3*i+2], 255})
	}
	f, err := os.Create(s.previewPath(slot))
	if err != nil {
		return err
	}
	err = png.Encode(f, img)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}
