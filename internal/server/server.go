package server

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/wesleynepo/led-panel/internal/store"
	"github.com/wesleynepo/led-panel/panel"
	"github.com/wesleynepo/led-panel/web"
)

type Server struct {
	link  *panel.Link
	store *store.Store
}

func New(link *panel.Link, store *store.Store) *Server { return &Server{link: link, store: store} }

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", s.index)
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, _ *http.Request) { writeJSON(w, 200, s.link.Status()) })
	mux.HandleFunc("GET /api/state", s.state)
	mux.HandleFunc("GET /api/slots", s.slots)
	mux.HandleFunc("GET /img/{slot}", s.image)

	mux.HandleFunc("POST /api/connect", s.post(func(args) error { return s.link.Connect() }))
	mux.HandleFunc("POST /api/disconnect", s.post(func(args) error { return s.link.Disconnect() }))
	mux.HandleFunc("POST /api/image", s.post(func(a args) error {
		i, err := need(a.Index, "index")
		if err != nil {
			return err
		}
		return s.link.ShowImage(i)
	}))
	mux.HandleFunc("POST /api/upload", s.upload(func(a args) (panel.Uploaded, error) {
		rgb, err := base64.StdEncoding.DecodeString(a.RGB)
		if err != nil {
			return panel.Uploaded{}, err
		}
		return s.link.Upload(a.Slot, rgb)
	}))
	mux.HandleFunc("POST /api/upload-text", s.upload(func(a args) (panel.Uploaded, error) {
		return s.link.UploadText(panel.TextRequest{Slot: a.Slot, Text: a.Text, FG: a.FG, BG: a.BG})
	}))
	mux.HandleFunc("POST /api/delete", s.post(func(a args) error {
		slot, err := need(a.Slot, "slot")
		if err != nil {
			return err
		}
		if err := s.link.Delete(slot); err != nil {
			return err
		}
		return s.store.Forget(store.Slot(slot))
	}))
	mux.HandleFunc("POST /api/power", s.post(func(a args) error { return s.link.SetPower(a.On) }))
	mux.HandleFunc("POST /api/rotation", s.post(func(a args) error {
		d, err := need(a.Degrees, "degrees")
		if err != nil {
			return err
		}
		return s.link.SetRotation(d)
	}))
	mux.HandleFunc("POST /api/brightness", s.post(func(a args) error {
		lv, err := need(a.Level, "level")
		if err != nil {
			return err
		}
		return s.link.SetBrightness(lv)
	}))
	return localOnly(mux)
}

func (s *Server) indexHTML() []byte {
	if b, err := os.ReadFile(filepath.Join("web", "index.html")); err == nil {
		return b
	}
	return web.Index
}

func writeJSON(w http.ResponseWriter, code int, obj any) {
	b, _ := json.Marshal(obj)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(b)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

func isLocal(host string) bool { return host == "127.0.0.1" || host == "localhost" }

// localOnly rejects requests whose Host or Origin is not this machine, which stops DNS rebinding and cross-site calls.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.Host)
		if err != nil {
			host = r.Host
		}
		ok := isLocal(host)
		if origin := r.Header.Get("Origin"); ok && origin != "" {
			u, err := url.Parse(origin)
			ok = err == nil && isLocal(u.Hostname())
		}
		if !ok {
			fail(w, 403, errors.New("forbidden"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) index(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(s.indexHTML())
}

func (s *Server) state(w http.ResponseWriter, _ *http.Request) {
	st, err := s.link.ReadState()
	if err != nil {
		fail(w, 400, err)
		return
	}
	writeJSON(w, 200, st)
}

func (s *Server) slots(w http.ResponseWriter, _ *http.Request) {
	slots, err := s.allSlots()
	if err != nil {
		fail(w, 500, err)
		return
	}
	resp := map[string]any{"slots": slots, "current": nil, "error": nil, "used": nil, "total": panel.TotalSlots}
	if s.link.Connected() {
		if cur, keys, err := s.link.PanelState(); err != nil {
			resp["error"] = err.Error()
		} else {
			resp["current"], resp["used"] = cur, len(keys)
			resp["slots"] = withPanelKeys(slots, keys)
		}
	}
	writeJSON(w, 200, resp)
}

func (s *Server) image(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("slot")
	n, err := strconv.Atoi(raw)
	if err != nil || strconv.Itoa(n) != raw {
		http.NotFound(w, r)
		return
	}
	body, err := s.store.Preview(store.Slot(n))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "max-age=3600")
	_, _ = w.Write(body)
}

// args is the union of every POST body; each route reads the fields it needs.
type args struct {
	Index   *int        `json:"index"`
	Slot    *int        `json:"slot"`
	RGB     string      `json:"rgb"`
	Text    string      `json:"text"`
	FG      panel.Color `json:"fg"`
	BG      panel.Color `json:"bg"`
	On      bool        `json:"on"`
	Degrees *int        `json:"degrees"`
	Level   *int        `json:"level"`
}

func need(p *int, name string) (int, error) {
	if p == nil {
		return 0, fmt.Errorf("missing %q", name)
	}
	return *p, nil
}

const maxBody = 1 << 20

// decode reads a POST body. Requiring a JSON content type forces a CORS preflight on cross-site pages, so it doubles as
// the CSRF guard. An empty body is fine.
func decode(w http.ResponseWriter, r *http.Request) (args, bool) {
	var a args
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		fail(w, 403, errors.New("forbidden"))
		return a, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil && !errors.Is(err, io.EOF) {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, 413, errors.New("request body too large"))
		} else {
			fail(w, 400, err)
		}
		return a, false
	}
	return a, true
}

func (s *Server) post(fn func(args) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if a, ok := decode(w, r); ok {
			s.reply(w, fn(a))
		}
	}
}

// upload answers an upload and, when the panel took it, remembers it locally.
func (s *Server) upload(fn func(args) (panel.Uploaded, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, ok := decode(w, r)
		if !ok {
			return
		}
		up, err := fn(a)
		if err == nil {
			err = s.store.Record(store.Slot(up.Slot), up.RGB, store.Entry{Key: up.Key, TS: up.TS})
		}
		if err != nil {
			s.reply(w, err)
			return
		}
		resp := s.link.Status()
		resp["slot"] = up.Slot
		writeJSON(w, 200, resp)
	}
}

func (s *Server) reply(w http.ResponseWriter, err error) {
	if err != nil {
		s.link.Note("error: %v", err)
		resp := s.link.Status()
		resp["error"] = err.Error()
		writeJSON(w, 400, resp)
		return
	}
	writeJSON(w, 200, s.link.Status())
}
