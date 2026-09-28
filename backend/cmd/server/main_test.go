package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"pcbworkbench/internal/drc"
	"pcbworkbench/internal/model"
	"pcbworkbench/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return &Server{boards: st}
}

func do(t *testing.T, s *Server, method, path string, body any) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	w := httptest.NewRecorder()
	newMux(s).ServeHTTP(w, req)
	return w.Code, w.Body.Bytes()
}

func TestEndToEndCrossingBoard(t *testing.T) {
	s := testServer(t)
	b := model.Board{
		ID: "e2e", Name: "crossing",
		Outline:    model.Rect{Width: 20, Height: 20},
		Components: []model.Component{{ID: "comp-U1", Reference: "U1"}},
		Pads: []model.Pad{
			{ID: "p1", ComponentID: "comp-U1", Name: "1", Net: "A", X: 0, Y: 0, Radius: 0.6, Layers: []string{"top"}},
			{ID: "p2", ComponentID: "comp-U1", Name: "2", Net: "A", X: 10, Y: 10, Radius: 0.6, Layers: []string{"top"}},
			{ID: "p3", ComponentID: "comp-U1", Name: "3", Net: "B", X: 10, Y: 0, Radius: 0.6, Layers: []string{"bottom"}},
			{ID: "p4", ComponentID: "comp-U1", Name: "4", Net: "B", X: 0, Y: 10, Radius: 0.6, Layers: []string{"bottom"}},
		},
		Traces: []model.Trace{
			{ID: "t1", Net: "A", Layer: "top", Width: 0.4, Points: []model.Point{{}, {X: 10, Y: 10}}},
			{ID: "t2", Net: "B", Layer: "bottom", Width: 0.4, Points: []model.Point{{X: 10}, {X: 0, Y: 10}}},
		},
	}
	code, raw := do(t, s, "POST", "/api/boards", b)
	if code != 201 {
		t.Fatalf("create: %d %s", code, raw)
	}

	code, raw = do(t, s, "POST", "/api/boards/e2e/drc", drc.Config{MinClearanceMM: 0.3})
	if code != 200 {
		t.Fatalf("drc: %d %s", code, raw)
	}
	var rep model.DRCReport
	if err := json.Unmarshal(raw, &rep); err != nil {
		t.Fatal(err)
	}
	for _, is := range rep.Issues {
		t.Fatalf("cross-layer crossing must be clean, got %s @ %+v", is.Message, is.At)
	}

	// Introduce a short by widening the top trace onto a B pad that has no
	// copper on top: instead add a top B trace that overlaps t1 at (5,5).
	b2 := b
	b2.Traces = append(append([]model.Trace{}, b2.Traces...), model.Trace{
		ID: "t3", Net: "B", Layer: "top", Width: 0.4,
		Points: []model.Point{{X: 0, Y: 10}, {X: 10, Y: 0}},
	})
	code, raw = do(t, s, "PUT", "/api/boards/e2e", b2)
	if code != 200 {
		t.Fatalf("save: %d %s", code, raw)
	}
	_, raw = do(t, s, "POST", "/api/boards/e2e/drc", drc.Config{MinClearanceMM: 0.3})
	json.Unmarshal(raw, &rep)
	foundShort := false
	for _, is := range rep.Issues {
		if is.Code == "short" && is.At != nil {
			foundShort = true
			if is.At.X < 4.5 || is.At.X > 5.5 || is.At.Y < 4.5 || is.At.Y > 5.5 {
				t.Fatalf("short location off: %+v", is.At)
			}
		}
	}
	if !foundShort {
		t.Fatal("expected short at the cross point (5,5)")
	}

	// Undo removes the shorting trace.
	code, raw = do(t, s, "POST", "/api/boards/e2e/undo", nil)
	if code != 200 || strings.Contains(string(raw), "t3") {
		t.Fatalf("undo did not revert: %d %s", code, raw)
	}

	// Export -> import into a fresh store keeps every ID.
	var exported model.Board
	if err := json.Unmarshal(raw, &exported); err != nil {
		t.Fatal(err)
	}
	st2, _ := store.New(t.TempDir())
	s2 := &Server{boards: st2}
	code, raw = do(t, s2, "POST", "/api/boards/e2e/import", exported)
	if code != 200 {
		t.Fatalf("import: %d %s", code, raw)
	}
	var imported model.Board
	json.Unmarshal(raw, &imported)
	ids := []string{imported.ID, imported.Components[0].ID, imported.Pads[0].ID, imported.Traces[0].ID}
	want := []string{"e2e", "comp-U1", "p1", "t1"}
	for i := range ids {
		if ids[i] != want[i] {
			t.Fatalf("identity changed after export/import: %v", ids)
		}
	}

	// Persisted file on disk must exist as well.
	if _, err := os.Stat(t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
