package server

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

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
	mux.HandleFunc("GET /", s.handleGet)
	mux.HandleFunc("POST /", s.handlePost)
	return mux
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

func localOnly(r *http.Request) bool {
	host := r.Host
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	if !isLocal(host) {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		return err == nil && isLocal(u.Hostname())
	}
	return true
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	if !localOnly(r) {
		fail(w, 403, errors.New("forbidden"))
		return
	}
	switch path := r.URL.Path; {
	case path == "/api/status":
		writeJSON(w, 200, s.link.Status())
	case path == "/api/state":
		st, err := s.link.ReadState()
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, st)
	case path == "/api/slots":
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
	case strings.HasPrefix(path, "/img/"):
		n, err := strconv.Atoi(path[5:])
		if err != nil {
			fail(w, 404, err)
			return
		}
		body, err := s.store.Preview(store.Slot(n))
		if err != nil {
			fail(w, 404, err)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(body)
	case path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(s.indexHTML())
	default:
		fail(w, 404, errors.New("not found"))
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

// recorded answers an upload and, when the panel took it, remembers it locally.
func (s *Server) recorded(w http.ResponseWriter, up panel.Uploaded, err error) {
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

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	if !localOnly(r) || r.Header.Get("Content-Type") != "application/json" {
		fail(w, 403, errors.New("forbidden"))
		return
	}
	reply := func(err error) { s.reply(w, err) }
	var args struct {
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
	if body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20)); len(bytes.TrimSpace(body)) > 0 {
		if err := json.Unmarshal(body, &args); err != nil {
			reply(err)
			return
		}
	}
	need := func(p *int, name string) (int, error) {
		if p == nil {
			return 0, fmt.Errorf("missing %q", name)
		}
		return *p, nil
	}
	switch r.URL.Path {
	case "/api/connect":
		reply(s.link.Connect())
	case "/api/disconnect":
		reply(s.link.Disconnect())
	case "/api/image":
		i, err := need(args.Index, "index")
		if err == nil {
			err = s.link.ShowImage(i)
		}
		reply(err)
	case "/api/upload":
		rgb, err := base64.StdEncoding.DecodeString(args.RGB)
		if err != nil {
			reply(err)
			return
		}
		up, err := s.link.Upload(args.Slot, rgb)
		s.recorded(w, up, err)
	case "/api/upload-text":
		up, err := s.link.UploadText(panel.TextRequest{Slot: args.Slot, Text: args.Text, FG: args.FG, BG: args.BG})
		s.recorded(w, up, err)
	case "/api/delete":
		slot, err := need(args.Slot, "slot")
		if err == nil {
			err = s.link.Delete(slot)
		}
		if err == nil {
			err = s.store.Forget(store.Slot(slot))
		}
		reply(err)
	case "/api/power":
		reply(s.link.SetPower(args.On))
	case "/api/rotation":
		d, err := need(args.Degrees, "degrees")
		if err == nil {
			err = s.link.SetRotation(d)
		}
		reply(err)
	case "/api/brightness":
		lv, err := need(args.Level, "level")
		if err == nil {
			err = s.link.SetBrightness(lv)
		}
		reply(err)
	default:
		fail(w, 404, errors.New("not found"))
	}
}
