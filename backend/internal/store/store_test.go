package store

import (
	"testing"

	"pcbworkbench/internal/model"
)

func sampleBoard() model.Board {
	return model.Board{
		ID:   "board-1",
		Name: "crossing board",
		Components: []model.Component{
			{ID: "comp-U1", Reference: "U1", Footprint: "DIP-2"},
		},
		Pads: []model.Pad{
			{ID: "pad-U1-1", ComponentID: "comp-U1", Name: "1", Net: "A", X: 0, Y: 0, Radius: 0.5, Layers: []string{"top"}},
			{ID: "pad-U1-2", ComponentID: "comp-U1", Name: "2", Net: "B", X: 5, Y: 0, Radius: 0.5, Layers: []string{"bottom"}},
		},
		Traces: []model.Trace{
			{ID: "trace-1", Net: "A", Layer: "top", Width: 0.3, Points: []model.Point{{X: 0, Y: 0}, {X: 5, Y: 5}}},
		},
		Vias: []model.Via{
			{ID: "via-1", Net: "A", X: 5, Y: 5, Radius: 0.3, Layers: []string{"top", "bottom"}},
		},
	}
}

func TestSaveUndoRoundTrip(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.Create(sampleBoard())
	if err != nil {
		t.Fatal(err)
	}
	rev := b
	rev.Name = "edited"
	rev.Traces = append(rev.Traces, model.Trace{ID: "trace-2", Layer: "bottom", Width: 0.2, Points: []model.Point{{X: 1}, {X: 2}}})
	if _, err := s.Save(b.ID, rev); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(b.ID)
	if err != nil || got.Name != "edited" || len(got.Traces) != 2 {
		t.Fatalf("save failed: %+v %v", got, err)
	}
	prev, err := s.Undo(b.ID)
	if err != nil || prev.Name != "crossing board" || len(prev.Traces) != 1 {
		t.Fatalf("undo failed: %+v %v", prev, err)
	}
	redone, err := s.Redo(b.ID)
	if err != nil || redone.Name != "edited" || len(redone.Traces) != 2 {
		t.Fatalf("redo failed: %+v %v", redone, err)
	}
}

// Export is the raw board JSON; re-importing it must preserve every identity.
func TestImportPreservesAllIDs(t *testing.T) {
	dir1 := t.TempDir()
	s1, _ := New(dir1)
	b, err := s1.Create(sampleBoard())
	if err != nil {
		t.Fatal(err)
	}

	dir2 := t.TempDir()
	s2, _ := New(dir2)
	got, err := s2.Import(b, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{got.ID, got.Components[0].ID, got.Pads[0].ID, got.Pads[1].ID, got.Traces[0].ID, got.Vias[0].ID}
	want := []string{"board-1", "comp-U1", "pad-U1-1", "pad-U1-2", "trace-1", "via-1"}
	for i := range ids {
		if ids[i] != want[i] {
			t.Fatalf("identity %d changed: got %s want %s", i, ids[i], want[i])
		}
	}

	// Reopening the data directory restores the board with the same IDs.
	s3, err := New(dir2)
	if err != nil {
		t.Fatal(err)
	}
	boards := s3.List()
	if len(boards) != 1 || boards[0].Pads[0].ID != "pad-U1-1" || boards[0].Vias[0].Layers[1] != "bottom" {
		t.Fatalf("reopened data lost identity/layers: %+v", boards)
	}
}
