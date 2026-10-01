package main

import (
	"encoding/json"
	"math"
	"testing"
)

func baseBoard() *Board {
	return &Board{
		ID:      "b1",
		Outline: []Point{pt(0, 0), pt(50, 0), pt(50, 50), pt(0, 50)},
		Layers:  []string{"top", "bottom"},
		Nets: []Net{
			{ID: "n1", Name: "A"},
			{ID: "n2", Name: "B"},
		},
	}
}

func errorsOf(errs []CheckError, typ string) []CheckError {
	var out []CheckError
	for _, e := range errs {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// Crossing traces on different layers must NOT connect, even with the
// same net name: connectivity comes from geometry, not names.
func TestCrossLayerCrossingNotConnected(t *testing.T) {
	b := baseBoard()
	b.Traces = []Trace{
		{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(45, 25)}},
		{ID: "t2", NetID: "n1", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(25, 5), pt(25, 45)}},
	}
	errs := CheckBoard(b)
	opens := errorsOf(errs, "open")
	if len(opens) != 1 {
		t.Fatalf("expected 1 open (same net, split across layers), got %v", errs)
	}
	if shorts := errorsOf(errs, "short"); len(shorts) != 0 {
		t.Fatalf("cross-layer crossing must not short, got %v", shorts)
	}
	// Different nets crossing on different layers: no short either.
	b.Traces[1].NetID = "n2"
	if errs := CheckBoard(b); len(errs) != 0 {
		t.Fatalf("different nets on different layers must be clean, got %v", errs)
	}
}

// A via connects exactly its declared layers.
func TestViaConnectsDeclaredLayers(t *testing.T) {
	mk := func(viaLayers []string) *Board {
		b := baseBoard()
		b.Traces = []Trace{
			{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
				Points: []Point{pt(5, 25), pt(25, 25)}},
			{ID: "t2", NetID: "n1", Layer: "bottom", Width: 500_000,
				Points: []Point{pt(25, 25), pt(45, 25)}},
		}
		b.Vias = []Via{{ID: "v1", NetID: "n1", X: 25 * mm, Y: 25 * mm,
			Diameter: 1_000_000, Layers: viaLayers}}
		return b
	}
	if errs := CheckBoard(mk([]string{"top", "bottom"})); len(errs) != 0 {
		t.Fatalf("via on both layers should connect, got %v", errs)
	}
	errs := CheckBoard(mk([]string{"top"}))
	if opens := errorsOf(errs, "open"); len(opens) != 1 {
		t.Fatalf("via declared only on top must not reach bottom, got %v", errs)
	}
}

// A 0.1 mm gap must be flagged as an open, located at the gap.
func TestTinyGapDetected(t *testing.T) {
	b := baseBoard()
	b.Pads = []Pad{{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
		X: 29_050_000, Y: 25 * mm, Diameter: 1_500_000}}
	b.Traces = []Trace{{ID: "t1", NetID: "n1", Layer: "top", Width: 400_000,
		Points: []Point{pt(10, 25), pt(28, 25)}}}
	errs := CheckBoard(b)
	opens := errorsOf(errs, "open")
	if len(opens) != 1 {
		t.Fatalf("expected open for 0.1mm gap, got %v", errs)
	}
	o := opens[0]
	// Gap is between x=28.2mm (trace edge) and x=28.3mm (pad edge).
	if o.X < 28.19*mm || o.X > 28.31*mm || o.X2 < 28.19*mm || o.X2 > 28.31*mm {
		t.Fatalf("open location should be at the gap, got (%v,%v)-(%v,%v)", o.X, o.Y, o.X2, o.Y2)
	}
	// Close the gap: extend trace to 28.5mm -> connected.
	b.Traces[0].Points[1] = pt(28.5, 25)
	if errs := CheckBoard(b); len(errs) != 0 {
		t.Fatalf("closed gap should be clean, got %v", errs)
	}
}

// A same-layer crossing of different nets is a short at the crossing point.
func TestShortReportsContactLocation(t *testing.T) {
	b := baseBoard()
	b.Traces = []Trace{
		{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(45, 25)}},
		{ID: "t2", NetID: "n2", Layer: "top", Width: 500_000,
			Points: []Point{pt(25, 5), pt(25, 45)}},
	}
	errs := CheckBoard(b)
	shorts := errorsOf(errs, "short")
	if len(shorts) != 1 {
		t.Fatalf("expected 1 short, got %v", errs)
	}
	s := shorts[0]
	if math.Abs(s.X-25*mm) > 1000 || math.Abs(s.Y-25*mm) > 1000 {
		t.Fatalf("short should point at crossing (25,25)mm, got (%.3f,%.3f)mm",
			s.X/1e6, s.Y/1e6)
	}
	if len(s.Elements) != 2 || len(s.Nets) != 2 {
		t.Fatalf("short should name elements and nets: %+v", s)
	}
}

// Same-net junction (T-connection) on one layer is connected.
func TestSameLayerJunctionConnects(t *testing.T) {
	b := baseBoard()
	b.Traces = []Trace{
		{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(45, 25)}},
		{ID: "t2", NetID: "n1", Layer: "top", Width: 500_000,
			Points: []Point{pt(25, 25), pt(25, 45)}},
	}
	if errs := CheckBoard(b); len(errs) != 0 {
		t.Fatalf("T junction should connect, got %v", errs)
	}
}

// Copper with no net assigned still conducts: an un-netted trace
// touching pads of two different nets is a short, and the report must
// name every object on the connecting path.
func TestShortThroughUnnettedTrace(t *testing.T) {
	b := baseBoard()
	b.Pads = []Pad{
		{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
			X: 10 * mm, Y: 25 * mm, Diameter: 2 * mm},
		{ID: "p2", NetID: "n2", Layer: "top", Shape: "circle",
			X: 40 * mm, Y: 25 * mm, Diameter: 2 * mm},
	}
	// Multi-segment trace with NO net, bridging the two pads.
	b.Traces = []Trace{{ID: "t-bridge", NetID: "", Layer: "top", Width: 500_000,
		Points: []Point{pt(10, 25), pt(25, 25), pt(25, 30), pt(40, 30), pt(40, 25)}}}
	errs := CheckBoard(b)
	shorts := errorsOf(errs, "short")
	if len(shorts) != 1 {
		t.Fatalf("un-netted trace bridging two nets must be a short, got %v", errs)
	}
	s := shorts[0]
	if len(s.Nets) != 2 {
		t.Fatalf("short should name both nets, got %+v", s)
	}
	joined := s.Elements
	has := func(id string) bool {
		for _, e := range joined {
			if e == id {
				return true
			}
		}
		return false
	}
	if !has("p1") || !has("p2") || !has("t-bridge") {
		t.Fatalf("short path should include both pads and the bridge trace, got %v", joined)
	}
	// Location must sit on the bridge, between the pads.
	if s.X < 9*mm || s.X > 41*mm || s.Y < 24*mm || s.Y > 31*mm {
		t.Fatalf("short location should be on the connecting path, got (%.3f,%.3f)mm",
			s.X/1e6, s.Y/1e6)
	}
	// Assigning the bridge to either net keeps the short (now direct).
	b.Traces[0].NetID = "n1"
	if shorts := errorsOf(CheckBoard(b), "short"); len(shorts) != 1 {
		t.Fatalf("bridge on n1 still shorts to n2, got %v", CheckBoard(b))
	}
	// Assigning both sides the same net clears the short.
	b.Pads[1].NetID = "n1"
	b.Traces[0].NetID = "n1"
	if errs := CheckBoard(b); len(errs) != 0 {
		t.Fatalf("single net should be clean, got %v", errs)
	}
}

// A short path may span layers: pad(n1) - top trace - via - bottom
// trace - pad(n2). The via and traces have no net of their own; the
// short must still be reported with the full cross-layer path.
func TestShortAcrossLayersThroughVia(t *testing.T) {
	b := baseBoard()
	b.Pads = []Pad{
		{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
			X: 5 * mm, Y: 25 * mm, Diameter: 2 * mm},
		{ID: "p2", NetID: "n2", Layer: "bottom", Shape: "circle",
			X: 45 * mm, Y: 25 * mm, Diameter: 2 * mm},
	}
	b.Traces = []Trace{
		{ID: "t-top", NetID: "", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(25, 25)}},
		{ID: "t-bot", NetID: "", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(25, 25), pt(45, 25)}},
	}
	b.Vias = []Via{{ID: "v1", NetID: "", X: 25 * mm, Y: 25 * mm,
		Diameter: 1_200_000, Layers: []string{"top", "bottom"}}}
	errs := CheckBoard(b)
	shorts := errorsOf(errs, "short")
	if len(shorts) != 1 {
		t.Fatalf("cross-layer bridge through via must be a short, got %v", errs)
	}
	s := shorts[0]
	for _, id := range []string{"p1", "p2", "v1"} {
		found := false
		for _, e := range s.Elements {
			if e == id {
				found = true
			}
		}
		if !found {
			t.Fatalf("short path should include %s, got %v", id, s.Elements)
		}
	}
	// Via declared only on top: bottom trace dangling, no short.
	b.Vias[0].Layers = []string{"top"}
	if shorts := errorsOf(CheckBoard(b), "short"); len(shorts) != 0 {
		t.Fatalf("via not on bottom must break the bridge, got %v", shorts)
	}
}

// Through-hole pad exists on every layer.
func TestThroughPadSpansLayers(t *testing.T) {
	b := baseBoard()
	b.Pads = []Pad{{ID: "p1", NetID: "n1", Layer: "through", Shape: "circle",
		X: 25 * mm, Y: 25 * mm, Diameter: 2 * mm, Drill: mm}}
	b.Traces = []Trace{
		{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(25, 25)}},
		{ID: "t2", NetID: "n1", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(25, 25), pt(45, 25)}},
	}
	if errs := CheckBoard(b); len(errs) != 0 {
		t.Fatalf("through pad should bridge layers, got %v", errs)
	}
}

// Export -> import keeps every element identity intact.
func TestExportImportPreservesIdentity(t *testing.T) {
	b := demoBoard()
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var back Board
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	ids := func(b *Board) map[string]bool {
		s := map[string]bool{b.ID: true}
		for _, n := range b.Nets {
			s[n.ID] = true
		}
		for _, p := range b.Pads {
			s[p.ID] = true
		}
		for _, tr := range b.Traces {
			s[tr.ID] = true
		}
		for _, v := range b.Vias {
			s[v.ID] = true
		}
		return s
	}
	a, c := ids(b), ids(&back)
	if len(a) != len(c) {
		t.Fatalf("id count changed: %d -> %d", len(a), len(c))
	}
	for id := range a {
		if !c[id] {
			t.Fatalf("lost id %q after round trip", id)
		}
	}
	// And the re-opened board produces identical check results.
	e1, e2 := CheckBoard(b), CheckBoard(&back)
	if len(e1) != len(e2) {
		t.Fatalf("check results differ after round trip: %d vs %d", len(e1), len(e2))
	}
}

// The shipped demo board must exhibit exactly the documented issues:
// the 0.1mm gap open, the cross-layer crossing reported as NOT connected
// (an open for net SIG, not a short), and no shorts at all.
func TestDemoBoardValidation(t *testing.T) {
	errs := CheckBoard(demoBoard())
	opens := errorsOf(errs, "open")
	if len(opens) != 2 {
		t.Fatalf("demo board should have exactly 2 opens, got %v", errs)
	}
	var gapOpen, crossOpen *CheckError
	for i := range opens {
		for _, el := range opens[i].Elements {
			if el == "pad-gnd-gap" {
				gapOpen = &opens[i]
			}
			if el == "trace-sig-cross" {
				crossOpen = &opens[i]
			}
		}
	}
	if gapOpen == nil {
		t.Fatalf("demo board must report the tiny gap open, got %v", opens)
	}
	if crossOpen == nil {
		t.Fatalf("demo board must report the cross-layer split, got %v", opens)
	}
	if shorts := errorsOf(errs, "short"); len(shorts) != 0 {
		t.Fatalf("demo board should have no shorts, got %v", shorts)
	}
}

// Zoom independence: checks run purely in board coordinates. Scaling the
// whole board geometry (what a view zoom would visually do) must not
// change the open/short verdict structure.
func TestZoomIndependence(t *testing.T) {
	b := demoBoard()
	before := CheckBoard(b)
	for k := 1; k <= 3; k++ {
		var scaled Board
		data, _ := json.Marshal(b)
		json.Unmarshal(data, &scaled)
		f := int64(1 << k)
		scalePt := func(p Point) Point { return Point{p.X * f, p.Y * f} }
		for i := range scaled.Outline {
			scaled.Outline[i] = scalePt(scaled.Outline[i])
		}
		for i := range scaled.Pads {
			p := &scaled.Pads[i]
			p.X, p.Y = p.X*f, p.Y*f
			p.Diameter, p.W, p.H, p.Drill = p.Diameter*f, p.W*f, p.H*f, p.Drill*f
		}
		for i := range scaled.Traces {
			tr := &scaled.Traces[i]
			tr.Width *= f
			for j := range tr.Points {
				tr.Points[j] = scalePt(tr.Points[j])
			}
		}
		for i := range scaled.Vias {
			v := &scaled.Vias[i]
			v.X, v.Y, v.Diameter, v.Drill = v.X*f, v.Y*f, v.Diameter*f, v.Drill*f
		}
		after := CheckBoard(&scaled)
		if len(after) != len(before) {
			t.Fatalf("scale %dx changed verdict count: %d -> %d", f, len(before), len(after))
		}
		for j := range before {
			if after[j].Type != before[j].Type {
				t.Fatalf("scale %dx changed error type at %d", f, j)
			}
		}
	}
}

// A short through multiple vias and un-netted copper must be
// explainable segment by segment: every hop names both elements, the
// layer, and the contact location, never just two net names.
func TestShortPathExplainedSegmentBySegment(t *testing.T) {
	b := baseBoard()
	b.Pads = []Pad{
		{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle",
			X: 5 * mm, Y: 25 * mm, Diameter: 2 * mm},
		{ID: "p2", NetID: "n2", Layer: "top", Shape: "circle",
			X: 45 * mm, Y: 25 * mm, Diameter: 2 * mm},
	}
	b.Traces = []Trace{
		{ID: "t1", NetID: "", Layer: "top", Width: 500_000,
			Points: []Point{pt(5, 25), pt(15, 25)}},
		{ID: "t2", NetID: "", Layer: "bottom", Width: 500_000,
			Points: []Point{pt(15, 25), pt(35, 25)}},
		{ID: "t3", NetID: "", Layer: "top", Width: 500_000,
			Points: []Point{pt(35, 25), pt(45, 25)}},
	}
	b.Vias = []Via{
		{ID: "v1", NetID: "", X: 15 * mm, Y: 25 * mm,
			Diameter: 1_200_000, Layers: []string{"top", "bottom"}},
		{ID: "v2", NetID: "", X: 35 * mm, Y: 25 * mm,
			Diameter: 1_200_000, Layers: []string{"top", "bottom"}},
	}
	res := Analyze(b)
	var shorts []CheckError
	for _, e := range res.Errors {
		if e.Type == "short" {
			shorts = append(shorts, e)
		}
	}
	if len(shorts) != 1 {
		t.Fatalf("expected 1 short, got %v", res.Errors)
	}
	s := shorts[0]
	if len(s.Segments) != len(s.Elements)-1 || len(s.Segments) < 4 {
		t.Fatalf("path of %d elements needs segments, got %+v", len(s.Elements), s.Segments)
	}
	for i, seg := range s.Segments {
		if seg.From != s.Elements[i] || seg.To != s.Elements[i+1] {
			t.Fatalf("segment %d endpoints mismatch: %+v vs %v", i, seg, s.Elements)
		}
		if seg.Layer == "" || seg.Text == "" {
			t.Fatalf("segment %d must name layer and explain itself: %+v", i, seg)
		}
		if seg.X < 4*mm || seg.X > 46*mm {
			t.Fatalf("segment %d contact off board: %+v", i, seg)
		}
	}
	b.Nets = append(b.Nets, Net{ID: "n3", Name: "C"})
	res = Analyze(b)
	status := map[string]string{}
	for _, n := range res.Nets {
		status[n.ID] = n.Status
	}
	if status["n1"] != "short" || status["n2"] != "short" || status["n3"] != "connected" {
		t.Fatalf("net statuses wrong: %+v", res.Nets)
	}
}

// Moving or deleting a via must immediately change the per-net verdict:
// nets that were connected through the via become open, and nets that
// never depended on it stay connected.
func TestNetStatusAfterViaMoveAndDelete(t *testing.T) {
	mk := func() *Board {
		b := baseBoard()
		b.Traces = []Trace{
			{ID: "t1", NetID: "n1", Layer: "top", Width: 500_000,
				Points: []Point{pt(5, 25), pt(25, 25)}},
			{ID: "t2", NetID: "n1", Layer: "bottom", Width: 500_000,
				Points: []Point{pt(25, 25), pt(45, 25)}},
			{ID: "t3", NetID: "n2", Layer: "top", Width: 500_000,
				Points: []Point{pt(5, 40), pt(45, 40)}},
		}
		b.Vias = []Via{{ID: "v1", NetID: "n1", X: 25 * mm, Y: 25 * mm,
			Diameter: 1_000_000, Layers: []string{"top", "bottom"}}}
		return b
	}
	statusOf := func(b *Board) map[string]string {
		m := map[string]string{}
		for _, n := range Analyze(b).Nets {
			m[n.ID] = n.Status
		}
		return m
	}
	b := mk()
	if s := statusOf(b); s["n1"] != "connected" || s["n2"] != "connected" {
		t.Fatalf("via present: both nets connected, got %v", s)
	}
	b = mk()
	b.Vias[0].X = 10 * mm
	b.Vias[0].Y = 10 * mm
	if s := statusOf(b); s["n1"] != "open" || s["n2"] != "connected" {
		t.Fatalf("via moved: n1 open, n2 connected, got %v", s)
	}
	b = mk()
	b.Vias = nil
	if s := statusOf(b); s["n1"] != "open" || s["n2"] != "connected" {
		t.Fatalf("via deleted: n1 open, n2 connected, got %v", s)
	}
	b = mk()
	if b.Vias[0].X != 25*mm || b.Vias[0].Y != 25*mm {
		t.Fatalf("via coordinates not restored: %+v", b.Vias[0])
	}
	if s := statusOf(b); s["n1"] != "connected" {
		t.Fatalf("via restored: n1 connected, got %v", s)
	}
}
