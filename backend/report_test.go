package main

import "testing"

// Multi-via chain across two via bridges must explain the short path hop
// by hop, including the unnamed copper in the middle and each layer
// transition.
func TestShortPathStepsAcrossMultipleVias(t *testing.T) {
	b := baseBoard()
	b.Layers = []string{"top", "mid", "bottom"}
	b.Pads = []Pad{
		{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
			X: 5 * mm, Y: 25 * mm, Diameter: 2 * mm},
		{ID: "p2", NetID: "n2", Layer: "bottom", Shape: "circle",
			X: 50 * mm, Y: 25 * mm, Diameter: 2 * mm},
	}
	b.Traces = []Trace{
		{ID: "t1", NetID: "", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(20, 25)}},
		{ID: "t2", NetID: "", Layer: "mid", Width: 500_000,
			Points: []Point{pt(20, 25), pt(35, 25)}},
		{ID: "t3", NetID: "", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(35, 25), pt(50, 25)}},
	}
	b.Vias = []Via{
		{ID: "v1", NetID: "", X: 20 * mm, Y: 25 * mm, Diameter: 1_200_000,
			Layers: []string{"top", "mid"}},
		{ID: "v2", NetID: "", X: 35 * mm, Y: 25 * mm, Diameter: 1_200_000,
			Layers: []string{"mid", "bottom"}},
	}
	errs := CheckBoard(b)
	shorts := errorsOf(errs, "short")
	if len(shorts) != 1 {
		t.Fatalf("expected 1 short via two vias, got %v", errs)
	}
	steps := shorts[0].Steps
	// Expect alternating element/hop: p1, hop, t1, hop, v1, hop, t2,
	// hop, v2, hop, t3, hop, p2 -> 13 steps.
	if len(steps) != 13 {
		t.Fatalf("expected 13 segment steps, got %d: %+v", len(steps), steps)
	}
	for i, st := range steps {
		if i%2 == 0 && st.Kind != "element" {
			t.Fatalf("step %d should be an element, got %+v", i, st)
		}
		if i%2 == 1 && st.Kind != "hop" {
			t.Fatalf("step %d should be a hop, got %+v", i, st)
		}
	}
	wantElems := []string{"p1", "t1", "v1", "t2", "v2", "t3", "p2"}
	for i, id := range wantElems {
		if got := steps[2*i].ID; got != id {
			t.Fatalf("element step %d = %q, want %q", i, got, id)
		}
	}
	// The unnamed middle trace must be labelled as such.
	middle := steps[6] // t2 index? element sequence position 3 -> step 6
	_ = middle
	t2 := steps[6]
	if t2.ID != "t2" || t2.NetID != "" || t2.NetName != "(未命名铜皮)" {
		t.Fatalf("middle trace must be unnamed copper, got %+v", t2)
	}
	// Via elements must list both bridged layers in their step.
	v1 := steps[4]
	if v1.ID != "v1" || v1.Layer != "top+mid" {
		t.Fatalf("via step must name both layers, got %+v", v1)
	}

	// Delete one via: the short disappears; each named pad still forms a
	// valid (single-component) net on its own layer — neither shorted nor
	// split. The unnamed middle trace is simply dangling copper.
	b.Vias = b.Vias[:1]
	if shorts := errorsOf(CheckBoard(b), "short"); len(shorts) != 0 {
		t.Fatalf("removing a via must break the bridge, got %v", shorts)
	}
	status := map[string]NetStatus{}
	for _, ns := range NetSummary(b) {
		status[ns.NetID] = ns
	}
	if status["n1"].State != "connected" || status["n2"].State != "connected" {
		t.Fatalf("both nets should be intact after bridge removal, got %+v %+v",
			status["n1"], status["n2"])
	}
	// Name the middle trace n1 and detach the far bottom pad: now removing
	// the bottom via genuinely splits net n1 (top vs dangling bottom).
	b.Pads[1].X = 60 * mm
	b.Traces[1].NetID = "n1"
	b.Traces[2].NetID = "n1"
	for _, ns := range NetSummary(b) {
		status[ns.NetID] = ns
	}
	if status["n1"].State != "open" {
		t.Fatalf("naming the dangling chain must surface an open, got %+v", status["n1"])
	}

	// Restore the via, pad and unnamed copper: the full chain shorts again.
	b.Pads[1].X = 50 * mm
	b.Traces[1].NetID = ""
	b.Traces[2].NetID = ""
	b.Vias = append(b.Vias, Via{ID: "v2", NetID: "", X: 35 * mm, Y: 25 * mm,
		Diameter: 1_200_000, Layers: []string{"mid", "bottom"}})
	status = map[string]NetStatus{}
	for _, ns := range NetSummary(b) {
		status[ns.NetID] = ns
	}
	if status["n1"].State != "short" || status["n2"].State != "short" {
		t.Fatalf("restored bridge must short both nets, got %+v %+v",
			status["n1"], status["n2"])
	}
}

// Moving a via away from the copper breaks the cross-layer link, even
// though the via object still exists. Summary must flip connected->open.
func TestMoveViaBreaksAndRestoresLink(t *testing.T) {
	b := baseBoard()
	b.Traces = []Trace{
		{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(25, 25)}},
		{ID: "t2", NetID: "n1", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(25, 25), pt(45, 25)}},
	}
	b.Vias = []Via{{ID: "v1", NetID: "n1", X: 25 * mm, Y: 25 * mm,
		Diameter: 1_200_000, Layers: []string{"top", "bottom"}}}
	if st := findStatus(NetSummary(b), "n1"); st.State != "connected" {
		t.Fatalf("via on contact should connect, got %+v", st)
	}
	// Move via 10 mm away (far beyond any touch tolerance).
	b.Vias[0].X = 35 * mm
	if st := findStatus(NetSummary(b), "n1"); st.State != "open" {
		t.Fatalf("moved-away via must open the net, got %+v", st)
	}
	b.Vias[0].X = 25 * mm
	if st := findStatus(NetSummary(b), "n1"); st.State != "connected" {
		t.Fatalf("moving via back should restore connection, got %+v", st)
	}
}

// Adjacent pads of different nets that merely sit close (but do not
// touch) are neither shorted nor opened; unnamed copper touching both is
// the fault and the open report carries segment steps.
func TestAdjacentDifferentNetPadsAndUnnamedCopper(t *testing.T) {
	b := baseBoard()
	b.Pads = []Pad{
		{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
			X: 10 * mm, Y: 25 * mm, Diameter: 1_000_000},
		{ID: "p2", NetID: "n2", Layer: "top", Shape: "circle",
			// 0.2 mm gap between edges (centers 2.2 mm, radii 1 mm).
			X: 12_200_000, Y: 25 * mm, Diameter: 1_000_000},
	}
	if errs := CheckBoard(b); len(errs) != 0 {
		t.Fatalf("close-but-separate pads must be clean, got %v", errs)
	}
	// Drop an unnamed trace onto both: now a short with full steps.
	b.Traces = []Trace{{ID: "tx", NetID: "", Layer: "top", Width: 500_000,
		Points: []Point{pt(10, 25), pt(12.2, 25)}}}
	shorts := errorsOf(CheckBoard(b), "short")
	if len(shorts) != 1 {
		t.Fatalf("unnamed trace across pads must short, got %v", CheckBoard(b))
	}
	if len(shorts[0].Steps) < 5 {
		t.Fatalf("short must explain each segment, got %+v", shorts[0].Steps)
	}
}

func findStatus(ss []NetStatus, id string) NetStatus {
	for _, s := range ss {
		if s.NetID == id {
			return s
		}
	}
	return NetStatus{NetID: id, State: "missing"}
}
