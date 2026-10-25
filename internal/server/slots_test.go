package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wesleynepo/led-panel/internal/store"
	"github.com/wesleynepo/led-panel/panel"
)

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

func get(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "http://127.0.0.1:8765"+path, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestSlotsListUploadsWithoutPanel(t *testing.T) {
	st := store.New(t.TempDir())
	if err := st.Record(8, make([]byte, 64*64*3), store.Entry{Key: "k8", TS: 5}); err != nil {
		t.Fatal(err)
	}
	if err := st.Record(6, make([]byte, 64*64*3), store.Entry{Key: "k6", TS: 4}); err != nil {
		t.Fatal(err)
	}
	h := New(panel.New(), st).Handler()

	rec := get(t, h, "/api/slots")
	var resp struct {
		Slots []slotInfo `json:"slots"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || rec.Code != 200 {
		t.Fatalf("status %d, %v: %s", rec.Code, err, rec.Body)
	}
	if len(resp.Slots) != 2 || resp.Slots[0].Slot != 6 || resp.Slots[1].Key != "k8" || resp.Slots[1].Src != "/img/8?v=5" {
		t.Fatalf("slots: %+v", resp.Slots)
	}

	if rec := get(t, h, "/img/8"); rec.Code != 200 || rec.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("img/8: %d %s", rec.Code, rec.Header().Get("Content-Type"))
	}
	if rec := get(t, h, "/img/9"); rec.Code != 404 {
		t.Fatalf("img/9: %d", rec.Code)
	}
}
