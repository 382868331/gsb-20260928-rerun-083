package main

import "testing"

// variantBoard: net-a pads joined only when the jumper is installed;
// net-b reachable only through a pad that variant v2 depopulates.
func variantBoard() *Board {
	return &Board{
		ID: "b-var", Name: "variant test", Revision: 7,
		Outline: []Point{pt(0, 0), pt(40, 0), pt(40, 30), pt(0, 30)},
		Layers:  []string{"top", "bottom"},
		Nets: []Net{
			{ID: "net-a", Name: "A"},
			{ID: "net-b", Name: "B"},
		},
		Pads: []Pad{
			{ID: "pa1", NetID: "net-a", Layer: "top", Shape: "circle", X: 5 * mm, Y: 5 * mm, Diameter: 2 * mm},
			{ID: "pa2", NetID: "net-a", Layer: "top", Shape: "circle", X: 20 * mm, Y: 5 * mm, Diameter: 2 * mm},
			{ID: "pb1", NetID: "net-b", Layer: "through", Shape: "circle", X: 5 * mm, Y: 20 * mm, Diameter: 2 * mm},
		},
		Traces: []Trace{
			{ID: "ta1", NetID: "net-a", Layer: "top", Width: 500_000,
				Points: []Point{pt(5, 5), pt(10, 5)}},
			{ID: "ta2", NetID: "net-a", Layer: "top", Width: 500_000,
				Points: []Point{pt(15, 5), pt(20, 5)}},
		},
		Jumpers: []Jumper{
			{ID: "j1", NetID: "net-a", Layer: "top", Width: 500_000,
				Points: []Point{pt(10, 5), pt(15, 5)}},
		},
		Variants: []Variant{
			{ID: "v1", Name: "带跳线", Jumpers: []string{"j1"}},
			{ID: "v2", Name: "免器件", DNP: []string{"pb1"}},
		},
	}
}

func netState(r Report, id string) string {
	for _, ns := range r.NetStatus {
		if ns.NetID == id {
			return ns.State
		}
	}
	return ""
}

func TestJumperInstalledOnlyInItsVariant(t *testing.T) {
	b := variantBoard()
	base := AnalyzeVariant(b, "")
	if got := netState(base, "net-a"); got != "open" {
		t.Fatalf("base: net-a = %s, want open (jumper not installed)", got)
	}
	withJ := AnalyzeVariant(b, "v1")
	if got := netState(withJ, "net-a"); got != "connected" {
		t.Fatalf("v1: net-a = %s, want connected (jumper installed)", got)
	}
	// Re-checking the base after the jumper variant must not reuse the
	// jumper-variant conclusion.
	again := AnalyzeVariant(b, "")
	if got := netState(again, "net-a"); got != "open" {
		t.Fatalf("base re-check: net-a = %s, want open (no cross-variant reuse)", got)
	}
}

func TestDNPDeviceOpenOnlyInItsVariant(t *testing.T) {
	b := variantBoard()
	full := AnalyzeVariant(b, "")
	if got := netState(full, "net-b"); got != "connected" {
		t.Fatalf("base: net-b = %s, want connected", got)
	}
	dnp := AnalyzeVariant(b, "v2")
	if got := netState(dnp, "net-b"); got != "open" {
		t.Fatalf("v2: net-b = %s, want open (only device depopulated)", got)
	}
	if got := netState(dnp, "net-a"); got != "open" {
		t.Fatalf("v2: net-a = %s, want open (jumper still not installed)", got)
	}
}

func TestVariantAnalysisKeepsSharedGeometry(t *testing.T) {
	b := variantBoard()
	tracesBefore := len(b.Traces)
	padsBefore := len(b.Pads)
	AnalyzeVariant(b, "v1")
	AnalyzeVariant(b, "v2")
	if len(b.Traces) != tracesBefore || len(b.Pads) != padsBefore {
		t.Fatalf("shared geometry mutated: traces %d->%d pads %d->%d",
			tracesBefore, len(b.Traces), padsBefore, len(b.Pads))
	}
}

func TestCompareVariantsDiff(t *testing.T) {
	b := variantBoard()
	rep := CompareVariants(b)
	if len(rep.Variants) != 3 { // base + v1 + v2
		t.Fatalf("variants = %d, want 3", len(rep.Variants))
	}
	if rep.Revision != 7 {
		t.Fatalf("revision = %d, want 7", rep.Revision)
	}
	diff := map[string]NetDiff{}
	for _, d := range rep.Diff {
		diff[d.NetID] = d
	}
	da, ok := diff["net-a"]
	if !ok {
		t.Fatal("net-a missing from diff (open in base/v2, connected in v1)")
	}
	if len(da.OnlyOpen) != 2 || da.States["v1"] != "connected" {
		t.Fatalf("net-a diff wrong: %+v", da)
	}
	db, ok := diff["net-b"]
	if !ok {
		t.Fatal("net-b missing from diff (open only in v2)")
	}
	if len(db.OnlyOpen) != 1 || db.OnlyOpen[0] != "v2" {
		t.Fatalf("net-b diff wrong: %+v", db)
	}
}

func TestVariantFaultPathNamesJumper(t *testing.T) {
	// A jumper shorting two nets must appear in the located fault path.
	b := &Board{
		ID: "b-short", Layers: []string{"top"},
		Nets: []Net{{ID: "n1", Name: "N1"}, {ID: "n2", Name: "N2"}},
		Pads: []Pad{
			{ID: "p1", NetID: "n1", Layer: "top", Shape: "circle", X: 5 * mm, Y: 5 * mm, Diameter: 2 * mm},
			{ID: "p2", NetID: "n2", Layer: "top", Shape: "circle", X: 12 * mm, Y: 5 * mm, Diameter: 2 * mm},
		},
		Jumpers: []Jumper{
			{ID: "j-short", Layer: "top", Width: 500_000, Points: []Point{pt(5, 5), pt(12, 5)}},
		},
		Variants: []Variant{{ID: "vj", Name: "跳线", Jumpers: []string{"j-short"}}},
	}
	rep := AnalyzeVariant(b, "vj")
	if len(rep.Errors) == 0 {
		t.Fatal("expected a short error in variant vj")
	}
	found := false
	for _, st := range rep.Errors[0].Steps {
		if st.ID == "j-short" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fault path should walk through jumper j-short: %+v", rep.Errors[0].Steps)
	}
	// Base assembly has no jumper: no short.
	if errs := AnalyzeVariant(b, "").Errors; len(errs) != 0 {
		t.Fatalf("base should be clean, got %+v", errs)
	}
}

func TestSanitizeBoardFillsMissingSections(t *testing.T) {
	b := &Board{ID: "partial"}
	sanitizeBoard(b)
	if len(b.Layers) != 2 || b.Pads == nil || b.Traces == nil || b.Jumpers == nil || b.Variants == nil {
		t.Fatalf("sanitize should fill defaults: %+v", b)
	}
}
