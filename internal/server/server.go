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

	"github.com/wesleynepo/led-panel/panel"
	"github.com/wesleynepo/led-panel/web"
)

// Config locates the files the UI shows next to the panel's own state.
type Config struct {
	CacheDir string // downloaded program pictures
	// AppDoc is the LOY SPACE app's folder (iOS app on Apple silicon) with its per-panel slot list and local images.
	AppDoc string
}

// Server is the local web UI and JSON API over one panel link.
type Server struct {
	link *panel.Link
	cfg  Config
}

func New(link *panel.Link, cfg Config) *Server { return &Server{link: link, cfg: cfg} }

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
	w.Write(b)
}

func fail(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"error": err.Error()})
}

func isLocal(host string) bool { return host == "127.0.0.1" || host == "localhost" }

// localOnly blocks DNS-rebinding and cross-site requests from other web pages.
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
		slots := s.allSlots()
		resp := map[string]any{"slots": slots, "current": nil, "error": nil, "used": nil, "total": panel.TotalSlots}
		if s.link.Connected() {
			if cur, keys, err := s.link.PanelState(); err != nil {
				resp["error"] = err.Error()
			} else {
				resp["current"], resp["used"] = cur, len(keys)
				for i := range slots {
					m := keys[slots[i].Slot] == slots[i].Key
					slots[i].Match = &m
				}
			}
		}
		writeJSON(w, 200, resp)
	case strings.HasPrefix(path, "/img/"):
		n, err := strconv.Atoi(path[5:])
		if err != nil {
			fail(w, 404, err)
			return
		}
		body, ctype, err := s.slotImage(n)
		if err != nil {
			fail(w, 404, err)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write(body)
	case path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(s.indexHTML())
	default:
		fail(w, 404, errors.New("not found"))
	}
}

func (s *Server) handlePost(w http.ResponseWriter, r *http.Request) {
	if !localOnly(r) || r.Header.Get("Content-Type") != "application/json" {
		fail(w, 403, errors.New("forbidden"))
		return
	}
	reply := func(err error) {
		if err != nil {
			s.link.Note("error: %v", err)
			resp := s.link.Status()
			resp["error"] = err.Error()
			writeJSON(w, 400, resp)
			return
		}
		writeJSON(w, 200, s.link.Status())
	}
	var args struct {
		Index   *int   `json:"index"`
		Slot    *int   `json:"slot"`
		RGB     string `json:"rgb"`
		On      bool   `json:"on"`
		Degrees *int   `json:"degrees"`
		Level   *int   `json:"level"`
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
		slot, err := s.link.Upload(args.Slot, rgb)
		if err != nil {
			reply(err)
			return
		}
		resp := s.link.Status()
		resp["slot"] = slot
		writeJSON(w, 200, resp)
	case "/api/delete":
		slot, err := need(args.Slot, "slot")
		if err == nil {
			err = s.link.Delete(slot)
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
