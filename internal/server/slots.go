package server

import (
	"fmt"
	"os"
	"sort"
	"strconv"
)

type slotInfo struct {
	Slot  int    `json:"slot"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Src   string `json:"src"`
	Match *bool  `json:"match"`
}

func (s *Server) allSlots() []slotInfo {
	var slots []slotInfo
	uploads := s.link.Uploads().Load()
	var nums []int
	for k := range uploads {
		if n, err := strconv.Atoi(k); err == nil {
			nums = append(nums, n)
		}
	}
	sort.Ints(nums)
	for _, n := range nums {
		slots = append(slots, slotInfo{Slot: n, Type: "upload", Key: uploads[strconv.Itoa(n)].Key})
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
	b, err := os.ReadFile(s.link.Uploads().PreviewPath(slot))
	return b, "image/png", err
}
