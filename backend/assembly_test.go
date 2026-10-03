package main

import (
	"strings"
	"testing"
)

func asmState(t *testing.T, b *Board, asmID, netID string) string {
	t.Helper()
	for _, ns := range AnalyzeAssembly(b, asmID).NetStatus {
		if ns.NetID == netID {
			return ns.State
		}
	}
	t.Fatalf("net %s missing in assembly %q", netID, asmID)
	return ""
}

// The optional jumper conducts only in variants that mount it: PWR is
// connected on the base board and in the full assembly, and open only in
// the no-jumper variant. Copper/via geometry is shared by all variants.
func TestAssemblyJumperChangesConnectivity(t *testing.T) {
	b := demoBoard()
	if got := asmState(t, b, "", "net-pwr"); got != "connected" {
		t.Fatalf("base board PWR = %s, want connected", got)
	}
	if got := asmState(t, b, "asm-full", "net-pwr"); got != "connected" {
		t.Fatalf("full assembly PWR = %s, want connected", got)
	}
	if got := asmState(t, b, "asm-no-jmp", "net-pwr"); got != "open" {
		t.Fatalf("no-jumper assembly PWR = %s, want open", got)
	}
	if got := asmState(t, b, "asm-no-jmp", "net-vcc"); got != "connected" {
		t.Fatalf("no-jumper assembly VCC = %s, want connected", got)
	}
}

// A variant that leaves a via unmounted must see the cross-layer link
// break, while the base board keeps it.
func TestAssemblyOmitsVia(t *testing.T) {
	b := demoBoard()
	b.Assemblies = append(b.Assemblies,
		Assembly{ID: "asm-no-via", Name: "不装过孔", Exclude: []string{"via-sig"}})
	base := len(AnalyzeAssembly(b, "").Errors)
	novia := len(AnalyzeAssembly(b, "asm-no-via").Errors)
	if novia <= base {
		t.Fatalf("omitting via-sig should add opens: base %d, no-via %d", base, novia)
	}
}

// Compare reports nets that are open/short only in some variants, and
// every variant report stays independent.
func TestCompareAssembliesDiff(t *testing.T) {
	b := demoBoard()
	cr := CompareAssemblies(b)
	if len(cr.Reports) != 3 {
		t.Fatalf("expected 3 variant reports, got %d", len(cr.Reports))
	}
	var pwr *VariantDiff
	for i := range cr.Diffs {
		if cr.Diffs[i].NetID == "net-pwr" {
			pwr = &cr.Diffs[i]
		}
	}
	if pwr == nil {
		t.Fatalf("PWR diff missing, got %+v", cr.Diffs)
	}
	if pwr.Kind != "open" || len(pwr.OnlyIn) != 1 || pwr.OnlyIn[0] != "无跳线方案" {
		t.Fatalf("PWR diff = %+v, want open only in 无跳线方案", pwr)
	}
	if len(pwr.States) != 3 {
		t.Fatalf("PWR diff states = %+v", pwr.States)
	}
	found := false
	for _, e := range cr.Reports["asm-no-jmp"].Errors {
		if e.Type == "open" && len(e.Nets) == 1 && e.Nets[0] == "net-pwr" {
			found = true
			if len(e.Steps) == 0 {
				t.Fatalf("PWR open in no-jumper variant has no path steps")
			}
			for _, st := range e.Steps {
				if st.ID == "trace-pwr-jmp" {
					t.Fatalf("unmounted jumper leaked into variant path: %+v", e.Steps)
				}
			}
		}
	}
	if !found {
		t.Fatalf("no-jumper variant must report the PWR open")
	}
	for _, e := range cr.Reports[""].Errors {
		if len(e.Nets) == 1 && e.Nets[0] == "net-pwr" {
			t.Fatalf("base board wrongly reports PWR fault: %v", e.Message)
		}
	}
}

// A short that exists only while a jumper is mounted shows up as a
// short-only diff, and the short path names the mounted jumper.
func TestCompareShortOnlyInOneVariant(t *testing.T) {
	b := demoBoard()
	b.Traces = append(b.Traces, Trace{
		ID: "trace-jmp-short", NetID: "", Layer: "top", Width: 500000,
		Points: []Point{pt(10, 25.75), pt(10, 33.9)},
	})
	b.Assemblies = append(b.Assemblies,
		Assembly{ID: "asm-no-stub", Name: "不装短接跳线", Exclude: []string{"trace-jmp-short"}})
	cr := CompareAssemblies(b)
	var gnd *VariantDiff
	for i := range cr.Diffs {
		if cr.Diffs[i].NetID == "net-gnd" && cr.Diffs[i].Kind == "short" {
			gnd = &cr.Diffs[i]
		}
	}
	if gnd == nil {
		t.Fatalf("expected short-only diff for GND, got %+v", cr.Diffs)
	}
	if len(gnd.OnlyIn) != 3 {
		t.Fatalf("GND short should appear wherever the stub is mounted, got %+v", gnd.OnlyIn)
	}
	found := false
	for _, e := range cr.Reports[""].Errors {
		if e.Type == "short" {
			for _, el := range e.Elements {
				if el == "trace-jmp-short" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Fatalf("base short path must include the mounted jumper")
	}
	for _, e := range cr.Reports["asm-no-stub"].Errors {
		if e.Type == "short" {
			t.Fatalf("no-stub variant wrongly inherits short: %v", e.Message)
		}
	}
}

// Partially missing board data keeps everything valid and reports each
// skipped or repaired element with its location.
func TestNormalizeKeepsValidContent(t *testing.T) {
	b := demoBoard()
	b.Traces = append(b.Traces,
		Trace{ID: "trace-empty", NetID: "net-sig", Layer: "top", Width: 500000},
		Trace{ID: "trace-nowidth", NetID: "net-sig", Points: []Point{pt(1, 1), pt(2, 2)}},
	)
	b.Pads = append(b.Pads, Pad{ID: "pad-bad", NetID: "net-gnd", Layer: "top",
		Shape: "circle", X: 5 * mm, Y: 5 * mm, Diameter: 0})
	b.Vias = append(b.Vias, Via{ID: "via-nolayers", NetID: "net-sig",
		X: 20 * mm, Y: 20 * mm, Diameter: 1 * mm})
	b.Assemblies[1].Exclude = append(b.Assemblies[1].Exclude, "ghost-elem")
	warns := normalizeBoard(b)
	joined := strings.Join(warns, "\n")
	for _, want := range []string{"trace-empty", "pad-bad", "via-nolayers", "ghost-elem", "trace-nowidth"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("warnings should mention %s, got:\n%s", want, joined)
		}
	}
	if !strings.Contains(joined, "mm") {
		t.Fatalf("warnings should locate repairs on the board, got:\n%s", joined)
	}
	if len(b.Pads) != 7 || len(b.Traces) != 7 || len(b.Vias) != 2 {
		t.Fatalf("valid content lost: pads %d traces %d vias %d",
			len(b.Pads), len(b.Traces), len(b.Vias))
	}
	if len(b.Assemblies[1].Exclude) != 1 || b.Assemblies[1].Exclude[0] != "trace-pwr-jmp" {
		t.Fatalf("assembly exclusions not cleaned: %+v", b.Assemblies[1].Exclude)
	}
	_ = AnalyzeBoard(b)
}

// Nil slices normalize to empty so JSON serves [] and rendering/checks
// never trip over missing sections.
func TestNormalizeNilSlices(t *testing.T) {
	b := &Board{ID: "x"}
	warns := normalizeBoard(b)
	if b.Pads == nil || b.Traces == nil || b.Vias == nil || b.Nets == nil ||
		b.Outline == nil || b.Assemblies == nil {
		t.Fatalf("nil slices survived normalization")
	}
	if len(b.Layers) == 0 {
		t.Fatalf("layers should default")
	}
	if len(warns) == 0 {
		t.Fatalf("missing layers should be reported")
	}
}
