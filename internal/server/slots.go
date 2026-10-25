package server

import (
	"cmp"
	"fmt"
	"slices"
)

type slotInfo struct {
	Slot  int    `json:"slot"`
	Type  string `json:"type"`
	Key   string `json:"key"`
	Src   string `json:"src"`
	Match *bool  `json:"match"`
}

func (s *Server) allSlots() ([]slotInfo, error) {
	uploads, err := s.store.Entries()
	if err != nil {
		return nil, err
	}
	slots := make([]slotInfo, 0, len(uploads))
	for slot, e := range uploads {
		slots = append(slots, slotInfo{Slot: int(slot), Type: "upload", Key: e.Key, Src: fmt.Sprintf("/img/%d?v=%d", slot, e.TS)})
	}
	slices.SortFunc(slots, func(a, b slotInfo) int { return cmp.Compare(a.Slot, b.Slot) })
	return slots, nil
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
	slices.SortFunc(slots, func(a, b slotInfo) int { return cmp.Compare(a.Slot, b.Slot) })
	return slots
}
