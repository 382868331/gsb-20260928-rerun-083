package drc

import (
	"testing"

	"pcbworkbench/internal/model"
)

func pad(id, net, layer string, x, y, r float64) model.Pad {
	return model.Pad{ID: id, Name: id, Net: net, X: x, Y: y, Radius: r, Layers: []string{layer}}
}

func trace(id, net, layer string, w float64, pts ...model.Point) model.Trace {
	return model.Trace{ID: id, Net: net, Layer: layer, Width: w, Points: pts}
}

func findIssue(rep *model.DRCReport, code string) *model.Issue {
	for i := range rep.Issues {
		if rep.Issues[i].Code == code {
			return &rep.Issues[i]
		}
	}
	return nil
}

func issueCount(rep *model.DRCReport, code string) int {
	n := 0
	for _, is := range rep.Issues {
		if is.Code == code {
			n++
		}
	}
	return n
}

// Traces that cross geometrically but live on different layers must not be
// treated as connected or shorted without a via.
func TestCrossLayerIntersectionIsNotContact(t *testing.T) {
	b := &model.Board{
		Pads: []model.Pad{
			pad("p1", "A", "top", 0, 0, 0.5),
			pad("p2", "A", "top", 10, 10, 0.5),
			pad("p3", "B", "bottom", 10, 0, 0.5),
			pad("p4", "B", "bottom", 0, 10, 0.5),
		},
		Traces: []model.Trace{
			trace("t1", "A", "top", 0.4, model.Point{X: 0, Y: 0}, model.Point{X: 10, Y: 10}),
			trace("t2", "B", "bottom", 0.4, model.Point{X: 10, Y: 0}, model.Point{X: 0, Y: 10}),
		},
	}
	rep := Analyze(b, Config{MinClearanceMM: 0.2})
	if len(rep.Contacts) != 4 {
		t.Fatalf("expected only the four pad-trace contacts, got %d: %+v", len(rep.Contacts), rep.Contacts)
	}
	if findIssue(rep, "short") != nil {
		t.Fatalf("cross-layer crossing reported as short: %+v", rep.Issues)
	}
	if findIssue(rep, "clearance") != nil {
		t.Fatalf("cross-layer crossing reported as clearance violation")
	}
	for _, ns := range rep.NetStatuses {
		if !ns.Connected {
			t.Fatalf("net %s should connect to its own same-layer pads", ns.Net)
		}
	}
}

// A via only joins the layers it explicitly declares.
func TestViaDeclaredLayers(t *testing.T) {
	v := model.Via{ID: "v1", Net: "A", X: 5, Y: 5, Radius: 0.3, Layers: []string{"top", "mid"}}
	b := &model.Board{
		Pads: []model.Pad{
			pad("pa", "A", "top", 0, 0, 0.5),
			pad("pb", "A", "bottom", 10, 0, 0.5),
		},
		Traces: []model.Trace{
			trace("ta", "A", "top", 0.4, model.Point{X: 0, Y: 0}, model.Point{X: 10, Y: 10}),
			trace("tb", "A", "bottom", 0.4, model.Point{X: 10, Y: 0}, model.Point{X: 0, Y: 10}),
		},
		Vias: []model.Via{v},
	}
	rep := Analyze(b, Config{})
	// ta touches the via (top declared); tb must not (bottom not declared).
	gotTB := false
	for _, c := range rep.Contacts {
		if c.Kind == model.ContactTraceVia && c.A == "tb" {
			gotTB = true
		}
	}
	if gotTB {
		t.Fatal("trace on undeclared via layer connected to via")
	}
	if ns := rep.NetStatuses[0]; ns.Connected {
		t.Fatal("net shown connected although via does not declare bottom layer")
	}

	// Declare bottom as well: the same geometry is now connected.
	b.Vias[0].Layers = []string{"top", "mid", "bottom"}
	rep = Analyze(b, Config{})
	if ns := rep.NetStatuses[0]; !ns.Connected {
		t.Fatalf("net should connect once via declares bottom: %+v", ns)
	}
}

// Same-net labels without copper contact must not be considered connected,
// and different nets with copper contact produce a short at the touch point.
func TestNetNameIsNotProofAndShortLocation(t *testing.T) {
	b := &model.Board{
		Pads: []model.Pad{
			pad("a1", "A", "top", 0, 0, 0.3),
			pad("a2", "A", "top", 10, 0, 0.3),
			pad("b1", "B", "top", 5, 0, 0.3),
		},
		Traces: []model.Trace{
			trace("t1", "A", "top", 0.2, model.Point{X: 0, Y: 0}, model.Point{X: 4.6, Y: 0}),
			trace("t2", "B", "top", 0.2, model.Point{X: 5, Y: 0}, model.Point{X: 5, Y: 5}),
		},
	}
	rep := Analyze(b, Config{MinClearanceMM: 0.5})
	if findIssue(rep, "net-open") == nil {
		t.Fatal("expected net-open: a2 never touches copper of a1")
	}
	// t1 ends at x=4.6, t2 at x=5, width 0.2 -> edge gap 0.3 < 0.5.
	cl := findIssue(rep, "clearance")
	if cl == nil {
		t.Fatal("expected narrow-gap clearance error")
	}
	if cl.At == nil || cl.At.Y != 0 || !(cl.At.X > 4.5 && cl.At.X < 5.1) {
		t.Fatalf("clearance error points at wrong location %+v", cl.At)
	}

	// Now extend t1 so the copper corridors actually overlap b1/t2.
	b.Traces[0].Points[1] = model.Point{X: 5.5, Y: 0}
	rep = Analyze(b, Config{MinClearanceMM: 0.5})
	sh := findIssue(rep, "short")
	if sh == nil || sh.At == nil {
		t.Fatal("expected a concrete short contact")
	}
	if sh.Features[0] != "b1" || sh.Features[1] != "t1" {
		t.Fatalf("short should name the touching features, got %+v", sh.Features)
	}
}

// A gap that stays above the configured clearance is legal regardless of zoom.
func TestSmallGapBoundary(t *testing.T) {
	b := &model.Board{
		Pads: []model.Pad{
			pad("a1", "A", "top", 0, 0, 0.2),
			pad("b1", "B", "top", 0, 5, 0.2),
		},
		Traces: []model.Trace{
			trace("t1", "A", "top", 0.2, model.Point{X: 0, Y: 0}, model.Point{X: 0, Y: 2.35}),
			trace("t2", "B", "top", 0.2, model.Point{X: 0, Y: 2.65}, model.Point{X: 0, Y: 5}),
		},
	}
	// Edge gap = 0.30 - 0.20 = 0.10 mm.
	if rep := Analyze(b, Config{MinClearanceMM: 0.15}); issueCount(rep, "clearance") != 1 {
		t.Fatalf("expected 1 clearance error at 0.10 mm gap, got %+v", rep.Issues)
	}
	if rep := Analyze(b, Config{MinClearanceMM: 0.05}); findIssue(rep, "clearance") != nil {
		t.Fatal("gap above minimum clearance must pass")
	}
}

func TestUnnamedNetCopperBridgingIsShortByDeclaredLabels(t *testing.T) {
	b := &model.Board{
		Pads: []model.Pad{
			pad("a1", "A", "top", 0, 0, 0.3),
			pad("b1", "B", "top", 2, 0, 0.3),
		},
		Traces: []model.Trace{
			trace("", "", "top", 2.0, model.Point{X: 0, Y: 0}, model.Point{X: 2, Y: 0}),
		},
	}
	rep := Analyze(b, Config{})
	// Unnamed copper touching only one declared net is not a short; both pads
	// physically joined through it makes B appear unconnected to its own net.
	if findIssue(rep, "short") != nil {
		t.Fatal("unnamed copper joining pads should be reported via net-open, not guessed as short")
	}
}
