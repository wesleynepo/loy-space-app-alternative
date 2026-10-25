package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

//go:embed web/index.html
var embeddedHTML []byte

func indexHTML() []byte {
	if b, err := os.ReadFile(filepath.Join("web", "index.html")); err == nil {
		return b
	}
	return embeddedHTML
}

var (
	uploadsDir = ".uploads" // our own uploads: previews + slot keys
	cacheDir   = ".cache"
	// LOY SPACE (iOS app on Apple silicon) keeps its per-panel slot list and local images here.
	appDoc = expandHome(getenv("LOY_DOC", "~/Library/Containers/com.yskd.loywf/Data/Library/Pandora/apps/__UNI__F2139F6/doc"))
)

var link = newLink()

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func expandHome(p string) string {
	if rest, ok := strings.CutPrefix(p, "~/"); ok {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, rest)
	}
	return p
}

func savePreview(slot int, rgb []byte) error {
	if err := os.MkdirAll(uploadsDir, 0o755); err != nil {
		return err
	}
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for i := 0; i < 64*64; i++ {
		img.Set(i%64, i/64, color.RGBA{rgb[3*i], rgb[3*i+1], rgb[3*i+2], 255})
	}
	f, err := os.Create(filepath.Join(uploadsDir, fmt.Sprintf("slot%d.png", slot)))
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

type programEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
	Path string `json:"path"`
}

func loadProgram() []programEntry {
	var p []programEntry
	if b, err := os.ReadFile(filepath.Join(appDoc, "524A250818_program_2_64_64.json")); err == nil {
		json.Unmarshal(b, &p)
	}
	return p
}

type slotInfo struct {
	Slot  int    `json:"slot"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Src   string `json:"src"`
	Match *bool  `json:"match"`
}

// allSlots is the app's program list, plus our own uploads in slots the app doesn't know about.
func allSlots() []slotInfo {
	var slots []slotInfo
	for i, e := range loadProgram() {
		slots = append(slots, slotInfo{Slot: i, Type: e.Type, Key: e.Key})
	}
	uploads := loadUploads()
	var nums []int
	for k := range uploads {
		if n, err := strconv.Atoi(k); err == nil {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	for _, n := range nums {
		if n >= len(slots) {
			slots = append(slots, slotInfo{Slot: n, Type: "upload", Key: uploads[strconv.Itoa(n)].Key})
		}
	}
	for i := range slots {
		k := slots[i].Key
		slots[i].Src = fmt.Sprintf("/img/%d?k=%s", slots[i].Slot, k[max(0, len(k)-8):])
	}
	return slots
}

// slotImage returns bytes and content type for a slot's picture: CDN download (cached) or a file in the app's folder.
func slotImage(slot int) ([]byte, string, error) {
	program := loadProgram()
	if slot >= len(program) {
		b, err := os.ReadFile(filepath.Join(uploadsDir, fmt.Sprintf("slot%d.png", slot)))
		return b, "image/png", err
	}
	path := program[slot].Path
	var file string
	if strings.HasPrefix(path, "http") {
		u, err := url.Parse(path)
		if err != nil || u.Scheme != "https" || !strings.HasSuffix(u.Hostname(), ".popled.cn") {
			return nil, "", errors.New("unexpected image host")
		}
		sum := sha1.Sum([]byte(path))
		file = filepath.Join(cacheDir, hex.EncodeToString(sum[:])+filepath.Ext(u.Path))
		if _, err := os.Stat(file); err != nil {
			os.MkdirAll(cacheDir, 0o755)
			client := http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(path)
			if err != nil {
				return nil, "", err
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != 200 {
				return nil, "", fmt.Errorf("download failed: %d %v", resp.StatusCode, err)
			}
			os.WriteFile(file, b, 0o644)
		}
	} else {
		file = filepath.Join(appDoc, strings.TrimPrefix(path, "_doc/"))
		if rel, err := filepath.Rel(appDoc, filepath.Clean(file)); err != nil || strings.HasPrefix(rel, "..") {
			return nil, "", errors.New("path outside the app folder")
		}
	}
	b, err := os.ReadFile(file)
	ctype := mime.TypeByExtension(filepath.Ext(file))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	return b, ctype, err
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

func handleGet(w http.ResponseWriter, r *http.Request) {
	if !localOnly(r) {
		fail(w, 403, errors.New("forbidden"))
		return
	}
	switch path := r.URL.Path; {
	case path == "/api/status":
		writeJSON(w, 200, link.status())
	case path == "/api/state":
		st, err := link.ReadState()
		if err != nil {
			fail(w, 400, err)
			return
		}
		writeJSON(w, 200, st)
	case path == "/api/slots":
		slots := allSlots()
		resp := map[string]any{"slots": slots, "current": nil, "error": nil, "used": nil, "total": totalSlots}
		if link.Connected() {
			if cur, keys, err := link.PanelState(); err != nil {
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
		body, ctype, err := slotImage(n)
		if err != nil {
			fail(w, 404, err)
			return
		}
		w.Header().Set("Content-Type", ctype)
		w.Header().Set("Cache-Control", "max-age=3600")
		w.Write(body)
	case path == "/":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML())
	default:
		fail(w, 404, errors.New("not found"))
	}
}

func inSlotRange(slot int) bool { return firstFreeSlot <= slot && slot <= lastSlot }

func handlePost(w http.ResponseWriter, r *http.Request) {
	if !localOnly(r) || r.Header.Get("Content-Type") != "application/json" {
		fail(w, 403, errors.New("forbidden"))
		return
	}
	reply := func(err error) {
		if err != nil {
			link.note("error: %v", err)
			resp := link.status()
			resp["error"] = err.Error()
			writeJSON(w, 400, resp)
			return
		}
		writeJSON(w, 200, link.status())
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
		reply(link.Connect())
	case "/api/disconnect":
		reply(link.Disconnect())
	case "/api/image":
		i, err := need(args.Index, "index")
		if err == nil && (i < 0 || i > 59) {
			err = errors.New("index out of range")
		}
		if err == nil {
			err = link.Command(func(uint16) []byte { return showImage(11, i) }, 11) // the app always used seq 11
		}
		reply(err)
	case "/api/upload":
		slot, err := uploadSlot(args.Slot, args.RGB)
		if err == nil {
			rgb, _ := base64.StdEncoding.DecodeString(args.RGB)
			_, err = link.Upload(slot, rgb)
		}
		if err != nil {
			reply(err)
			return
		}
		resp := link.status()
		resp["slot"] = slot
		writeJSON(w, 200, resp)
	case "/api/delete":
		slot, err := need(args.Slot, "slot")
		if err == nil && !inSlotRange(slot) {
			err = fmt.Errorf("deletes are limited to slots %d-%d", firstFreeSlot, lastSlot)
		}
		if err == nil {
			err = link.Delete(slot)
		}
		reply(err)
	case "/api/power":
		reply(link.Command(func(s uint16) []byte { return setPower(s, args.On) }, link.nextSeq()))
	case "/api/rotation":
		d, err := need(args.Degrees, "degrees")
		if err == nil && (d < 0 || d > 270 || d%90 != 0) {
			err = errors.New("degrees must be 0, 90, 180 or 270")
		}
		if err == nil {
			err = link.Command(func(s uint16) []byte { return setRotation(s, d/90) }, link.nextSeq())
		}
		reply(err)
	case "/api/brightness":
		lv, err := need(args.Level, "level")
		if err == nil && (lv < 1 || lv > 15) {
			err = errors.New("level must be 1-15 (0 is avoided: the app maps it to 15)")
		}
		if err == nil {
			err = link.Command(func(s uint16) []byte { return setBrightness(s, lv) }, link.nextSeq())
		}
		reply(err)
	default:
		fail(w, 404, errors.New("not found"))
	}
}

// uploadSlot validates an upload request and picks the target slot: the requested one, or the first the panel reports empty.
func uploadSlot(requested *int, rgbB64 string) (int, error) {
	rgb, err := base64.StdEncoding.DecodeString(rgbB64)
	if err != nil {
		return 0, err
	}
	if len(rgb) != 64*64*3 {
		return 0, errors.New("expected 64x64 RGB (12288 bytes)")
	}
	if requested != nil {
		if !inSlotRange(*requested) {
			return 0, fmt.Errorf("uploads go to slots %d-%d only", firstFreeSlot, lastSlot)
		}
		return *requested, nil
	}
	_, taken, err := link.PanelState()
	if err != nil {
		return 0, err
	}
	for s := firstFreeSlot; s <= lastSlot; s++ {
		if _, used := taken[s]; !used {
			return s, nil
		}
	}
	return 0, fmt.Errorf("slots %d-%d are all in use - delete one first", firstFreeSlot, lastSlot)
}

func main() {
	port := "8765"
	if len(os.Args) > 1 {
		port = os.Args[1]
	}
	if d := os.Getenv("LED_PANEL_DIR"); d != "" {
		os.Chdir(d)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", handleGet)
	mux.HandleFunc("POST /", handlePost)
	srv := &http.Server{Addr: "127.0.0.1:" + port, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		if link.Connected() {
			link.Disconnect()
		}
		srv.Close()
	}()
	fmt.Println("http://127.0.0.1:" + port)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
