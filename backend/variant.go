package main

import "sort"

// variantOf finds a variant by id; nil means the base assembly
// (every pad populated, no optional jumpers installed).
func variantOf(b *Board, id string) *Variant {
	for i := range b.Variants {
		if b.Variants[i].ID == id {
			return &b.Variants[i]
		}
	}
	return nil
}

// boardForVariant derives the conductive view of one assembly variant.
// Traces and vias are shared verbatim (same geometry, same IDs); DNP
// pads are removed and only the variant's installed jumpers appear, as
// copper on their declared layer. The input board is never mutated, so
// analyzing one variant cannot leak state into another.
func boardForVariant(b *Board, v *Variant) *Board {
	out := *b // shallow copy: outline/layers/nets/traces/vias shared read-only
	if v == nil {
		out.Jumpers = nil
		return &out
	}
	dnp := map[string]bool{}
	for _, id := range v.DNP {
		dnp[id] = true
	}
	pads := make([]Pad, 0, len(b.Pads))
	for _, p := range b.Pads {
		if !dnp[p.ID] {
			pads = append(pads, p)
		}
	}
	out.Pads = pads
	installed := map[string]bool{}
	for _, id := range v.Jumpers {
		installed[id] = true
	}
	traces := make([]Trace, len(b.Traces), len(b.Traces)+len(v.Jumpers))
	copy(traces, b.Traces)
	for _, j := range b.Jumpers {
		if !installed[j.ID] {
			continue
		}
		// An installed jumper conducts like a wire on its layer; keep
		// its own ID so fault paths name the jumper explicitly.
		traces = append(traces, Trace{
			ID: j.ID, NetID: j.NetID, Layer: j.Layer,
			Width: j.Width, Points: j.Points,
		})
	}
	out.Traces = traces
	out.Jumpers = nil
	return &out
}

// AnalyzeVariant runs the geometric connectivity check for one variant.
// variantID "" is the base assembly. Each call rebuilds the analysis
// from the posted board, so results always match that board revision
// and that variant's own component selection.
func AnalyzeVariant(b *Board, variantID string) Report {
	return AnalyzeBoard(boardForVariant(b, variantOf(b, variantID)))
}

// Selection snapshots the assembly choices a variant report was
// computed from, so the client can tell later whether the user changed
// jumper/DNP selections after the report was frozen.
type Selection struct {
	DNP     []string `json:"dnp"`
	Jumpers []string `json:"jumpers"`
}

// VariantReport is one variant's verdict, stamped with the board
// revision it was computed from.
type VariantReport struct {
	VariantID string       `json:"variantId"`
	Name      string       `json:"name"`
	Revision  int          `json:"revision"`
	Status    string       `json:"status"` // "ok" | "empty" | "missing"
	Notes     []string     `json:"notes"`
	Selection Selection    `json:"selection"`
	Errors    []CheckError `json:"errors"`
	NetStatus []NetStatus  `json:"netStatus"`
}

// NetDiff lists a net whose connectivity state differs between assembly
// variants (e.g. shorted only when a jumper is installed, open only
// when a device is depopulated).
type NetDiff struct {
	NetID     string            `json:"netId"`
	NetName   string            `json:"netName"`
	States    map[string]string `json:"states"`
	OnlyOpen  []string          `json:"onlyOpen"`
	OnlyShort []string          `json:"onlyShort"`
	// Paths explains, per variant where this net conducts while others
	// lose it, the exact copper/via/device route that carries it.
	Paths map[string][]PathStep `json:"paths"`
}

// CompareReport holds every variant's independent result plus the
// cross-variant net diff.
type CompareReport struct {
	Revision int             `json:"revision"`
	Variants []VariantReport `json:"variants"`
	Diff     []NetDiff       `json:"diff"`
}

// CompareVariants analyzes the base assembly plus every declared
// variant against the same shared copper geometry and reports which
// nets are shorted or open only under specific assembly options.
func CompareVariants(b *Board) CompareReport {
	type slot struct {
		id   string
		name string
	}
	slots := []slot{{"", "基准 (全部装配, 无跳线)"}}
	for _, v := range b.Variants {
		name := v.Name
		if name == "" {
			name = v.ID
		}
		slots = append(slots, slot{v.ID, name})
	}
	rep := CompareReport{Revision: b.Revision, Variants: []VariantReport{}, Diff: []NetDiff{}}
	stateOf := map[string]map[string]string{}
	nameOf := map[string]string{}
	analyses := map[string]*analysis{}
	derived := map[string]*Board{}
	for _, s := range slots {
		v := variantOf(b, s.id)
		vr := VariantReport{
			VariantID: s.id, Name: s.name, Revision: b.Revision,
			Status: "ok", Notes: []string{}, Selection: selectionOf(v),
			Errors: []CheckError{}, NetStatus: []NetStatus{},
		}
		// A variant referencing pads/jumpers the board no longer has is
		// incomplete data: say so, but still analyze what exists so one
		// broken variant never blocks the others or the board itself.
		if missing := missingRefs(b, v); len(missing) > 0 {
			vr.Status = "missing"
			vr.Notes = append(vr.Notes, missing...)
		}
		vb := boardForVariant(b, v)
		if len(vb.Pads)+len(vb.Traces)+len(vb.Vias) == 0 {
			vr.Status = "empty"
			vr.Notes = append(vr.Notes, "该方案没有任何可用导电路径 (器件/铜线/过孔均为空)")
			rep.Variants = append(rep.Variants, vr)
			continue
		}
		r, a := safeAnalyze(vb, &vr)
		if a != nil {
			analyses[s.id] = a
			derived[s.id] = vb
		}
		vr.Errors = r.Errors
		vr.NetStatus = r.NetStatus
		rep.Variants = append(rep.Variants, vr)
		for _, ns := range r.NetStatus {
			if stateOf[ns.NetID] == nil {
				stateOf[ns.NetID] = map[string]string{}
			}
			stateOf[ns.NetID][s.id] = ns.State
			nameOf[ns.NetID] = ns.NetName
		}
	}
	netIDs := make([]string, 0, len(stateOf))
	for id := range stateOf {
		netIDs = append(netIDs, id)
	}
	sort.Strings(netIDs)
	for _, id := range netIDs {
		states := stateOf[id]
		first := states[slots[0].id]
		same := true
		onlyOpen, onlyShort := []string{}, []string{}
		for _, s := range slots {
			st := states[s.id]
			if st != first {
				same = false
			}
			if st == "open" {
				onlyOpen = append(onlyOpen, s.id)
			}
			if st == "short" {
				onlyShort = append(onlyShort, s.id)
			}
		}
		if same {
			continue
		}
		d := NetDiff{
			NetID: id, NetName: nameOf[id], States: states,
			OnlyOpen: onlyOpen, OnlyShort: onlyShort,
			Paths: map[string][]PathStep{},
		}
		// For every variant where this net still conducts while another
		// loses it, record the exact conductive route (copper, vias,
		// installed devices) so the UI can replay it segment by segment.
		for _, s := range slots {
			if states[s.id] != "connected" {
				continue
			}
			a, ok := analyses[s.id]
			if !ok {
				continue
			}
			path, ats := netConnPath(a, id)
			if len(path) == 0 {
				continue
			}
			d.Paths[s.id] = buildSteps(derived[s.id], a.elemNet, path, ats, a.hopLayer)
		}
		rep.Diff = append(rep.Diff, d)
	}
	return rep
}

// selectionOf snapshots a variant's assembly choices (nil for base).
func selectionOf(v *Variant) Selection {
	if v == nil {
		return Selection{DNP: []string{}, Jumpers: []string{}}
	}
	return Selection{
		DNP:     append([]string{}, v.DNP...),
		Jumpers: append([]string{}, v.Jumpers...),
	}
}

// missingRefs lists the assembly references a variant makes to elements
// the board no longer contains (deleted pads / jumpers).
func missingRefs(b *Board, v *Variant) []string {
	if v == nil {
		return nil
	}
	padIDs := map[string]bool{}
	for _, p := range b.Pads {
		padIDs[p.ID] = true
	}
	jumperIDs := map[string]bool{}
	for _, j := range b.Jumpers {
		jumperIDs[j.ID] = true
	}
	var out []string
	for _, id := range v.DNP {
		if !padIDs[id] {
			out = append(out, "方案引用了不存在的器件 "+id+" (资料缺失, 已忽略)")
		}
	}
	for _, id := range v.Jumpers {
		if !jumperIDs[id] {
			out = append(out, "方案引用了不存在的跳线 "+id+" (资料缺失, 已忽略)")
		}
	}
	return out
}

// safeAnalyze runs the geometric analysis for one derived variant view;
// a failure is recorded on that variant's report instead of aborting
// the whole comparison.
func safeAnalyze(vb *Board, vr *VariantReport) (r Report, a *analysis) {
	defer func() {
		if rec := recover(); rec != nil {
			vr.Status = "missing"
			vr.Notes = append(vr.Notes, "分析未能完成, 结果不完整")
			r = Report{Errors: []CheckError{}, NetStatus: []NetStatus{}}
			a = nil
		}
	}()
	an := buildAnalysis(vb)
	return Report{Errors: checkAnalysis(an), NetStatus: summaryOf(vb, an)}, an
}
