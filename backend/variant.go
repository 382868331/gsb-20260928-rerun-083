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
	rep := CompareReport{Revision: b.Revision}
	stateOf := map[string]map[string]string{}
	nameOf := map[string]string{}
	for _, s := range slots {
		r := AnalyzeVariant(b, s.id)
		vr := VariantReport{
			VariantID: s.id, Name: s.name, Revision: b.Revision,
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
		var onlyOpen, onlyShort []string
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
