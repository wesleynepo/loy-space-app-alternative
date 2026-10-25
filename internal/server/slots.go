package server

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type programEntry struct {
	Type string `json:"type"`
	Key  string `json:"key"`
	Path string `json:"path"`
}

func (s *Server) loadProgram() []programEntry {
	var p []programEntry
	if b, err := os.ReadFile(filepath.Join(s.cfg.AppDoc, "524A250818_program_2_64_64.json")); err == nil {
		_ = json.Unmarshal(b, &p)
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

func (s *Server) allSlots() []slotInfo {
	var slots []slotInfo
	for i, e := range s.loadProgram() {
		slots = append(slots, slotInfo{Slot: i, Type: e.Type, Key: e.Key})
	}
	uploads := s.link.Uploads().Load()
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

func withPanelKeys(slots []slotInfo, keys map[int]string) []slotInfo {
	known := map[int]bool{}
	for i := range slots {
		m := keys[slots[i].Slot] == slots[i].Key
		slots[i].Match = &m
		known[slots[i].Slot] = true
	}
	for n, key := range keys {
		if !known[n] {
			m := true
			slots = append(slots, slotInfo{Slot: n, Type: "unknown", Key: key, Match: &m})
		}
	}
	sort.Slice(slots, func(i, j int) bool { return slots[i].Slot < slots[j].Slot })
	return slots
}

func (s *Server) slotImage(slot int) ([]byte, string, error) {
	program := s.loadProgram()
	if slot >= len(program) {
		b, err := os.ReadFile(s.link.Uploads().PreviewPath(slot))
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
		file = filepath.Join(s.cfg.CacheDir, hex.EncodeToString(sum[:])+filepath.Ext(u.Path))
		if _, err := os.Stat(file); err != nil {
			_ = os.MkdirAll(s.cfg.CacheDir, 0o755)
			client := http.Client{Timeout: 10 * time.Second}
			resp, err := client.Get(path)
			if err != nil {
				return nil, "", err
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, "", fmt.Errorf("download failed: %w", err)
			}
			if resp.StatusCode != http.StatusOK {
				return nil, "", fmt.Errorf("download failed: status %d", resp.StatusCode)
			}
			_ = os.WriteFile(file, b, 0o644)
		}
	} else {
		file = filepath.Join(s.cfg.AppDoc, strings.TrimPrefix(path, "_doc/"))
		if rel, err := filepath.Rel(s.cfg.AppDoc, filepath.Clean(file)); err != nil || strings.HasPrefix(rel, "..") {
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
