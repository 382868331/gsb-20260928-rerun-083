package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// repairBoard: net-a is open between pad-a1's trace (ends at x=20) and
// pad-a2 (edge at x=29). Jumper j1 bridges the gap straight through a
// net-b trace at x=25 (shared connection point -> short). Jumpers j2
// and j3 take a detour around net-b and fix net-a cleanly.
func repairBoard() *Board {
	b := &Board{
		ID:      "board-repair",
		Name:    "修复求解验证板",
		Outline: []Point{pt(0, 0), pt(60, 0), pt(60, 40), pt(0, 40)},
		Layers:  []string{"top", "bottom"},
		Nets: []Net{
			{ID: "net-a", Name: "A"},
			{ID: "net-b", Name: "B"},
			{ID: "net-c", Name: "C"},
		},
		Pads: []Pad{
			{ID: "pad-a1", NetID: "net-a", Layer: "top", Shape: "circle", X: 10 * mm, Y: 10 * mm, Diameter: 2 * mm},
			{ID: "pad-a2", NetID: "net-a", Layer: "top", Shape: "circle", X: 30 * mm, Y: 10 * mm, Diameter: 2 * mm},
			{ID: "pad-b1", NetID: "net-b", Layer: "top", Shape: "circle", X: 25 * mm, Y: 5 * mm, Diameter: 2 * mm},
			{ID: "pad-b2", NetID: "net-b", Layer: "top", Shape: "circle", X: 25 * mm, Y: 15 * mm, Diameter: 2 * mm},
			{ID: "pad-c1", NetID: "net-c", Layer: "top", Shape: "circle", X: 40 * mm, Y: 30 * mm, Diameter: 2 * mm},
			{ID: "pad-c2", NetID: "net-c", Layer: "top", Shape: "circle", X: 48 * mm, Y: 30 * mm, Diameter: 2 * mm},
			{ID: "pad-c3", NetID: "net-c", Layer: "top", Shape: "circle", X: 56 * mm, Y: 30 * mm, Diameter: 2 * mm},
		},
		Traces: []Trace{
			{ID: "trace-a1", NetID: "net-a", Layer: "top", Width: 500_000, Points: []Point{pt(10, 10), pt(20, 10)}},
			{ID: "trace-b", NetID: "net-b", Layer: "top", Width: 500_000, Points: []Point{pt(25, 5), pt(25, 15)}},
			{ID: "trace-c1", NetID: "net-c", Layer: "top", Width: 500_000, Points: []Point{pt(40, 30), pt(47.5, 30)}},
			{ID: "trace-c2", NetID: "net-c", Layer: "top", Width: 500_000, Points: []Point{pt(48.5, 30), pt(56, 30)}},
		},
		Jumpers: []Jumper{
			// Straight across the gap, but crosses trace-b at (25,10).
			{ID: "j1", NetID: "net-a", Layer: "top", Width: 500_000, Points: []Point{pt(20, 10), pt(30, 10)}},
			// Detour around net-b.
			{ID: "j2", NetID: "net-a", Layer: "top", Width: 500_000, Points: []Point{pt(20, 10), pt(20, 2), pt(30, 2), pt(30, 10)}},
			{ID: "j3", NetID: "net-a", Layer: "top", Width: 500_000, Points: []Point{pt(20, 10), pt(20, 18), pt(30, 18), pt(30, 10)}},
		},
		Variants: []Variant{
			{ID: "var-noc", Name: "免C器件", DNP: []string{"pad-c2"}, Jumpers: []string{}},
		},
	}
	sanitizeBoard(b)
	return b
}

func repairSpec(mods ...RepairMod) *RepairSpec {
	return &RepairSpec{MustConnect: []string{"net-a"}, NoShort: []string{"net-b"}, Mods: mods}
}

func TestRepairMutexJumpersAndMinCost(t *testing.T) {
	b := repairBoard()
	spec := repairSpec(
		RepairMod{ID: "m1", Kind: "jumper", Target: "j1", Cost: 1},
		RepairMod{ID: "m2", Kind: "jumper", Target: "j2", Cost: 2, Excludes: []string{"m3"}},
		RepairMod{ID: "m3", Kind: "jumper", Target: "j3", Cost: 2},
	)
	res := SolveRepair(b, "", spec)
	if len(res.Solutions) != 2 {
		t.Fatalf("expected 2 equal-cost solutions, got %+v (conflicts: %v)", res.Solutions, res.Conflicts)
	}
	for _, sol := range res.Solutions {
		if sol.Cost != 2 {
			t.Fatalf("min cost should be 2 (j1 shorts net-b), got %d", sol.Cost)
		}
		if len(sol.Mods) != 1 || sol.Mods[0] == "m1" {
			t.Fatalf("solution must be exactly one of m2/m3, got %v", sol.Mods)
		}
	}
	if res.Solutions[0].Mods[0] == res.Solutions[1].Mods[0] {
		t.Fatalf("the two solutions must be different combinations: %+v", res.Solutions)
	}
	// The shorting jumper alone must preview as a conflict, not a fix.
	var m1 *ModEffect
	for i := range res.ModEffects {
		if res.ModEffects[i].ModID == "m1" {
			m1 = &res.ModEffects[i]
		}
	}
	if m1 == nil || len(m1.Conflicts) == 0 {
		t.Fatalf("m1 preview must flag the net-b short, got %+v", res.ModEffects)
	}
}

func TestRepairSharedPointShortConflict(t *testing.T) {
	b := repairBoard()
	// Only the shorting jumper is allowed: no valid combination exists.
	res := SolveRepair(b, "", repairSpec(RepairMod{ID: "m1", Kind: "jumper", Target: "j1", Cost: 1}))
	if len(res.Solutions) != 0 {
		t.Fatalf("j1 shorts net-b at the shared point, must be rejected: %+v", res.Solutions)
	}
	joined := strings.Join(res.Conflicts, "; ")
	if !strings.Contains(joined, "B") || !strings.Contains(joined, "短") {
		t.Fatalf("conflicts must explain the net-b short, got %v", res.Conflicts)
	}
}

func TestRepairMissingDataNotPassed(t *testing.T) {
	b := repairBoard()
	spec := &RepairSpec{
		MustConnect: []string{"net-a", "net-ghost"},
		Mods: []RepairMod{
			{ID: "m1", Kind: "jumper", Target: "j-ghost", Cost: 1},
			{ID: "m2", Kind: "jumper", Target: "j2", Cost: 1},
		},
	}
	res := SolveRepair(b, "", spec)
	if len(res.Solutions) != 0 {
		t.Fatalf("missing net/jumper must not pass as a solution: %+v", res.Solutions)
	}
	joined := strings.Join(res.Warnings, "; ")
	if !strings.Contains(joined, "j-ghost") || !strings.Contains(joined, "net-ghost") {
		t.Fatalf("warnings must name the dangling references, got %v", res.Warnings)
	}
	found := false
	for _, c := range res.Conflicts {
		if strings.Contains(c, "net-ghost") {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflicts must flag the unsatisfiable net, got %v", res.Conflicts)
	}
}

func TestRepairPopulateMod(t *testing.T) {
	b := repairBoard()
	// In variant var-noc, pad-c2 is depopulated so net-c is open; the
	// only fix is re-populating it (更换/补装器件).
	spec := &RepairSpec{
		MustConnect: []string{"net-c"},
		Mods: []RepairMod{
			{ID: "m1", Kind: "populate", Target: "pad-c2", Cost: 3},
			{ID: "m2", Kind: "jumper", Target: "j2", Cost: 1}, // irrelevant net
		},
	}
	res := SolveRepair(b, "var-noc", spec)
	if len(res.Solutions) != 1 || res.Solutions[0].Cost != 3 || res.Solutions[0].Mods[0] != "m1" {
		t.Fatalf("only populating pad-c2 fixes net-c, got %+v", res.Solutions)
	}
}

func newRepairTestServer(t *testing.T) *server {
	t.Helper()
	dir := t.TempDir()
	st := newStore(filepath.Join(dir, "board.json"))
	b := repairBoard()
	if err := st.save(b); err != nil {
		t.Fatal(err)
	}
	return &server{st: st, compares: newCompareStore(st.path), repairs: newRepairStore(st.path), board: b}
}

func solveForTest(t *testing.T, s *server, spec *RepairSpec) RepairResult {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"board": s.board, "variantId": "", "spec": spec})
	r := httptest.NewRequest("POST", "/api/repair/solve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.solveRepair(w, r)
	if w.Code != 200 {
		t.Fatalf("solve failed: %s", w.Body.String())
	}
	var res struct {
		Result RepairResult `json:"result"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return res.Result
}

func TestRepairApplyAndStaleKey(t *testing.T) {
	s := newRepairTestServer(t)
	spec := repairSpec(RepairMod{ID: "m2", Kind: "jumper", Target: "j2", Cost: 2})
	res := solveForTest(t, s, spec)
	if len(res.Solutions) != 1 {
		t.Fatalf("expected one solution, got %+v", res.Solutions)
	}
	apply := func(key string, mutate func(b *Board)) *httptest.ResponseRecorder {
		data, _ := json.Marshal(s.board)
		var nb Board
		if err := json.Unmarshal(data, &nb); err != nil {
			t.Fatal(err)
		}
		if mutate != nil {
			mutate(&nb)
		}
		body, _ := json.Marshal(map[string]any{
			"board": &nb, "variantId": "", "spec": spec,
			"mods": res.Solutions[0].Mods, "key": key,
		})
		r := httptest.NewRequest("POST", "/api/repair/apply", bytes.NewReader(body))
		w := httptest.NewRecorder()
		s.applyRepair(w, r)
		return w
	}
	// Board changed after the solve (pad moved): the old key must be
	// refused and a re-solve demanded.
	w := apply(res.Key, func(b *Board) { b.Pads[0].X += 1000 })
	if w.Code != 409 {
		t.Fatalf("stale basis must be refused, got %d: %s", w.Code, w.Body.String())
	}
	// Unchanged board with the solve key applies as one new variant.
	w = apply(res.Key, nil)
	if w.Code != 200 {
		t.Fatalf("apply failed: %s", w.Body.String())
	}
	var out struct {
		Board     Board  `json:"board"`
		VariantID string `json:"variantId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.VariantID == "" || out.Board.Revision != s.board.Revision {
		t.Fatalf("apply must save a new revision with the repair variant, got %+v", out)
	}
	v := variantOf(&out.Board, out.VariantID)
	if v == nil || !containsStr(v.Jumpers, "j2") {
		t.Fatalf("new variant must install j2, got %+v", v)
	}
	// The solved round was frozen as a separate repair report.
	reps, _, err := s.repairs.loadAll()
	if err != nil || len(reps) != 1 {
		t.Fatalf("expected one stored repair report, got %v %v", reps, err)
	}
	if reps[0].Key != res.Key || reps[0].Revision != repairBoard().Revision {
		t.Fatalf("stored report must keep its original basis, got %+v", reps[0])
	}
}

func TestRepairNoSolutionConflictSet(t *testing.T) {
	b := repairBoard()
	// Contradictory requirements: net-a must connect but the only
	// jumpers either short net-b (forbidden) or are excluded together.
	spec := repairSpec(
		RepairMod{ID: "m1", Kind: "jumper", Target: "j1", Cost: 1},
		RepairMod{ID: "m2", Kind: "jumper", Target: "j2", Cost: 1, Excludes: []string{"m3"}},
		RepairMod{ID: "m3", Kind: "jumper", Target: "j3", Cost: 1, Excludes: []string{"m2"}},
	)
	// Force j2/j3 to be unusable by also requiring net-c to stay as-is
	// while demanding net-a connect via m1 only: drop m2/m3 targets by
	// removing the jumpers from the board instead.
	b.Jumpers = b.Jumpers[:1]
	spec.Mods = spec.Mods[:1]
	res := SolveRepair(b, "", spec)
	if len(res.Solutions) != 0 {
		t.Fatalf("expected no solution, got %+v", res.Solutions)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("a reviewable conflict set is required when no solution exists")
	}
}
