package main

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// copperShape is one piece of copper owned by an element on one layer.
type copperShape struct {
	elemID string
	netID  string
	layer  string
	sh     shape
}

func nm(v int64) float64 { return float64(v) }

// buildShapes expands board elements into per-layer copper shapes.
// A via/pad is a single graph node whose shapes span its declared layers,
// which is exactly how cross-layer connectivity is established.
func buildShapes(b *Board) []copperShape {
	var out []copperShape
	for _, p := range b.Pads {
		layers := []string{p.Layer}
		if p.Layer == "through" {
			layers = b.Layers
		}
		var sh shape
		if p.Shape == "rect" {
			sh = rectShape(nm(p.X), nm(p.Y), nm(p.W), nm(p.H))
		} else {
			sh = circleShape(nm(p.X), nm(p.Y), nm(p.Diameter)/2)
		}
		for _, l := range layers {
			out = append(out, copperShape{p.ID, p.NetID, l, sh})
		}
	}
	for _, t := range b.Traces {
		half := nm(t.Width) / 2
		for i := 0; i+1 < len(t.Points); i++ {
			a := fpt{nm(t.Points[i].X), nm(t.Points[i].Y)}
			c := fpt{nm(t.Points[i+1].X), nm(t.Points[i+1].Y)}
			out = append(out, copperShape{t.ID, t.NetID, t.Layer, capsuleShape(a, c, half)})
		}
		if len(t.Points) == 1 {
			p := fpt{nm(t.Points[0].X), nm(t.Points[0].Y)}
			out = append(out, copperShape{t.ID, t.NetID, t.Layer, circleShape(p.x, p.y, half)})
		}
	}
	for _, v := range b.Vias {
		for _, l := range v.Layers {
			out = append(out, copperShape{v.ID, v.NetID, l,
				circleShape(nm(v.X), nm(v.Y), nm(v.Diameter)/2)})
		}
	}
	return out
}

type unionFind struct{ parent map[string]string }

func newUF() *unionFind { return &unionFind{map[string]string{}} }

func (u *unionFind) find(x string) string {
	p, ok := u.parent[x]
	if !ok {
		u.parent[x] = x
		return x
	}
	if p != x {
		u.parent[x] = u.find(p)
	}
	return u.parent[x]
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}

// CheckError pinpoints a connectivity problem at an exact board location
// (nanometers, board space).
type CheckError struct {
	Type     string        `json:"type"` // "open" | "short"
	Message  string        `json:"message"`
	Nets     []string      `json:"nets"`
	Elements []string      `json:"elements"`
	Segments []PathSegment `json:"segments"`
	X        float64       `json:"x"`
	Y        float64       `json:"y"`
	X2       float64       `json:"x2"`
	Y2       float64       `json:"y2"`
}

// PathSegment is one hop of a conductive fault path: two elements
// touching on a layer at a board location, with a human-readable
// explanation so the user can walk the path segment by segment.
type PathSegment struct {
	From  string  `json:"from"`
	To    string  `json:"to"`
	Layer string  `json:"layer"`
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Text  string  `json:"text"`
}

// NetStatus summarizes one net after a check: fully connected, split
// into disjoint copper (open), or merged with another net (short).
type NetStatus struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"` // "connected" | "open" | "short"
	Components int    `json:"components"`
}

// CheckResult is the full analysis returned to the client: located
// errors plus the per-net status used for instant recheck summaries
// after local edits (move/delete of vias, traces, pads).
type CheckResult struct {
	Errors []CheckError `json:"errors"`
	Nets   []NetStatus  `json:"nets"`
}

// CheckBoard analyzes real copper geometry (never net names) and reports
// opens (one net split into disjoint copper) and shorts (different nets
// physically touching), each with the contact/gap location.
func CheckBoard(b *Board) []CheckError { return Analyze(b).Errors }

// Analyze runs the full connectivity analysis and also derives the
// per-net status summary used by the instant-recheck UI.
func Analyze(b *Board) CheckResult {
	shapes := buildShapes(b)
	uf := newUF()
	type contact struct {
		a, b copperShape
		at   fpt
	}
	var contacts []contact
	for i := 0; i < len(shapes); i++ {
		for j := i + 1; j < len(shapes); j++ {
			if shapes[i].layer != shapes[j].layer || shapes[i].elemID == shapes[j].elemID {
				continue
			}
			if ok, at := shapesTouch(shapes[i].sh, shapes[j].sh); ok {
				uf.union(shapes[i].elemID, shapes[j].elemID)
				contacts = append(contacts, contact{shapes[i], shapes[j], at})
			}
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

	var errs []CheckError

	// Shorts: a connected component of copper that carries two or more
	// different net names is a short — even when the bridge is copper
	// with no net assigned (uncommitted trace/via). Locate the short on
	// the contact path that joins the two nets so the user can see
	// exactly which objects connect them.
	adj := map[string][]contactEdge{}
	for _, c := range contacts {
		adj[c.a.elemID] = append(adj[c.a.elemID], contactEdge{c.b.elemID, c.at, c.a.layer})
		adj[c.b.elemID] = append(adj[c.b.elemID], contactEdge{c.a.elemID, c.at, c.b.layer})
	}
	compNets := map[string]map[string]bool{} // component root -> nets present
	for id, net := range elemNet {
		if net == "" {
			continue
		}
		root := uf.find(id)
		if compNets[root] == nil {
			compNets[root] = map[string]bool{}
		}
		compNets[root][net] = true
	}
	roots := make([]string, 0, len(compNets))
	for root := range compNets {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	for _, root := range roots {
		nets := compNets[root]
		if len(nets) < 2 {
			continue
		}
		names := make([]string, 0, len(nets))
		for n := range nets {
			names = append(names, n)
		}
		sort.Strings(names)
		path, hops := shortPath(adj, elemNet, root, names[0])
		at := make([]fpt, len(hops))
		segments := make([]PathSegment, len(hops))
		for i, h := range hops {
			at[i] = h.at
			segments[i] = PathSegment{
				From: path[i], To: path[i+1], Layer: h.layer,
				X: h.at.x, Y: h.at.y,
				Text: fmt.Sprintf("%s <-> %s @ %s (%.3f, %.3f) mm",
					path[i], path[i+1], h.layer, h.at.x/1e6, h.at.y/1e6),
			}
		}
		m := at[len(at)/2]
		netNames := make([]string, len(names))
		for i, n := range names {
			netNames[i] = b.netName(n)
		}
		errs = append(errs, CheckError{
			Type: "short",
			Message: fmt.Sprintf("短路: 网络 %s 经 %s 在 (%.3f, %.3f) mm 处连通",
				strings.Join(netNames, " 与 "), strings.Join(path, " -> "), m.x/1e6, m.y/1e6),
			Nets:     names,
			Elements: path,
			Segments: segments,
			X:        m.x, Y: m.y, X2: m.x, Y2: m.y,
		})
	}

	// Opens: elements of one net spread over multiple components.
	netComps := map[string]map[string][]string{} // net -> comp -> elems
	for id, net := range elemNet {
		if net == "" {
			continue
		}
		comp := uf.find(id)
		if netComps[net] == nil {
			netComps[net] = map[string][]string{}
		}
		netComps[net][comp] = append(netComps[net][comp], id)
	}
	nets := make([]string, 0, len(netComps))
	for n := range netComps {
		nets = append(nets, n)
	}
	sort.Strings(nets)
	for _, net := range nets {
		comps := netComps[net]
		if len(comps) < 2 {
			continue
		}
		compIDs := make([]string, 0, len(comps))
		for c := range comps {
			compIDs = append(compIDs, c)
		}
		sort.Strings(compIDs)
		// Report the gap between each extra component and the first one.
		base := comps[compIDs[0]]
		for _, other := range compIDs[1:] {
			elems := comps[other]
			pa, pb, d, layer := closestBetween(shapes, toSet(base), toSet(elems))
			m := fmid(pa, pb)
			from := nearestElem(shapes, toSet(base), pa)
			to := nearestElem(shapes, toSet(elems), pb)
			errs = append(errs, CheckError{
				Type: "open",
				Message: fmt.Sprintf("开路: 网络 %s 未连通, 最近间隙 %.3f mm 位于 (%.3f, %.3f) mm",
					b.netName(net), d/1e6, m.x/1e6, m.y/1e6),
				Nets:     []string{net},
				Elements: append(append([]string{}, base...), elems...),
				Segments: []PathSegment{{
					From: from, To: to, Layer: layer, X: m.x, Y: m.y,
					Text: fmt.Sprintf("%s 与 %s 在 %s 层断开, 间隙 %.3f mm: (%.3f, %.3f) -> (%.3f, %.3f) mm",
						from, to, layer, d/1e6, pa.x/1e6, pa.y/1e6, pb.x/1e6, pb.y/1e6),
				}},
				X: pa.x, Y: pa.y, X2: pb.x, Y2: pb.y,
			})
		}
	}
	return CheckResult{Errors: errs, Nets: netStatuses(b, uf, elemNet, compNets)}
}

type contactEdge struct {
	to    string
	at    fpt
	layer string
}

// netStatuses derives one status per declared net: "short" if the net
// shares copper with another net, "open" if its copper is split into
// multiple components, otherwise "connected".
func netStatuses(b *Board, uf *unionFind, elemNet map[string]string, compNets map[string]map[string]bool) []NetStatus {
	compsOf := map[string]map[string]bool{} // net -> component roots
	for id, net := range elemNet {
		if net == "" {
			continue
		}
		if compsOf[net] == nil {
			compsOf[net] = map[string]bool{}
		}
		compsOf[net][uf.find(id)] = true
	}
	var out []NetStatus
	for _, n := range b.Nets {
		st := NetStatus{ID: n.ID, Name: n.Name, Status: "connected"}
		roots := compsOf[n.ID]
		st.Components = len(roots)
		if len(roots) > 1 {
			st.Status = "open"
		}
		for root := range roots {
			if len(compNets[root]) > 1 {
				st.Status = "short"
			}
		}
		out = append(out, st)
	}
	return out
}

// nearestElem returns the id of the element in set whose shape is
// closest to point p (used to name the endpoints of an open gap).
func nearestElem(shapes []copperShape, set map[string]bool, p fpt) string {
	best := math.Inf(1)
	id := ""
	for _, s := range shapes {
		if !set[s.elemID] {
			continue
		}
		d, _, _ := shapeDist(s.sh, shape{kind: shapeCircle, c1: p})
		if d < best {
			best, id = d, s.elemID
		}
	}
	return id
}

type pathHop struct {
	at    fpt
	layer string
}

// shortPath finds a contact path inside one component from an element of
// net `from` to an element of any other named net, returning the element
// ids in order and the contact point of each hop along the path.
func shortPath(adj map[string][]contactEdge, elemNet map[string]string, root, from string) ([]string, []pathHop) {
	prev := map[string]string{}
	prevHop := map[string]pathHop{}
	seen := map[string]bool{}
	var queue []string
	ids := make([]string, 0, len(elemNet))
	for id := range elemNet {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if elemNet[id] == from {
			seen[id] = true
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if net := elemNet[cur]; net != "" && net != from {
			// Reconstruct the path back to a `from` element.
			var path []string
			var hops []pathHop
			for node := cur; ; {
				path = append([]string{node}, path...)
				p, ok := prev[node]
				if !ok {
					break
				}
				hops = append([]pathHop{prevHop[node]}, hops...)
				node = p
			}
			return path, hops
		}
		for _, e := range adj[cur] {
			if !seen[e.to] {
				seen[e.to] = true
				prev[e.to] = cur
				prevHop[e.to] = pathHop{e.at, e.layer}
				queue = append(queue, e.to)
			}
		}
	}
	// Unreachable in a valid component; fall back to the root element.
	return []string{root}, []pathHop{{}}
}

func toSet(ids []string) map[string]bool {
	s := map[string]bool{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// closestBetween finds the nearest same-layer shape pair between two
// element sets (cross-layer pairs can never connect, so they are ignored).
func closestBetween(shapes []copperShape, setA, setB map[string]bool) (fpt, fpt, float64, string) {
	best := math.Inf(1)
	var ba, bb fpt
	var layer string
	for _, sa := range shapes {
		if !setA[sa.elemID] {
			continue
		}
		for _, sb := range shapes {
			if !setB[sb.elemID] || sa.layer != sb.layer {
				continue
			}
			d, pa, pb := shapeDist(sa.sh, sb.sh)
			if d < best {
				best, ba, bb, layer = d, pa, pb, sa.layer
			}
		}
	}
	return ba, bb, best, layer
}
