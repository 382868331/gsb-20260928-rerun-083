package main

import "sort"

// findAssembly returns the assembly by id, or nil for the base board
// (everything mounted).
func findAssembly(b *Board, id string) *Assembly {
	for i := range b.Assemblies {
		if b.Assemblies[i].ID == id {
			return &b.Assemblies[i]
		}
	}
	return nil
}

// boardForAssembly derives the effective board for one stuffing variant:
// identical copper/via geometry, minus the elements this variant leaves
// unmounted. The original board is never mutated, so each variant's
// verdict stays tied to its own component selection.
func boardForAssembly(b *Board, asm *Assembly) *Board {
	if asm == nil || len(asm.Exclude) == 0 {
		return b
	}
	skip := map[string]bool{}
	for _, id := range asm.Exclude {
		skip[id] = true
	}
	nb := *b
	nb.Pads = nil
	for _, p := range b.Pads {
		if !skip[p.ID] {
			nb.Pads = append(nb.Pads, p)
		}
	}
	nb.Traces = nil
	for _, t := range b.Traces {
		if !skip[t.ID] {
			nb.Traces = append(nb.Traces, t)
		}
	}
	nb.Vias = nil
	for _, v := range b.Vias {
		if !skip[v.ID] {
			nb.Vias = append(nb.Vias, v)
		}
	}
	return &nb
}

// AnalyzeAssembly runs the same geometry analysis restricted to the
// elements mounted in the named variant (empty id = base board).
func AnalyzeAssembly(b *Board, asmID string) Report {
	return AnalyzeBoard(boardForAssembly(b, findAssembly(b, asmID)))
}

// NetVariantState is one net's verdict inside one assembly variant.
type NetVariantState struct {
	AssemblyID   string `json:"assemblyId"`
	AssemblyName string `json:"assemblyName"`
	State        string `json:"state"` // "connected" | "open" | "short"
}

// VariantDiff flags a net whose connectivity differs between variants:
// it is shorted or open only under some stuffing options.
type VariantDiff struct {
	NetID   string            `json:"netId"`
	NetName string            `json:"netName"`
	Kind    string            `json:"kind"`   // "short" | "open"
	OnlyIn  []string          `json:"onlyIn"` // assembly names where it occurs
	States  []NetVariantState `json:"states"` // per-variant verdicts
}

// CompareReport bundles each variant's own report with the cross-variant
// diff, so the UI can replay per-variant paths independently.
type CompareReport struct {
	Reports map[string]Report `json:"reports"` // keyed by assembly id ("" = base)
	Diffs   []VariantDiff     `json:"diffs"`
}

type variantSlot struct {
	id   string
	name string
}

// CompareAssemblies analyzes the base board plus every declared variant
// against the same shared geometry and reports nets whose short/open
// status exists only in some variants.
func CompareAssemblies(b *Board) CompareReport {
	slots := []variantSlot{{id: "", name: "基板(全装)"}}
	for _, a := range b.Assemblies {
		slots = append(slots, variantSlot{id: a.ID, name: a.Name})
	}
	reports := map[string]Report{}
	// net -> per-slot state
	states := map[string][]NetVariantState{}
	for _, s := range slots {
		rp := AnalyzeAssembly(b, s.id)
		reports[s.id] = rp
		for _, ns := range rp.NetStatus {
			states[ns.NetID] = append(states[ns.NetID], NetVariantState{
				AssemblyID: s.id, AssemblyName: s.name, State: ns.State,
			})
		}
	}
	var diffs []VariantDiff
	netIDs := make([]string, 0, len(states))
	for id := range states {
		netIDs = append(netIDs, id)
	}
	sort.Strings(netIDs)
	for _, id := range netIDs {
		vs := states[id]
		if len(vs) != len(slots) {
			continue
		}
		for _, kind := range []string{"short", "open"} {
			var onlyIn []string
			for _, st := range vs {
				if st.State == kind {
					onlyIn = append(onlyIn, st.AssemblyName)
				}
			}
			if len(onlyIn) == 0 || len(onlyIn) == len(slots) {
				continue
			}
			diffs = append(diffs, VariantDiff{
				NetID: id, NetName: b.netName(id), Kind: kind,
				OnlyIn: onlyIn, States: vs,
			})
		}
	}
	return CompareReport{Reports: reports, Diffs: diffs}
}
