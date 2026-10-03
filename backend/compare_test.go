package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// The compare report must never emit JSON null where the UI expects a
// list: empty results are empty arrays, otherwise the web page crashes
// while rendering and the whole workbench (save/edit included) blanks.
func TestCompareJSONNeverNullLists(t *testing.T) {
	b := &Board{
		ID: "b-clean", Revision: 3, Layers: []string{"top"},
		Nets: []Net{{ID: "n1", Name: "N1"}},
		Pads: []Pad{
			{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle", X: mm, Y: mm, Diameter: 2 * mm},
		},
	}
	sanitizeBoard(b)
	out, err := json.Marshal(CompareVariants(b))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, bad := range []string{`"diff":null`, `"variants":null`, `"errors":null`, `"netStatus":null`, `"onlyOpen":null`, `"onlyShort":null`, `"notes":null`, `"paths":null`} {
		if strings.Contains(s, bad) {
			t.Fatalf("compare JSON contains %s: %s", bad, s)
		}
	}
	if !strings.Contains(s, `"diff":[]`) || !strings.Contains(s, `"errors":[]`) {
		t.Fatalf("empty results must be empty arrays: %s", s)
	}
}

// A variant that references elements the board no longer has is
// reported as missing data, yet every other variant and the base view
// are still analyzed.
func TestCompareVariantMissingDataIsolated(t *testing.T) {
	b := variantBoard()
	b.Variants = append(b.Variants, Variant{
		ID: "v-broken", Name: "缺资料",
		DNP: []string{"pad-gone"}, Jumpers: []string{"jumper-gone"},
	})
	rep := CompareVariants(b)
	if len(rep.Variants) != 4 {
		t.Fatalf("variants = %d, want 4", len(rep.Variants))
	}
	byID := map[string]VariantReport{}
	for _, vr := range rep.Variants {
		byID[vr.VariantID] = vr
	}
	broken := byID["v-broken"]
	if broken.Status != "missing" || len(broken.Notes) == 0 {
		t.Fatalf("broken variant should be flagged missing with notes: %+v", broken)
	}
	if byID["v1"].Status != "ok" || byID[""].Status != "ok" {
		t.Fatalf("other variants must stay analyzable: %+v", rep.Variants)
	}
	// The broken variant still got a real analysis of what exists.
	if len(byID["v1"].NetStatus) == 0 || len(broken.NetStatus) == 0 {
		t.Fatalf("net status missing: %+v", rep.Variants)
	}
}

// A variant whose assembly leaves no conductive elements at all is
// reported as having no usable paths, distinct from "no faults".
func TestCompareVariantEmpty(t *testing.T) {
	b := variantBoard()
	b.Variants = append(b.Variants, Variant{
		ID: "v-none", Name: "全缺装",
		DNP: []string{"pa1", "pa2", "pb1"},
	})
	// Remove shared copper so the DNP-everything variant has nothing.
	b.Traces = nil
	rep := CompareVariants(b)
	byID := map[string]VariantReport{}
	for _, vr := range rep.Variants {
		byID[vr.VariantID] = vr
	}
	if got := byID["v-none"].Status; got != "empty" {
		t.Fatalf("v-none status = %s, want empty", got)
	}
	if got := byID[""].Status; got != "ok" {
		t.Fatalf("base status = %s, want ok", got)
	}
}

// Each variant report freezes the revision and the exact assembly
// selection it was computed from.
func TestCompareBindsRevisionAndSelection(t *testing.T) {
	b := variantBoard()
	rep := CompareVariants(b)
	if rep.Revision != 7 {
		t.Fatalf("revision = %d, want 7", rep.Revision)
	}
	byID := map[string]VariantReport{}
	for _, vr := range rep.Variants {
		byID[vr.VariantID] = vr
		if vr.Revision != 7 {
			t.Fatalf("variant %q revision = %d, want 7", vr.VariantID, vr.Revision)
		}
	}
	if len(byID["v1"].Selection.Jumpers) != 1 || byID["v1"].Selection.Jumpers[0] != "j1" {
		t.Fatalf("v1 selection not frozen: %+v", byID[""].Selection)
	}
	if len(byID["v2"].Selection.DNP) != 1 || byID["v2"].Selection.DNP[0] != "pb1" {
		t.Fatalf("v2 selection not frozen: %+v", byID["v2"].Selection)
	}
	if len(byID[""].Selection.DNP) != 0 || len(byID[""].Selection.Jumpers) != 0 {
		t.Fatalf("base selection must be empty, got %+v", byID[""].Selection)
	}
}

// A net that conducts only under one variant gets a segment-by-segment
// route naming the copper and the installed jumper that carry it.
func TestCompareConnectedOnlyPathNamesJumper(t *testing.T) {
	b := variantBoard()
	rep := CompareVariants(b)
	var da *NetDiff
	for i := range rep.Diff {
		if rep.Diff[i].NetID == "net-a" {
			da = &rep.Diff[i]
		}
	}
	if da == nil {
		t.Fatal("net-a missing from diff")
	}
	steps := da.Paths["v1"]
	if len(steps) == 0 {
		t.Fatalf("net-a should have a conductive path for v1: %+v", da.Paths)
	}
	foundJumper := false
	for _, st := range steps {
		if st.Kind == "element" && st.ID == "j1" {
			foundJumper = true
		}
	}
	if !foundJumper {
		t.Fatalf("path should walk through installed jumper j1: %+v", steps)
	}
	if _, ok := da.Paths["v2"]; ok {
		t.Fatal("net-a is open in v2, no conductive path expected there")
	}
}

// Comparing a partially missing board document still succeeds and keeps
// every declared variant's slot.
func TestComparePartialBoardTolerated(t *testing.T) {
	s := newTestServer(t)
	partial := []byte(`{"id":"b-partial","revision":2,"pads":[{"id":"p1","netId":"","layer":"top","shape":"circle","x":1000000,"y":1000000,"diameter":2000000}],"variants":[{"id":"v1","name":"方案1"}]}`)
	w := httptest.NewRecorder()
	s.compareVariants(w, httptest.NewRequest("POST", "/api/board/variants/compare", bytes.NewReader(partial)))
	if w.Code != 200 {
		t.Fatalf("compare partial board failed: %s", w.Body.String())
	}
	var rep CompareReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Variants) != 2 {
		t.Fatalf("variants = %d, want 2 (base + v1): %+v", len(rep.Variants), rep.Variants)
	}
	if rep.Diff == nil || rep.Variants[0].Errors == nil || rep.Variants[0].NetStatus == nil {
		t.Fatal("lists must be non-nil even for a partial board")
	}
}
