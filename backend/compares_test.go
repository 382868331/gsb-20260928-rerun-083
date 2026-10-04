package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func postCompare(t *testing.T, s *server, b *Board) CompareReport {
	t.Helper()
	body, _ := json.Marshal(b)
	w := httptest.NewRecorder()
	s.compareVariants(w, httptest.NewRequest("POST", "/api/board/variants/compare", bytes.NewReader(body)))
	if w.Code != 200 {
		t.Fatalf("compare failed: %s", w.Body.String())
	}
	var rep CompareReport
	if err := json.Unmarshal(w.Body.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

func getCompares(t *testing.T, s *server) struct {
	Revision int                `json:"revision"`
	Key      string             `json:"key"`
	Reports  []compareListEntry `json:"reports"`
} {
	t.Helper()
	w := httptest.NewRecorder()
	s.listCompares(w, httptest.NewRequest("GET", "/api/board/compares", nil))
	if w.Code != 200 {
		t.Fatalf("list compares failed: %s", w.Body.String())
	}
	var res struct {
		Revision int                `json:"revision"`
		Key      string             `json:"key"`
		Reports  []compareListEntry `json:"reports"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res
}

func putBoardRaw(t *testing.T, s *server, b *Board) {
	t.Helper()
	data, _ := json.Marshal(b)
	w := httptest.NewRecorder()
	s.putBoard(w, httptest.NewRequest("PUT", "/api/board", bytes.NewReader(data)))
	if w.Code != 200 {
		t.Fatalf("put failed: %s", w.Body.String())
	}
}

// A frozen compare report (with its basis) survives a full save +
// restart: the reloaded server lists it and still recognizes the
// unchanged board as its basis.
func TestComparePersistsAcrossReload(t *testing.T) {
	s := newTestServer(t)
	rep := postCompare(t, s, s.board)
	if rep.ID == "" || rep.Key == "" {
		t.Fatalf("frozen report must carry id and key: %+v", rep)
	}

	// Simulate a restart: fresh server over the same store file.
	loaded, err := s.st.load()
	if err != nil {
		t.Fatal(err)
	}
	s2 := &server{st: s.st, board: loaded}
	res := getCompares(t, s2)
	if len(res.Reports) != 1 {
		t.Fatalf("expected 1 stored report after reload, got %d", len(res.Reports))
	}
	g := res.Reports[0]
	if g.ID != rep.ID || !g.Current {
		t.Fatalf("reloaded report = %+v, want id %s current=true", g, rep.ID)
	}
	if g.Basis == nil || g.Basis.Revision != s.board.Revision {
		t.Fatalf("report must keep its basis snapshot: %+v", g.Basis)
	}
	if len(g.Report.Variants) != 4 || g.Report.Diff == nil {
		t.Fatalf("report content incomplete after reload: %+v", g.Report)
	}
}

// Re-checking after the board changed appends a new report; the old one
// is kept and flagged as no longer matching the current board.
func TestCompareRecheckKeepsOldReports(t *testing.T) {
	s := newTestServer(t)
	first := postCompare(t, s, s.board)

	// Change geometry and save (revision bumps).
	var b Board
	data, _ := json.Marshal(s.board)
	json.Unmarshal(data, &b)
	b.Pads = append(b.Pads, Pad{ID: "pad-new", NetID: "net-vcc", Layer: "top",
		Shape: "circle", X: 20 * mm, Y: 35 * mm, Diameter: mm})
	putBoardRaw(t, s, &b)

	second := postCompare(t, s, s.board)
	if second.ID == first.ID {
		t.Fatal("re-check must generate a new report, not reuse the old id")
	}
	res := getCompares(t, s)
	if len(res.Reports) != 2 {
		t.Fatalf("old report must be kept alongside the new one, got %d", len(res.Reports))
	}
	if res.Reports[0].ID != first.ID || res.Reports[0].Current {
		t.Fatalf("old report should be kept and flagged stale: %+v", res.Reports[0])
	}
	if res.Reports[1].ID != second.ID || !res.Reports[1].Current {
		t.Fatalf("new report should be current: %+v", res.Reports[1])
	}
	if res.Reports[0].Basis == nil || len(res.Reports[0].Basis.Pads) != len(s.board.Pads)-1 {
		t.Fatal("old report's basis must still describe the old geometry")
	}
}

// Saving the board (a client PUT carries no compare history) must not
// wipe the reports stored with the project.
func TestCompareReportsSurviveSave(t *testing.T) {
	s := newTestServer(t)
	postCompare(t, s, s.board)
	var b Board
	data, _ := json.Marshal(s.board)
	json.Unmarshal(data, &b)
	b.Compares = nil // client does not send the history back
	putBoardRaw(t, s, &b)
	if len(s.board.Compares) != 1 {
		t.Fatalf("save wiped the stored reports: %+v", s.board.Compares)
	}
	loaded, err := s.st.load()
	if err != nil || len(loaded.Compares) != 1 {
		t.Fatalf("stored reports not persisted: %v %+v", err, loaded.Compares)
	}
}

// Undo-then-save restores identical geometry under a new revision; the
// old report must be recognized as matching again (the key fingerprints
// geometry and assembly, not the revision counter).
func TestCompareCurrentAgainAfterUndoGeometry(t *testing.T) {
	s := newTestServer(t)
	postCompare(t, s, s.board)

	var edited Board
	data, _ := json.Marshal(s.board)
	json.Unmarshal(data, &edited)
	edited.Variants[1].Jumpers = nil // toggle the jumper off in variant B
	putBoardRaw(t, s, &edited)
	if getCompares(t, s).Reports[0].Current {
		t.Fatal("report must be flagged stale while the board differs")
	}

	// Undo: restore the exact original geometry and save again.
	var restored Board
	json.Unmarshal(data, &restored)
	restored.Compares = nil
	putBoardRaw(t, s, &restored)
	res := getCompares(t, s)
	if !res.Reports[0].Current {
		t.Fatalf("identical geometry under a new revision must match the old basis (rev %d)", res.Revision)
	}
}

// A stored report whose own data is partially missing (no report lists,
// damaged basis) must not keep the remaining reports from loading.
func TestCompareListToleratesMissingEntryData(t *testing.T) {
	dir := t.TempDir()
	st := newStore(filepath.Join(dir, "board.json"))
	b := demoBoard()
	good := newStoredCompare(b, CompareVariants(b))
	data, _ := json.Marshal(b)
	var raw map[string]any
	json.Unmarshal(data, &raw)
	raw["compares"] = []any{
		map[string]any{"id": "broken", "revision": 3}, // no report, no basis
	}
	data, _ = json.Marshal(raw)
	var carrier Board
	json.Unmarshal(data, &carrier)
	carrier.Compares = append(carrier.Compares, good)
	if err := st.save(&carrier); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.load()
	if err != nil {
		t.Fatalf("one damaged entry must not break loading: %v", err)
	}
	if len(loaded.Compares) != 2 {
		t.Fatalf("both entries must load, got %d", len(loaded.Compares))
	}
	s := &server{st: st, board: loaded}
	res := getCompares(t, s)
	if len(res.Reports) != 2 {
		t.Fatalf("list must return both entries, got %d", len(res.Reports))
	}
	if res.Reports[0].Report.Variants == nil || res.Reports[0].Report.Diff == nil {
		t.Fatal("damaged entry must be sanitized to empty lists, not dropped or null")
	}
	if len(res.Reports[1].Report.Variants) != 4 || !res.Reports[1].Current {
		t.Fatalf("intact report must recover fully and stay current: %+v", res.Reports[1])
	}
}

// Comparing an unsaved posted board freezes that revision as the basis
// without replacing the saved board's geometry.
func TestCompareStoresPostedBasisWithoutTouchingSavedBoard(t *testing.T) {
	s := newTestServer(t)
	var posted Board
	data, _ := json.Marshal(s.board)
	json.Unmarshal(data, &posted)
	posted.Pads = posted.Pads[:len(posted.Pads)-1] // unsaved edit
	rep := postCompare(t, s, &posted)
	if len(s.board.Pads) != len(posted.Pads)+1 {
		t.Fatal("compare must not modify the saved board")
	}
	res := getCompares(t, s)
	if len(res.Reports) != 1 || res.Reports[0].ID != rep.ID {
		t.Fatalf("report not stored: %+v", res.Reports)
	}
	if res.Reports[0].Current {
		t.Fatal("basis is the unsaved edit, not the saved board: must be stale")
	}
	if len(res.Reports[0].Basis.Pads) != len(posted.Pads) {
		t.Fatal("basis snapshot must match the posted board, not the saved one")
	}
}
