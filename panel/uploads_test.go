package panel

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUploadsRecordAndForget(t *testing.T) {
	u := &Uploads{dir: filepath.Join(t.TempDir(), "up")}
	if err := u.record(7, make([]byte, 64*64*3), "aa", 1); err != nil {
		t.Fatal(err)
	}
	if got := u.Load()["7"]; got.Key != "aa" {
		t.Fatalf("after record: %+v", got)
	}
	if _, err := os.Stat(u.PreviewPath(7)); err != nil {
		t.Fatalf("preview missing: %v", err)
	}
	if err := u.forget(7); err != nil {
		t.Fatal(err)
	}
	if len(u.Load()) != 0 {
		t.Fatalf("slot still indexed: %+v", u.Load())
	}
	if err := u.forget(7); err != nil {
		t.Fatalf("forgetting twice must converge: %v", err)
	}
}

func TestUploadsKeepCorruptIndex(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "index.json")
	if err := os.WriteFile(index, []byte(`{"6":`), 0o644); err != nil {
		t.Fatal(err)
	}
	u := &Uploads{dir: dir}
	if err := u.record(7, make([]byte, 64*64*3), "aa", 1); err == nil {
		t.Fatal("record on a corrupt index must fail")
	}
	if b, _ := os.ReadFile(index); string(b) != `{"6":` {
		t.Fatalf("corrupt index was overwritten: %q", b)
	}
}
