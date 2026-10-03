package main

import "sort"

// NetPath describes how one net conducts inside one assembly variant:
// the segment-by-segment copper/via/installed-component route spanning
// the net's elements. State mirrors the per-net verdict ("connected",
// "open", "short") or "empty" when the net has no copper at all in this
// variant (没有可用路径).
func NetPath(b *Board, variantID, netID string) (string, []PathStep) {
	vb := boardForVariant(b, variantOf(b, variantID))
	a := buildAnalysis(vb)
	steps := []PathStep{}

	elems := []string{}
	for id, net := range a.elemNet {
		if net == netID {
			elems = append(elems, id)
		}
	}
	sort.Strings(elems)
	if len(elems) == 0 {
		return "empty", steps
	}
	state := "connected"
	comps := map[string]bool{}
	for _, id := range elems {
		comps[a.uf.find(id)] = true
	}
	if len(comps) > 1 {
		// Not one continuous route in this variant; no single path to show.
		return "open", steps
	}
	for _, ns := range summaryOf(vb, a) {
		if ns.NetID == netID {
			state = ns.State
		}
	}
	start, goal := elems[0], elems[len(elems)-1]
	path, ats := bfsPath(a.adj, start, goal)
	if len(path) == 0 {
		path = []string{start}
	}
	return state, buildSteps(vb, a.elemNet, path, ats, a.hopLayer)
}

// bfsPath walks the contact graph from start to goal, returning the
// element ids in order plus the contact point of each hop.
func bfsPath(adj map[string][]contactEdge, start, goal string) ([]string, []fpt) {
	if start == goal {
		return []string{start}, []fpt{}
	}
	prev := map[string]string{}
	prevAt := map[string]fpt{}
	seen := map[string]bool{start: true}
	queue := []string{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == goal {
			var path []string
			var ats []fpt
			for node := goal; ; {
				path = append([]string{node}, path...)
				p, ok := prev[node]
				if !ok {
					break
				}
				ats = append([]fpt{prevAt[node]}, ats...)
				node = p
			}
			return path, ats
		}
		for _, e := range adj[cur] {
			if !seen[e.to] {
				seen[e.to] = true
				prev[e.to] = cur
				prevAt[e.to] = e.at
				queue = append(queue, e.to)
			}
		}
	}
	return nil, nil
}
