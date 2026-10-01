package main

import "sort"

// analysis is the resolved conductive graph of a board: every element is
// a node, same-layer geometric contacts are edges (vetted by real copper
// geometry, never by net names), and union-find roots name components.
type analysis struct {
	b        *Board
	shapes   []copperShape
	uf       *unionFind
	adj      map[string][]contactEdge
	hopLayer map[[2]string]string
	elemNet  map[string]string
}

// buildAnalysis expands geometry and unions every touching pair of
// elements, including unnamed copper and cross-layer via bridges.
func buildAnalysis(b *Board) *analysis {
	shapes := buildShapes(b)
	uf := newUF()
	adj := map[string][]contactEdge{}
	hopLayer := map[[2]string]string{}
	for i := 0; i < len(shapes); i++ {
		for j := i + 1; j < len(shapes); j++ {
			sa, sb := shapes[i], shapes[j]
			if sa.layer != sb.layer || sa.elemID == sb.elemID {
				continue
			}
			ok, at := shapesTouch(sa.sh, sb.sh)
			if !ok {
				continue
			}
			uf.union(sa.elemID, sb.elemID)
			aa, bb := sa.elemID, sb.elemID
			key := [2]string{aa, bb}
			if prev, ok := hopLayer[key]; ok && prev != sa.layer {
				hopLayer[key] = prev + "+" + sa.layer
			} else if !ok {
				hopLayer[key] = sa.layer
			}
			adj[aa] = append(adj[aa], contactEdge{bb, at})
			adj[bb] = append(adj[bb], contactEdge{aa, at})
		}
	}
	elemNet := map[string]string{}
	for _, p := range b.Pads {
		elemNet[p.ID] = p.NetID
	}
	for _, t := range b.Traces {
		elemNet[t.ID] = t.NetID
	}
	for _, v := range b.Vias {
		elemNet[v.ID] = v.NetID
	}
	return &analysis{b, shapes, uf, adj, hopLayer, elemNet}
}

// NetStatus is one net's connectivity verdict after geometry analysis.
// "connected" means every element of the net sits in one copper
// component (possibly through unnamed copper / vias); "open" means it is
// split across components; "short" means the component also carries a
// different net name.
type NetStatus struct {
	NetID       string   `json:"netId"`
	NetName     string   `json:"netName"`
	State       string   `json:"state"` // "connected" | "open" | "short"
	Components  int      `json:"components"`
	ShortedWith []string `json:"shortedWith"` // net ids sharing its copper
	Elements    []string `json:"elements"`
}

// Report pairs the error list with a per-net summary, so the UI can diff
// connectivity immediately after a local move or a via deletion.
type Report struct {
	Errors    []CheckError `json:"errors"`
	NetStatus []NetStatus  `json:"netStatus"`
}

// AnalyzeBoard returns both located errors and the connectivity summary.
func AnalyzeBoard(b *Board) Report {
	a := buildAnalysis(b)
	return Report{Errors: checkAnalysis(a), NetStatus: summaryOf(b, a)}
}

// NetSummary derives each named net's state purely from geometry.
func NetSummary(b *Board) []NetStatus {
	return summaryOf(b, buildAnalysis(b))
}

func summaryOf(b *Board, a *analysis) []NetStatus {

	// component -> nets present, and net -> components/elements.
	compNets := map[string]map[string]bool{}
	netComps := map[string]map[string]bool{}
	netElems := map[string][]string{}
	for id, net := range a.elemNet {
		if net == "" {
			continue
		}
		root := a.uf.find(id)
		if compNets[root] == nil {
			compNets[root] = map[string]bool{}
		}
		compNets[root][net] = true
		if netComps[net] == nil {
			netComps[net] = map[string]bool{}
		}
		netComps[net][root] = true
		netElems[net] = append(netElems[net], id)
	}

	ids := make([]string, 0, len(b.Nets))
	for _, n := range b.Nets {
		ids = append(ids, n.ID)
	}
	sort.Strings(ids)
	out := make([]NetStatus, 0, len(ids))
	for _, net := range ids {
		comps := netComps[net]
		if comps == nil {
			// Net declared but no copper carries it yet.
			out = append(out, NetStatus{NetID: net, NetName: b.netName(net), State: "open"})
			continue
		}
		shortSet := map[string]bool{}
		for c := range comps {
			for other := range compNets[c] {
				if other != net {
					shortSet[other] = true
				}
			}
		}
		state := "connected"
		if len(comps) > 1 {
			state = "open"
		}
		if len(shortSet) > 0 {
			state = "short"
		}
		shorted := make([]string, 0, len(shortSet))
		for n := range shortSet {
			shorted = append(shorted, n)
		}
		sort.Strings(shorted)
		elems := append([]string{}, netElems[net]...)
		sort.Strings(elems)
		out = append(out, NetStatus{
			NetID: net, NetName: b.netName(net), State: state,
			Components: len(comps), ShortedWith: shorted, Elements: elems,
		})
	}
	return out
}
