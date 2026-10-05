package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// repairBoard has two open nets (A, C) and one forbidden-short net (B):
// jumper j-a-short connects A but crosses B's copper (shared connection
// point), j-a-clean connects A via a detour, and j-c-1 / j-c-2 are two
// mutually exclusive ways to connect C.
func repairBoard() *Board {
	return &Board{
		ID: "board-repair", Name: "修复求解测试板", Revision: 3,
		Outline: []Point{pt(0, 0), pt(20, 0), pt(20, 20), pt(0, 20)},
		Layers:  []string{"top"},
		Nets: []Net{
			{ID: "net-a", Name: "A"}, {ID: "net-b", Name: "B"}, {ID: "net-c", Name: "C"},
		},
		Pads: []Pad{
			{ID: "pad-a1", NetID: "net-a", Layer: "top", Shape: "circle", X: 2 * mm, Y: 2 * mm, Diameter: 2 * mm},
			{ID: "pad-a2", NetID: "net-a", Layer: "top", Shape: "circle", X: 12 * mm, Y: 2 * mm, Diameter: 2 * mm},
			{ID: "pad-c1", NetID: "net-c", Layer: "top", Shape: "circle", X: 2 * mm, Y: 12 * mm, Diameter: 2 * mm},
			{ID: "pad-c2", NetID: "net-c", Layer: "top", Shape: "circle", X: 12 * mm, Y: 12 * mm, Diameter: 2 * mm},
		},
		Traces: []Trace{
			// B copper bar crossing the direct A path at (7, 2).
			{ID: "trace-b", NetID: "net-b", Layer: "top", Width: 1 * mm,
				Points: []Point{pt(7, 0), pt(7, 4)}},
		},
		Vias: []Via{},
		Jumpers: []Jumper{
			// Straight A link: shorts into B at the crossing.
			{ID: "j-a-short", NetID: "net-a", Layer: "top", Width: 1 * mm,
				Points: []Point{pt(2, 2), pt(12, 2)}},
			// Detour A link: stays clear of B.
			{ID: "j-a-clean", NetID: "net-a", Layer: "top", Width: 1 * mm,
				Points: []Point{pt(2, 2), pt(2, 6), pt(12, 6), pt(12, 2)}},
			// Two equivalent C links.
			{ID: "j-c-1", NetID: "net-c", Layer: "top", Width: 1 * mm,
				Points: []Point{pt(2, 12), pt(12, 12)}},
			{ID: "j-c-2", NetID: "net-c", Layer: "top", Width: 1 * mm,
				Points: []Point{pt(2, 12), pt(2, 16), pt(12, 16), pt(12, 12)}},
		},
		Variants: []Variant{},
	}
}

func repairMods() []RepairMod {
	return []RepairMod{
		{ID: "m-a-short", Kind: "jumper", JumperID: "j-a-short", Cost: 1},
		{ID: "m-a-clean", Kind: "jumper", JumperID: "j-a-clean", Cost: 5},
		{ID: "m-c-1", Kind: "jumper", JumperID: "j-c-1", Cost: 2, Excludes: []string{"m-c-2"}},
		{ID: "m-c-2", Kind: "jumper", JumperID: "j-c-2", Cost: 2, Excludes: []string{"m-c-1"}},
	}
}

func solveReq(mods []RepairMod) *RepairRequest {
	return &RepairRequest{
		RequireConnected: []string{"net-a", "net-c"},
		ForbidShort:      []string{"net-b"},
		Mods:             mods,
	}
}

// The solver must not grab the cheapest jumper that fixes the first open
// it sees: j-a-short is cheaper but shorts B at the shared crossing, so
// every combination containing it is rejected and the detour wins.
func TestRepairSharedPointShortExcluded(t *testing.T) {
	b := repairBoard()
	res := SolveRepair(b, solveReq(repairMods()))
	if res.Status != "ok" {
		t.Fatalf("expected ok, got %s: %v", res.Status, res.Conflicts)
	}
	if len(res.Solutions) != 2 {
		t.Fatalf("expected 2 equal-cost combos (m-c-1 or m-c-2), got %+v", res.Solutions)
	}
	for _, sol := range res.Solutions {
		if sol.Cost != 7 { // m-a-clean (5) + one C link (2)
			t.Fatalf("unexpected min cost %d in %+v", sol.Cost, sol)
		}
		for _, id := range sol.ModIDs {
			if id == "m-a-short" {
				t.Fatalf("shorting jumper must be excluded from every solution: %+v", sol)
			}
		}
		hasClean := false
		for _, id := range sol.ModIDs {
			if id == "m-a-clean" {
				hasClean = true
			}
		}
		if !hasClean {
			t.Fatalf("solution must use the detour jumper: %+v", sol)
		}
	}
}

// With only the shorting jumper available there is no valid combination;
// the conflict report must name the forbidden short it would cause.
func TestRepairNoSolutionReportsConflict(t *testing.T) {
	b := repairBoard()
	mods := []RepairMod{
		{ID: "m-a-short", Kind: "jumper", JumperID: "j-a-short", Cost: 1},
		{ID: "m-c-1", Kind: "jumper", JumperID: "j-c-1", Cost: 2},
	}
	res := SolveRepair(b, solveReq(mods))
	if res.Status != "no-solution" {
		t.Fatalf("expected no-solution, got %s with %+v", res.Status, res.Solutions)
	}
	if len(res.Conflicts) == 0 {
		t.Fatal("no-solution must come with reviewable conflicts")
	}
	joined := strings.Join(res.Conflicts, "\n")
	if !strings.Contains(joined, "m-a-short") || !strings.Contains(joined, "B") {
		t.Fatalf("conflict must name the mod and the shorted net, got:\n%s", joined)
	}
}

// Mutually exclusive jumpers must never appear in the same combination,
// even when installing both would still satisfy the electrical rules.
func TestRepairMutualExclusionRespected(t *testing.T) {
	b := repairBoard()
	mods := repairMods()
	mods[2].Excludes = nil
	mods[3].Excludes = nil
	mods[2].Excludes = []string{"m-c-2"}
	res := SolveRepair(b, solveReq(mods))
	if res.Status != "ok" {
		t.Fatalf("expected ok, got %s", res.Status)
	}
	for _, sol := range res.Solutions {
		has1, has2 := false, false
		for _, id := range sol.ModIDs {
			if id == "m-c-1" {
				has1 = true
			}
			if id == "m-c-2" {
				has2 = true
			}
		}
		if has1 && has2 {
			t.Fatalf("mutually exclusive mods in one solution: %+v", sol)
		}
	}
}

// Missing nets, dangling mod references and absent board sections are
// incompleteness, never a silent pass.
func TestRepairMissingDataNotPassed(t *testing.T) {
	b := repairBoard()
	req := solveReq(repairMods())
	req.RequireConnected = append(req.RequireConnected, "net-ghost")
	if res := SolveRepair(b, req); res.Status != "incomplete" {
		t.Fatalf("unknown required net must be incomplete, got %s", res.Status)
	}
	req2 := solveReq([]RepairMod{{ID: "m-x", Kind: "jumper", JumperID: "j-ghost", Cost: 1}})
	if res := SolveRepair(b, req2); res.Status != "incomplete" {
		t.Fatalf("dangling jumper reference must be incomplete, got %s", res.Status)
	}
	// Board JSON missing whole sections must not come back as a clean solve.
	s := newRepairTestServer(t)
	body := []byte(`{"board":{"id":"x","revision":1,"nets":[{"id":"net-a","name":"A"}]},` +
		`"requireConnected":["net-a"],"mods":[]}`)
	r := httptest.NewRequest("POST", "/api/board/repair/solve", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.solveRepair(w, r)
	if w.Code != 200 {
		t.Fatalf("solve failed: %s", w.Body.String())
	}
	var res RepairResult
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Status != "incomplete" || len(res.Warnings) == 0 {
		t.Fatalf("missing sections must be flagged incomplete, got %+v", res)
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
	return &server{
		st: st, board: b,
		compares: newCompareStore(st.path),
		repairs:  newRepairStore(st.path),
	}
}

// Applying a combination re-verifies against the current revision,
// refuses stale bases, and saves the new verification separately from
// the frozen compare reports.
func TestRepairApplyFlow(t *testing.T) {
	s := newRepairTestServer(t)
	mods := repairMods()
	apply := map[string]any{
		"variantId": "", "mods": []RepairMod{mods[1], mods[2]},
		"requireConnected": []string{"net-a", "net-c"},
		"forbidShort":      []string{"net-b"},
		"basisRevision":    99, // stale on purpose
	}
	body, _ := json.Marshal(apply)
	r := httptest.NewRequest("POST", "/api/board/repair/apply", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.applyRepair(w, r)
	if w.Code != 409 {
		t.Fatalf("stale basis must be rejected, got %d: %s", w.Code, w.Body.String())
	}
	if s.board.Revision != 3 {
		t.Fatalf("rejected apply must not change the board, revision %d", s.board.Revision)
	}

	apply["basisRevision"] = 3
	body, _ = json.Marshal(apply)
	r = httptest.NewRequest("POST", "/api/board/repair/apply", bytes.NewReader(body))
	w = httptest.NewRecorder()
	s.applyRepair(w, r)
	if w.Code != 200 {
		t.Fatalf("apply failed: %s", w.Body.String())
	}
	var res struct {
		Board     Board  `json:"board"`
		VariantID string `json:"variantId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if res.Board.Revision != 4 {
		t.Fatalf("apply must bump the revision, got %d", res.Board.Revision)
	}
	var v *Variant
	for i := range res.Board.Variants {
		if res.Board.Variants[i].ID == res.VariantID {
			v = &res.Board.Variants[i]
		}
	}
	if v == nil || len(v.Jumpers) != 2 {
		t.Fatalf("applied variant must install both jumpers, got %+v", res.Board.Variants)
	}
	// The applied assembly must actually satisfy the constraints.
	chk := AnalyzeVariant(&res.Board, res.VariantID)
	states := map[string]string{}
	for _, ns := range chk.NetStatus {
		states[ns.NetID] = ns.State
	}
	if states["net-a"] != "connected" || states["net-c"] != "connected" || states["net-b"] == "short" {
		t.Fatalf("applied assembly violates constraints: %v", states)
	}
	// New verification saved separately; compare archive untouched.
	repairs, _, err := s.repairs.loadAll()
	if err != nil || len(repairs) != 1 {
		t.Fatalf("expected 1 saved repair record, got %v %v", repairs, err)
	}
	if repairs[0].BasisRevision != 3 || repairs[0].AppliedRevision != 4 {
		t.Fatalf("repair record must pin both revisions, got %+v", repairs[0])
	}
	compares, _, err := s.compares.loadAll()
	if err != nil || len(compares) != 0 {
		t.Fatalf("repair apply must not touch frozen compares, got %v", compares)
	}
}

// Preview reports each modification's own effect on the networks before
// anything is applied.
func TestRepairPreview(t *testing.T) {
	s := newRepairTestServer(t)
	body, _ := json.Marshal(map[string]any{
		"board": repairBoard(), "variantId": "", "mods": repairMods(),
	})
	r := httptest.NewRequest("POST", "/api/board/repair/preview", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.previewRepair(w, r)
	if w.Code != 200 {
		t.Fatalf("preview failed: %s", w.Body.String())
	}
	var res struct {
		Mods []struct {
			ModID   string     `json:"modId"`
			Changes []NetShift `json:"changes"`
		} `json:"mods"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if len(res.Mods) != 4 {
		t.Fatalf("expected one preview per mod, got %+v", res.Mods)
	}
	var shortPreview *struct {
		ModID   string     `json:"modId"`
		Changes []NetShift `json:"changes"`
	}
	for i := range res.Mods {
		if res.Mods[i].ModID == "m-a-short" {
			shortPreview = &res.Mods[i]
		}
	}
	if shortPreview == nil {
		t.Fatal("missing preview for m-a-short")
	}
	sawShort := false
	for _, ch := range shortPreview.Changes {
		if ch.NetID == "net-b" && ch.After == "short" {
			sawShort = true
		}
	}
	if !sawShort {
		t.Fatalf("preview must show j-a-short dragging B into a short, got %+v", shortPreview.Changes)
	}
}
