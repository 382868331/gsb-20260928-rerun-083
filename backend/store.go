package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type store struct {
	mu   sync.Mutex
	path string
}

func newStore(path string) *store { return &store{path: path} }

// sanitizeBoard fills safe defaults for partially missing board data so
// one absent section never makes the whole board unusable: missing
// lists become empty lists and a board without a layer stack gets the
// default two-layer stack. Existing content is kept verbatim.
func sanitizeBoard(b *Board) {
	sanitizeSections(b)
	sanitizeCompares(b)
}

// sanitizeSections repairs the board's own sections (no compare
// history); also used for each stored report's basis snapshot.
func sanitizeSections(b *Board) {
	if b.Outline == nil {
		b.Outline = []Point{}
	}
	if len(b.Layers) == 0 {
		b.Layers = []string{"top", "bottom"}
	}
	if b.Nets == nil {
		b.Nets = []Net{}
	}
	if b.Pads == nil {
		b.Pads = []Pad{}
	}
	if b.Traces == nil {
		b.Traces = []Trace{}
	}
	if b.Vias == nil {
		b.Vias = []Via{}
	}
	if b.Jumpers == nil {
		b.Jumpers = []Jumper{}
	}
	if b.Variants == nil {
		b.Variants = []Variant{}
	}
	for i := range b.Variants {
		if b.Variants[i].DNP == nil {
			b.Variants[i].DNP = []string{}
		}
		if b.Variants[i].Jumpers == nil {
			b.Variants[i].Jumpers = []string{}
		}
	}
}

// sanitizeCompares repairs each stored compare report independently, so
// one entry with missing sections (absent report lists, a damaged basis
// snapshot) never keeps the remaining reports from being recovered.
func sanitizeCompares(b *Board) {
	for i := range b.Compares {
		c := &b.Compares[i]
		if c.Report.Variants == nil {
			c.Report.Variants = []VariantReport{}
		}
		if c.Report.Diff == nil {
			c.Report.Diff = []NetDiff{}
		}
		if c.Report.Warnings == nil {
			c.Report.Warnings = []string{}
		}
		for j := range c.Report.Variants {
			v := &c.Report.Variants[j]
			if v.Warnings == nil {
				v.Warnings = []string{}
			}
			if v.Errors == nil {
				v.Errors = []CheckError{}
			}
			if v.NetStatus == nil {
				v.NetStatus = []NetStatus{}
			}
		}
		if c.Basis != nil {
			sanitizeSections(c.Basis)
			c.Basis.Compares = nil
		}
	}
}

func (s *store) load() (*Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		return nil, err
	}
	var b Board
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	sanitizeBoard(&b)
	return &b, nil
}

func (s *store) save(b *Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}
