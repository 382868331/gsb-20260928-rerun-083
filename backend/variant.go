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

// VariantReport is one variant's verdict, stamped with the board
// revision it was computed from.
type VariantReport struct {
	VariantID string       `json:"variantId"`
	Name      string       `json:"name"`
	Revision  int          `json:"revision"`
	Status    string       `json:"status"` // "ok" | "empty" | "incomplete"
	Warnings  []string     `json:"warnings"`
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
}

// CompareReport holds every variant's independent result plus the
// cross-variant net diff.
type CompareReport struct {
	ID       string          `json:"id,omitempty"`  // set once the report is frozen/persisted
	Key      string          `json:"key,omitempty"` // fingerprint of the basis board
	Revision int             `json:"revision"`
	Variants []VariantReport `json:"variants"`
	Diff     []NetDiff       `json:"diff"`
	Warnings []string        `json:"warnings"`
}

// variantWarnings lists assembly selections that reference board
// objects which do not exist (anymore): the variant can still be
// analyzed, but its basis is incomplete and the report must say so
// instead of silently ignoring the dangling ids.
func variantWarnings(b *Board, v *Variant) []string {
	w := []string{}
	if v == nil {
		return w
	}
	pads := map[string]bool{}
	for _, p := range b.Pads {
		pads[p.ID] = true
	}
	for _, id := range v.DNP {
		if !pads[id] {
			w = append(w, "方案 "+v.ID+" 的未装器件 "+id+" 在板上不存在, 已忽略")
		}
	}
	jumpers := map[string]bool{}
	for _, j := range b.Jumpers {
		jumpers[j.ID] = true
	}
	for _, id := range v.Jumpers {
		if !jumpers[id] {
			w = append(w, "方案 "+v.ID+" 的跳线 "+id+" 在板上不存在, 已忽略")
		}
	}
	return w
}

// variantStatus classifies one variant's result so the UI can tell
// "no faults found" apart from "no conductive path available at all"
// and "assembly data incomplete".
func variantStatus(vb *Board, warnings []string) string {
	if len(warnings) > 0 {
		return "incomplete"
	}
	if len(vb.Pads) == 0 && len(vb.Traces) == 0 && len(vb.Vias) == 0 {
		return "empty"
	}
	return "ok"
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
	rep := CompareReport{
		Revision: b.Revision,
		Variants: []VariantReport{},
		Diff:     []NetDiff{},
		Warnings: []string{},
	}
	stateOf := map[string]map[string]string{}
	nameOf := map[string]string{}
	for _, s := range slots {
		v := variantOf(b, s.id)
		warnings := variantWarnings(b, v)
		vb := boardForVariant(b, v)
		r := AnalyzeBoard(vb)
		vr := VariantReport{
			VariantID: s.id, Name: s.name, Revision: b.Revision,
			Status: variantStatus(vb, warnings), Warnings: warnings,
			Errors: r.Errors, NetStatus: r.NetStatus,
		}
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
		rep.Diff = append(rep.Diff, NetDiff{
			NetID: id, NetName: nameOf[id], States: states,
			OnlyOpen: onlyOpen, OnlyShort: onlyShort,
		})
	}
	return rep
}
