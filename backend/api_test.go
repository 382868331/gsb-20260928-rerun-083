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

// Save -> reload -> check: a board where an un-netted trace/via chain
// bridges two nets must report a located short after a full
// persistence round trip, with geometry intact (integer nm).
func TestSaveReloadShortThroughUnnettedCopper(t *testing.T) {
	s := newTestServer(t)

	b := baseBoard()
	b.Pads = []Pad{
		{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
			X: 10 * mm, Y: 25 * mm, Diameter: 2 * mm},
		{ID: "p2", NetID: "n2", Layer: "bottom", Shape: "circle",
			X: 45 * mm, Y: 25 * mm, Diameter: 2 * mm},
	}
	b.Traces = []Trace{
		{ID: "t1", NetID: "", Layer: "top", Width: 500_000,
			Points: []Point{pt(10, 25), pt(25, 25), pt(25, 20)}},
		{ID: "t2", NetID: "", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(25, 20), pt(45, 20), pt(45, 25)}},
	}
	b.Vias = []Via{{ID: "v1", NetID: "", X: 25 * mm, Y: 20 * mm,
		Diameter: 1_200_000, Layers: []string{"top", "bottom"}}}

	data, _ := json.Marshal(b)
	r := httptest.NewRequest("PUT", "/api/board", bytes.NewReader(data))
	w := httptest.NewRecorder()
	s.putBoard(w, r)
	if w.Code != 200 {
		t.Fatalf("save failed: %s", w.Body.String())
	}

	// Reload from disk: geometry must survive the round trip exactly.
	loaded, err := s.st.load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Traces) != 2 || loaded.Traces[0].Points[1] != (Point{25 * mm, 25 * mm}) {
		t.Fatalf("geometry changed after save/reload: %+v", loaded.Traces)
	}

	errs := CheckBoard(loaded)
	var shorts []CheckError
	for _, e := range errs {
		if e.Type == "short" {
			shorts = append(shorts, e)
		}
	}
	if len(shorts) != 1 {
		t.Fatalf("expected 1 short after reload, got %v", errs)
	}
	s0 := shorts[0]
	if len(s0.Nets) != 2 || len(s0.Elements) < 4 {
		t.Fatalf("short should name both nets and the full path, got %+v", s0)
	}
	if s0.X < 9*mm || s0.X > 46*mm || s0.Y < 19*mm || s0.Y > 26*mm {
		t.Fatalf("short location off the bridge: (%.3f,%.3f)mm", s0.X/1e6, s0.Y/1e6)
	}

	// Fractional coordinates must be rejected, not silently truncated.
	r = httptest.NewRequest("PUT", "/api/board",
		bytes.NewReader([]byte(`{"id":"b","pads":[{"id":"p","x":1.5,"y":0}]}`)))
	w = httptest.NewRecorder()
	s.putBoard(w, r)
	if w.Code != 400 {
		t.Fatalf("fractional nm should be rejected with 400, got %d", w.Code)
	}
}
