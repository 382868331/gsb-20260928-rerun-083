package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync/atomic"
	"time"
)

// StoredCompare is one frozen compare report plus the exact basis it was
// computed from: the board revision, a geometry/assembly key, and a full
// snapshot of the board. The snapshot lets the UI re-trace fault paths
// on the old revision after the board has moved on — clearly labelled
// as old geometry, never silently standing in for the current board.
type StoredCompare struct {
	ID       string        `json:"id"`
	Revision int           `json:"revision"` // basis board revision
	Key      string        `json:"key"`      // compareKeyOf(basis)
	Report   CompareReport `json:"report"`
	Basis    *Board        `json:"basis"`
}

// compareListEntry annotates a stored report with whether its basis
// still matches the board currently held by the server.
type compareListEntry struct {
	StoredCompare
	Current bool `json:"current"`
}

var compareSeq uint64

// compareKeyOf fingerprints everything a compare result depends on:
// the conductive geometry (pads/traces/vias/jumpers), the layer stack,
// the nets, and every variant's own assembly selection. The revision
// number is deliberately excluded: undoing an edit back to identical
// geometry (then saving, which bumps the revision) must still be
// recognized as "the board has not changed".
func compareKeyOf(b *Board) string {
	type canonical struct {
		Layers   []string  `json:"layers"`
		Nets     []Net     `json:"nets"`
		Pads     []Pad     `json:"pads"`
		Traces   []Trace   `json:"traces"`
		Vias     []Via     `json:"vias"`
		Jumpers  []Jumper  `json:"jumpers"`
		Variants []Variant `json:"variants"`
	}
	vs := make([]Variant, 0, len(b.Variants))
	for _, v := range b.Variants {
		dnp := append([]string{}, v.DNP...)
		jmp := append([]string{}, v.Jumpers...)
		sort.Strings(dnp)
		sort.Strings(jmp)
		vs = append(vs, Variant{ID: v.ID, Name: v.Name, DNP: dnp, Jumpers: jmp})
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].ID < vs[j].ID })
	data, _ := json.Marshal(canonical{
		Layers: b.Layers, Nets: b.Nets, Pads: b.Pads, Traces: b.Traces,
		Vias: b.Vias, Jumpers: b.Jumpers, Variants: vs,
	})
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// cloneBoardStripped deep-copies the board for use as a report basis.
// The copy carries no compare history of its own, so stored reports
// never nest inside each other.
func cloneBoardStripped(b *Board) *Board {
	data, err := json.Marshal(b)
	if err != nil {
		return nil
	}
	var out Board
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	out.Compares = nil
	sanitizeSections(&out)
	return &out
}

// newStoredCompare freezes a fresh compare report together with the
// board it was computed from.
func newStoredCompare(b *Board, rep CompareReport) StoredCompare {
	id := fmt.Sprintf("cmp-%d-%d", time.Now().UnixNano(), atomic.AddUint64(&compareSeq, 1))
	key := compareKeyOf(b)
	rep.ID = id
	rep.Key = key
	return StoredCompare{
		ID: id, Revision: b.Revision, Key: key,
		Report: rep, Basis: cloneBoardStripped(b),
	}
}
