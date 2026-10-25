package store

import (
	"os"
	"path/filepath"
	"testing"
)

var blank = make([]byte, 64*64*3)

func TestRecordAndForget(t *testing.T) {
	s := New(filepath.Join(t.TempDir(), "up"))
	if err := s.Record(7, blank, Entry{Key: "aa", TS: 1}); err != nil {
		t.Fatal(err)
	}
	ix, err := s.Entries()
	if err != nil || ix[7].Key != "aa" {
		t.Fatalf("after record: %+v, %v", ix, err)
	}
	if _, err := s.Preview(7); err != nil {
		t.Fatalf("preview missing: %v", err)
	}
	if err := s.Forget(7); err != nil {
		t.Fatal(err)
	}
	if ix, _ := s.Entries(); len(ix) != 0 {
		t.Fatalf("slot still indexed: %+v", ix)
	}
	if err := s.Forget(7); err != nil {
		t.Fatalf("forgetting twice must converge: %v", err)
	}
}

func TestIndexIsKeyedBySlotNumber(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.json"), []byte(`{"6":{"key":"bb","ts":2}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ix, err := New(dir).Entries()
	if err != nil || ix[6].Key != "bb" {
		t.Fatalf("got %+v, %v", ix, err)
	}
}

func TestKeepCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.json")
	if err := os.WriteFile(index, []byte(`{"6":`), 0o644); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	if _, err := s.Entries(); err == nil {
		t.Fatal("a corrupt index must be reported")
	}
	if err := s.Record(7, blank, Entry{Key: "aa", TS: 1}); err == nil {
		t.Fatal("record on a corrupt index must fail")
	}
	if b, _ := os.ReadFile(index); string(b) != `{"6":` {
		t.Fatalf("corrupt index was overwritten: %q", b)
	}
}
