package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StoredRepair freezes one repair solve round with its exact basis, so
// an old result keeps representing the revision it was computed from
// while new checks are saved as separate entries.
type StoredRepair struct {
	ID        string       `json:"id"`
	Key       string       `json:"key"`
	Revision  int          `json:"revision"`
	VariantID string       `json:"variantId"`
	Spec      RepairSpec   `json:"spec"`
	Result    RepairResult `json:"result"`
	Basis     Board        `json:"basis"`
}

// repairStore persists repair check reports next to the board file,
// separate from the frozen compare reports.
type repairStore struct {
	mu   sync.Mutex
	path string
}

func newRepairStore(boardPath string) *repairStore {
	return &repairStore{path: filepath.Join(filepath.Dir(boardPath), "repairs.json")}
}

func (s *repairStore) loadAll() ([]StoredRepair, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []StoredRepair{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return []StoredRepair{}, []string{"修复报告存档损坏, 无法解析: " + err.Error()}, nil
	}
	out := []StoredRepair{}
	warnings := []string{}
	for i, raw := range raws {
		var sr StoredRepair
		if err := json.Unmarshal(raw, &sr); err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 份修复报告损坏, 已跳过", i+1))
			continue
		}
		sanitizeBoard(&sr.Basis)
		out = append(out, sr)
	}
	return out, warnings, nil
}

func (s *repairStore) append(sr StoredRepair) (StoredRepair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := []StoredRepair{}
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, &list)
	} else if !os.IsNotExist(err) {
		return sr, err
	}
	sanitizeBoard(&sr.Basis)
	sr.ID = fmt.Sprintf("rep-%d", time.Now().UnixNano())
	sr.Revision = sr.Result.Revision
	list = append(list, sr)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return sr, err
	}
	out, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return sr, err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return sr, err
	}
	return sr, os.Rename(tmp, s.path)
}
