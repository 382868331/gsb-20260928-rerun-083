package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StoredCompare is a frozen compare report plus the exact basis it was
// computed from: the compare key (revision + variant selections +
// geometry fingerprint, computed client-side) and a full snapshot of
// the board. Keeping the basis snapshot means an old report can still
// be inspected — and its fault paths re-traced — after the live board
// has moved on, without the old revision ever impersonating the
// current geometry.
type StoredCompare struct {
	ID       string        `json:"id"`
	Key      string        `json:"key"`
	Revision int           `json:"revision"`
	Report   CompareReport `json:"report"`
	Basis    Board         `json:"basis"`
}

// compareStore persists frozen compare reports next to the board file,
// so they survive save + refresh and server restarts.
type compareStore struct {
	mu   sync.Mutex
	path string
}

func newCompareStore(boardPath string) *compareStore {
	return &compareStore{path: filepath.Join(filepath.Dir(boardPath), "compares.json")}
}

// loadAll reads every stored report. A single corrupt entry is skipped
// with a warning instead of taking the other reports down with it.
func (s *compareStore) loadAll() ([]StoredCompare, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []StoredCompare{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return []StoredCompare{}, []string{"比较报告存档损坏, 无法解析: " + err.Error()}, nil
	}
	out := []StoredCompare{}
	warnings := []string{}
	for i, raw := range raws {
		var sc StoredCompare
		if err := json.Unmarshal(raw, &sc); err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 份比较报告损坏, 已跳过", i+1))
			continue
		}
		sanitizeBoard(&sc.Basis)
		if sc.Report.Variants == nil {
			sc.Report.Variants = []VariantReport{}
		}
		if sc.Report.Diff == nil {
			sc.Report.Diff = []NetDiff{}
		}
		if sc.Report.Warnings == nil {
			sc.Report.Warnings = []string{}
		}
		out = append(out, sc)
	}
	return out, warnings, nil
}

func (s *compareStore) saveAll(list []StoredCompare) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// append freezes a new report. Old reports are always kept: re-checking
// the current board adds a new entry rather than replacing history.
func (s *compareStore) append(sc StoredCompare) (StoredCompare, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := []StoredCompare{}
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, &list)
	} else if !os.IsNotExist(err) {
		return sc, err
	}
	sanitizeBoard(&sc.Basis)
	sc.ID = fmt.Sprintf("cmp-%d", time.Now().UnixNano())
	sc.Revision = sc.Report.Revision
	list = append(list, sc)
	return sc, s.saveAll(list)
}
