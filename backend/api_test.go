package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func newTestServer(t *testing.T) *server {
	t.Helper()
	st := newStore(filepath.Join(t.TempDir(), "board.json"))
	b := demoBoard()
	if err := st.save(b); err != nil {
		t.Fatal(err)
	}
	return &server{st: st, board: b}
}

// Save -> check -> export -> import round trip over the HTTP handlers,
// verifying persistence and element identity preservation.
func TestAPIRoundTrip(t *testing.T) {
	s := newTestServer(t)

	// Check (unsaved body): demo board has 2 opens, 0 shorts.
	body, _ := json.Marshal(s.board)
	r := httptest.NewRequest("POST", "/api/board/check", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.checkBoard(w, r)
	var res struct{ Errors []CheckError }
	json.Unmarshal(w.Body.Bytes(), &res)
	if len(res.Errors) != 2 {
		t.Fatalf("expected 2 errors on demo board, got %d", len(res.Errors))
	}

	// Save a modified board (add a pad), revision bumps.
	var b Board
	json.Unmarshal(body, &b)
	b.Pads = append(b.Pads, Pad{ID: "pad-new", NetID: "net-vcc", Layer: "top",
		Shape: "circle", X: 20 * mm, Y: 35 * mm, Diameter: mm})
	data, _ := json.Marshal(&b)
	r = httptest.NewRequest("PUT", "/api/board", bytes.NewReader(data))
	w = httptest.NewRecorder()
	s.putBoard(w, r)
	if w.Code != 200 {
		t.Fatalf("put failed: %s", w.Body.String())
	}
	var saved Board
	json.Unmarshal(w.Body.Bytes(), &saved)
	if saved.Revision != 1 {
		t.Fatalf("revision should bump to 1, got %d", saved.Revision)
	}

	// Reload from disk: persisted.
	loaded, err := s.st.load()
	if err != nil || len(loaded.Pads) != len(b.Pads) {
		t.Fatalf("board not persisted: %v", err)
	}

	// Export -> import keeps every element id.
	r = httptest.NewRequest("GET", "/api/board/export", nil)
	w = httptest.NewRecorder()
	s.exportBoard(w, r)
	exported := w.Body.Bytes()

	r = httptest.NewRequest("POST", "/api/board/import", bytes.NewReader(exported))
	w = httptest.NewRecorder()
	s.importBoard(w, r)
	if w.Code != 200 {
		t.Fatalf("import failed: %s", w.Body.String())
	}
	var re Board
	json.Unmarshal(w.Body.Bytes(), &re)
	want := map[string]bool{}
	for _, p := range b.Pads {
		want[p.ID] = true
	}
	for _, tr := range b.Traces {
		want[tr.ID] = true
	}
	for _, v := range b.Vias {
		want[v.ID] = true
	}
	got := map[string]bool{}
	for _, p := range re.Pads {
		got[p.ID] = true
	}
	for _, tr := range re.Traces {
		got[tr.ID] = true
	}
	for _, v := range re.Vias {
		got[v.ID] = true
	}
	for id := range want {
		if !got[id] {
			t.Fatalf("element %q lost after export->import", id)
		}
	}
}

// A mouse-drawn trace lands on arbitrary integer-nm coordinates; the
// save -> reload round trip must preserve its geometry and the check
// verdicts exactly.
func TestSaveReloadPreservesDrawnGeometry(t *testing.T) {
	s := newTestServer(t)
	var b Board
	body, _ := json.Marshal(s.board)
	json.Unmarshal(body, &b)
	b.Traces = append(b.Traces, Trace{ID: "trace-drawn", NetID: "net-gnd",
		Layer: "top", Width: 500_000,
		Points: []Point{{X: 12_345_678, Y: 9_876_543}, {X: 33_333_333, Y: 11_111_111}, {X: 44_444_444, Y: 22_222_222}}})
	data, _ := json.Marshal(&b)
	r := httptest.NewRequest("PUT", "/api/board", bytes.NewReader(data))
	w := httptest.NewRecorder()
	s.putBoard(w, r)
	if w.Code != 200 {
		t.Fatalf("save of drawn trace failed: %s", w.Body.String())
	}
	loaded, err := s.st.load()
	if err != nil {
		t.Fatal(err)
	}
	var got *Trace
	for i := range loaded.Traces {
		if loaded.Traces[i].ID == "trace-drawn" {
			got = &loaded.Traces[i]
		}
	}
	if got == nil {
		t.Fatal("drawn trace lost after reload")
	}
	for i, p := range got.Points {
		if p != b.Traces[len(b.Traces)-1].Points[i] {
			t.Fatalf("point %d changed across save/reload: %v != %v", i, p, b.Traces[len(b.Traces)-1].Points[i])
		}
	}
	e1, e2 := CheckBoard(&b), CheckBoard(loaded)
	if len(e1) != len(e2) {
		t.Fatalf("check verdicts differ after reload: %d vs %d", len(e1), len(e2))
	}
}
