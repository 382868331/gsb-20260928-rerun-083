package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// Empty results must serialize as [] (never null) so the frontend can
// read .length without whitening the whole page.
func TestCompareNeverNullFields(t *testing.T) {
	b := &Board{
		ID: "b1", Name: "t", Revision: 3,
		Layers: []string{"top"},
		Nets:   []Net{{ID: "n1", Name: "N1"}},
		Pads: []Pad{
			{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle", X: 0, Y: 0, Diameter: 2 * mm},
			{ID: "p2", NetID: "n1", Layer: "top", Shape: "circle", X: 3 * mm, Y: 0, Diameter: 2 * mm},
		},
		Traces: []Trace{{ID: "t1", NetID: "n1", Layer: "top", Width: mm,
			Points: []Point{{X: 0, Y: 0}, {X: 3 * mm, Y: 0}}}},
	}
	rep := CompareVariants(b)
	data, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	js := string(data)
	for _, want := range []string{"\"diff\":[]", "\"errors\":[]", "\"warnings\":[]"} {
		if !strings.Contains(js, want) {
			t.Fatalf("compare JSON missing %s: %s", want, js)
		}
	}
	if strings.Contains(js, ":null") {
		t.Fatalf("compare JSON contains null: %s", js)
	}
}

// A variant with no conductive elements left is "empty" (no usable
// path), one referencing missing parts is "incomplete"; neither may
// disturb the other variants' results.
func TestCompareVariantStatus(t *testing.T) {
	b := &Board{
		ID: "b2", Name: "t", Revision: 1,
		Layers: []string{"top"},
		Nets:   []Net{{ID: "n1", Name: "N1"}},
		Pads: []Pad{
			{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle", X: 0, Y: 0, Diameter: 2 * mm},
			{ID: "p2", NetID: "n1", Layer: "top", Shape: "circle", X: 3 * mm, Y: 0, Diameter: 2 * mm},
		},
		Variants: []Variant{
			{ID: "var-empty", Name: "全缺装", DNP: []string{"p1", "p2"}},
			{ID: "var-broken", Name: "资料缺失", DNP: []string{"pad-gone"}, Jumpers: []string{"jumper-gone"}},
			{ID: "var-ok", Name: "正常"},
		},
	}
	rep := CompareVariants(b)
	byID := map[string]VariantReport{}
	for _, v := range rep.Variants {
		byID[v.VariantID] = v
	}
	if got := byID["var-empty"].Status; got != "empty" {
		t.Fatalf("var-empty status = %s, want empty", got)
	}
	if got := byID["var-broken"].Status; got != "incomplete" {
		t.Fatalf("var-broken status = %s, want incomplete", got)
	}
	if len(byID["var-broken"].Warnings) != 2 {
		t.Fatalf("var-broken warnings = %+v, want 2 entries", byID["var-broken"].Warnings)
	}
	if got := byID["var-ok"].Status; got != "ok" {
		t.Fatalf("var-ok status = %s, want ok", got)
	}
	if got := byID[""].Status; got != "ok" {
		t.Fatalf("base status = %s, want ok", got)
	}
	found := false
	for _, ns := range byID["var-ok"].NetStatus {
		if ns.NetID == "n1" {
			found = true
		}
	}
	if !found {
		t.Fatal("var-ok must still carry its net status despite broken siblings")
	}
}

// The net-path endpoint walks the route a net uses only in one variant
// (installed jumper included, segment by segment).
func TestAPINetPath(t *testing.T) {
	s := newTestServer(t)
	type netPathRes struct {
		State string     `json:"state"`
		Steps []PathStep `json:"steps"`
	}
	post := func(variantID, netID string) netPathRes {
		body, _ := json.Marshal(map[string]any{"board": s.board, "variantId": variantID, "netId": netID})
		w := httptest.NewRecorder()
		s.netPath(w, httptest.NewRequest("POST", "/api/board/netpath", bytes.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("netpath %s/%s failed: %s", variantID, netID, w.Body.String())
		}
		var res netPathRes
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	r := post("var-b", "net-gnd")
	if r.State != "connected" {
		t.Fatalf("var-b net-gnd state = %s, want connected", r.State)
	}
	hasJumper := false
	for _, st := range r.Steps {
		if st.Kind == "element" && st.ID == "jumper-gnd" {
			hasJumper = true
		}
	}
	if !hasJumper {
		t.Fatalf("var-b net-gnd path must name the installed jumper: %+v", r.Steps)
	}
	if r := post("", "net-gnd"); r.State != "open" {
		t.Fatalf("base net-gnd state = %s, want open (jumper not installed)", r.State)
	}
	if r := post("var-b", "net-no-such"); r.State != "empty" {
		t.Fatalf("unknown net state = %s, want empty", r.State)
	}
}

// A partially missing board document still compares: warnings name the
// absent sections and every list stays a list.
func TestAPIComparePartialBoard(t *testing.T) {
	s := newTestServer(t)
	partial := []byte("{\"id\":\"bp\",\"name\":\"缺资料\",\"revision\":7,\"pads\":[{\"id\":\"p1\",\"netId\":\"\",\"layer\":\"top\",\"shape\":\"circle\",\"x\":1,\"y\":1,\"diameter\":2}]}")
	w := httptest.NewRecorder()
	s.compareVariants(w, httptest.NewRequest("POST", "/api/board/variants/compare", bytes.NewReader(partial)))
	if w.Code != 200 {
		t.Fatalf("compare partial failed: %s", w.Body.String())
	}
	var rep CompareReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Warnings) == 0 {
		t.Fatal("missing sections must be reported as warnings")
	}
	if rep.Diff == nil || rep.Variants == nil {
		t.Fatal("diff/variants must be empty lists, never null")
	}
	if len(rep.Variants) != 1 || rep.Variants[0].Errors == nil || rep.Variants[0].NetStatus == nil {
		t.Fatalf("base variant result incomplete: %+v", rep.Variants)
	}
	if rep.Revision != 7 {
		t.Fatalf("revision = %d, want 7 (report binds the posted revision)", rep.Revision)
	}
	if s.board.ID == "bp" {
		t.Fatal("compare must not replace the saved board")
	}
}
