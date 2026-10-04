package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newCompareTestServer(t *testing.T) *server {
	t.Helper()
	st := newStore(filepath.Join(t.TempDir(), "board.json"))
	b := demoBoard()
	if err := st.save(b); err != nil {
		t.Fatal(err)
	}
	return &server{st: st, compares: newCompareStore(st.path), board: b}
}

func postCompare(t *testing.T, s *server, sc StoredCompare) StoredCompare {
	t.Helper()
	body, _ := json.Marshal(sc)
	req := httptest.NewRequest("POST", "/api/board/compares", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.saveCompare(rec, req)
	if rec.Code != 200 {
		t.Fatalf("saveCompare: %d %s", rec.Code, rec.Body.String())
	}
	var out StoredCompare
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func listComparesOf(t *testing.T, s *server) ([]StoredCompare, []string) {
	t.Helper()
	req := httptest.NewRequest("GET", "/api/board/compares", nil)
	rec := httptest.NewRecorder()
	s.listCompares(rec, req)
	if rec.Code != 200 {
		t.Fatalf("listCompares: %d", rec.Code)
	}
	var env struct {
		Compares []StoredCompare `json:"compares"`
		Warnings []string        `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	return env.Compares, env.Warnings
}

// A frozen report survives a simulated restart (fresh server over the
// same files) with its basis snapshot intact.
func TestComparePersistsAcrossRestart(t *testing.T) {
	s := newCompareTestServer(t)
	basis := demoBoard()
	rep := CompareVariants(basis)
	saved := postCompare(t, s, StoredCompare{Key: "k1", Report: rep, Basis: *basis})
	if saved.ID == "" {
		t.Fatal("stored compare has no id")
	}

	s2 := &server{st: s.st, compares: newCompareStore(s.st.path), board: s.board}
	got, warnings := listComparesOf(t, s2)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(got) != 1 || got[0].Key != "k1" {
		t.Fatalf("expected the frozen report after restart, got %+v", got)
	}
	if len(got[0].Basis.Pads) != len(basis.Pads) {
		t.Fatalf("basis snapshot lost pads: %d vs %d", len(got[0].Basis.Pads), len(basis.Pads))
	}
	if len(got[0].Report.Variants) != len(rep.Variants) {
		t.Fatalf("report variants lost: %d vs %d", len(got[0].Report.Variants), len(rep.Variants))
	}
}

// Re-checking appends a new report; the old one is kept.
func TestCompareRecheckKeepsOldReports(t *testing.T) {
	s := newCompareTestServer(t)
	basis := demoBoard()
	postCompare(t, s, StoredCompare{Key: "old", Report: CompareVariants(basis), Basis: *basis})
	postCompare(t, s, StoredCompare{Key: "new", Report: CompareVariants(basis), Basis: *basis})
	got, _ := listComparesOf(t, s)
	if len(got) != 2 || got[0].Key != "old" || got[1].Key != "new" {
		t.Fatalf("re-check must append, not replace: %+v", got)
	}
	if got[0].ID == got[1].ID {
		t.Fatal("stored reports need distinct ids")
	}
}

// One damaged entry must not take the other reports down with it.
func TestCompareToleratesSingleCorruptEntry(t *testing.T) {
	s := newCompareTestServer(t)
	basis := demoBoard()
	postCompare(t, s, StoredCompare{Key: "good", Report: CompareVariants(basis), Basis: *basis})

	data, err := os.ReadFile(s.compares.path)
	if err != nil {
		t.Fatal(err)
	}
	var list []json.RawMessage
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	list = append(list, json.RawMessage(`{"id": 123, "report": "broken"}`))
	broken, _ := json.Marshal(list)
	if err := os.WriteFile(s.compares.path, broken, 0o644); err != nil {
		t.Fatal(err)
	}

	got, warnings := listComparesOf(t, s)
	if len(got) != 1 || got[0].Key != "good" {
		t.Fatalf("good report must survive a corrupt sibling: %+v", got)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "已跳过") {
		t.Fatalf("expected a skip warning, got %v", warnings)
	}
}

// A stored report whose basis is missing whole sections still loads,
// sanitized, instead of failing the listing.
func TestCompareBasisPartiallyMissing(t *testing.T) {
	s := newCompareTestServer(t)
	body := `{"key":"k","report":{"revision":1,"variants":[],"diff":[]},"basis":{"id":"b","name":"x"}}`
	req := httptest.NewRequest("POST", "/api/board/compares", strings.NewReader(body))
	rec := httptest.NewRecorder()
	s.saveCompare(rec, req)
	if rec.Code != 200 {
		t.Fatalf("saveCompare: %d %s", rec.Code, rec.Body.String())
	}
	got, warnings := listComparesOf(t, s)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(got) != 1 || got[0].Basis.Layers == nil || got[0].Basis.Variants == nil {
		t.Fatalf("basis must be sanitized on load: %+v", got)
	}
}
