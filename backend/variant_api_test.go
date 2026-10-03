package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// Variant checks over HTTP: the same posted board revision yields
// per-variant verdicts that never borrow each other's conclusions.
func TestAPIVariantCheckIndependent(t *testing.T) {
	s := newTestServer(t)
	post := func(variantID string) (res struct {
		Errors    []CheckError `json:"errors"`
		NetStatus []NetStatus  `json:"netStatus"`
		VariantID string       `json:"variantId"`
		Revision  int          `json:"revision"`
	}) {
		body, _ := json.Marshal(map[string]any{"board": s.board, "variantId": variantID})
		w := httptest.NewRecorder()
		s.checkBoard(w, httptest.NewRequest("POST", "/api/board/check", bytes.NewReader(body)))
		if w.Code != 200 {
			t.Fatalf("check %q failed: %s", variantID, w.Body.String())
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}
	state := func(variantID, netID string) string {
		res := post(variantID)
		if res.VariantID != variantID {
			t.Fatalf("echo variantId = %q, want %q", res.VariantID, variantID)
		}
		for _, ns := range res.NetStatus {
			if ns.NetID == netID {
				return ns.State
			}
		}
		return ""
	}
	// Demo board: GND gap is bridged only by the jumper installed in
	// variants B and C; VCC pad is depopulated only in variant C.
	if got := state("", "net-gnd"); got != "open" {
		t.Fatalf("base net-gnd = %s, want open", got)
	}
	if got := state("var-b", "net-gnd"); got != "connected" {
		t.Fatalf("var-b net-gnd = %s, want connected (jumper installed)", got)
	}
	if got := state("", "net-gnd"); got != "open" {
		t.Fatalf("base net-gnd after var-b = %s, want open (no reuse)", got)
	}
	if got := state("var-c", "net-vcc"); got != "open" {
		t.Fatalf("var-c net-vcc = %s, want open (device depopulated)", got)
	}
	if got := state("var-b", "net-vcc"); got != "connected" {
		t.Fatalf("var-b net-vcc = %s, want connected", got)
	}
}

func TestAPICompareVariants(t *testing.T) {
	s := newTestServer(t)
	body, _ := json.Marshal(s.board)
	w := httptest.NewRecorder()
	s.compareVariants(w, httptest.NewRequest("POST", "/api/board/variants/compare", bytes.NewReader(body)))
	if w.Code != 200 {
		t.Fatalf("compare failed: %s", w.Body.String())
	}
	var rep CompareReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Variants) != 4 { // base + 3 demo variants
		t.Fatalf("variants = %d, want 4", len(rep.Variants))
	}
	diff := map[string]NetDiff{}
	for _, d := range rep.Diff {
		diff[d.NetID] = d
	}
	gnd, ok := diff["net-gnd"]
	if !ok || len(gnd.OnlyOpen) == 0 {
		t.Fatalf("net-gnd should differ (jumper bridges gap only in some variants): %+v", rep.Diff)
	}
	vcc, ok := diff["net-vcc"]
	if !ok || len(vcc.OnlyOpen) != 1 || vcc.OnlyOpen[0] != "var-c" {
		t.Fatalf("net-vcc should be open only in var-c: %+v", rep.Diff)
	}
	if _, ok := diff["net-sig"]; ok {
		t.Fatal("net-sig is open in every variant equally, must not appear in diff")
	}
}

// A partially missing board document must still save and check: the
// absent sections fall back to defaults instead of failing the request.
func TestAPIPartialBoardTolerated(t *testing.T) {
	s := newTestServer(t)
	partial := []byte(`{"id":"b-partial","name":"缺资料板","pads":[{"id":"p1","netId":"","layer":"top","shape":"circle","x":1000000,"y":1000000,"diameter":2000000}]}`)
	w := httptest.NewRecorder()
	s.putBoard(w, httptest.NewRequest("PUT", "/api/board", bytes.NewReader(partial)))
	if w.Code != 200 {
		t.Fatalf("put partial board failed: %s", w.Body.String())
	}
	var saved Board
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Layers) != 2 || saved.Traces == nil || saved.Jumpers == nil || saved.Variants == nil {
		t.Fatalf("missing sections should be defaulted, got %+v", saved)
	}
	if len(saved.Pads) != 1 {
		t.Fatalf("existing pad must be preserved, got %+v", saved.Pads)
	}
	// And it still checks without error.
	w2 := httptest.NewRecorder()
	s.checkBoard(w2, httptest.NewRequest("POST", "/api/board/check", nil))
	if w2.Code != 200 {
		t.Fatalf("check on partial board failed: %s", w2.Body.String())
	}
}
