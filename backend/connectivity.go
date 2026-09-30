package main

import (
	"fmt"
	"math"
	"sort"
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

// edge is one physical contact between two elements, located at a
// board-space point.
type edge struct {
	to string
	at fpt
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
	Type     string   `json:"type"` // "open" | "short"
	Message  string   `json:"message"`
	Nets     []string `json:"nets"`
	Elements []string `json:"elements"`
	X        float64  `json:"x"`
	Y        float64  `json:"y"`
	X2       float64  `json:"x2"`
	Y2       float64  `json:"y2"`
}

// CheckBoard analyzes real copper geometry (never net names) and reports
// opens (one net split into disjoint copper) and shorts (different nets
// physically touching), each with the contact/gap location.
func CheckBoard(b *Board) []CheckError {
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

	// Contact graph between elements (edges carry the contact point),
	// used to localize shorts along the actual copper path.
	adj := map[string][]edge{}
	for _, c := range contacts {
		adj[c.a.elemID] = append(adj[c.a.elemID], edge{c.b.elemID, c.at})
		adj[c.b.elemID] = append(adj[c.b.elemID], edge{c.a.elemID, c.at})
	}

	// Group elements into physical connectivity components. Netless
	// copper (netId == "") still conducts, so it is part of a component
	// and can bridge two nets.
	compElems := map[string][]string{}
	for id := range elemNet {
		root := uf.find(id)
		compElems[root] = append(compElems[root], id)
	}
	roots := make([]string, 0, len(compElems))
	for r := range compElems {
		roots = append(roots, r)
	}
	sort.Strings(roots)

	// Shorts: one physical component carrying two or more different
	// nets, no matter whether the nets touch directly or through
	// netless copper / vias / multi-segment traces.
	for _, root := range roots {
		elems := compElems[root]
		netSet := map[string]bool{}
		for _, e := range elems {
			if n := elemNet[e]; n != "" {
				netSet[n] = true
			}
		}
		if len(netSet) < 2 {
			continue
		}
		nets := make([]string, 0, len(netSet))
		for n := range netSet {
			nets = append(nets, n)
		}
		sort.Strings(nets)
		path, pts := shortPath(elems, elemNet, adj, nets[0], nets[1])
		if len(path) == 0 {
			path = elems
			pts = []fpt{{}, {}}
		}
		first, last := pts[0], pts[len(pts)-1]
		var msg string
		if len(path) <= 2 {
			msg = fmt.Sprintf("短路: 网络 %s 与 %s 在 (%.3f, %.3f) mm 处接触",
				b.netName(nets[0]), b.netName(nets[1]), first.x/1e6, first.y/1e6)
		} else {
			msg = fmt.Sprintf("短路: 网络 %s 与 %s 经 %d 个元件连通, 路径从 (%.3f, %.3f) 到 (%.3f, %.3f) mm",
				b.netName(nets[0]), b.netName(nets[1]), len(path),
				first.x/1e6, first.y/1e6, last.x/1e6, last.y/1e6)
		}
		errs = append(errs, CheckError{
			Type:     "short",
			Message:  msg,
			Nets:     nets,
			Elements: path,
			X:        first.x, Y: first.y, X2: last.x, Y2: last.y,
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
			pa, pb, d := closestBetween(shapes, toSet(base), toSet(elems))
			m := fmid(pa, pb)
			errs = append(errs, CheckError{
				Type: "open",
				Message: fmt.Sprintf("开路: 网络 %s 未连通, 最近间隙 %.3f mm 位于 (%.3f, %.3f) mm",
					b.netName(net), d/1e6, m.x/1e6, m.y/1e6),
				Nets:     []string{net},
				Elements: append(append([]string{}, base...), elems...),
				X:        pa.x, Y: pa.y, X2: pb.x, Y2: pb.y,
			})
		}
	}
	return errs
}

// shortPath finds the shortest element chain (BFS over the contact
// graph) physically connecting netA to netB inside one component, and
// returns the element ids in order plus the contact point of each hop.
// Netless copper along the way is included, which is what lets the UI
// show exactly which objects bridge the two nets.
func shortPath(elems []string, elemNet map[string]string, adj map[string][]edge, netA, netB string) ([]string, []fpt) {
	inComp := toSet(elems)
	type prev struct {
		from string
		at   fpt
	}
	came := map[string]prev{}
	seen := map[string]bool{}
	var queue []string
	for _, e := range elems {
		if elemNet[e] == netA {
			seen[e] = true
			queue = append(queue, e)
		}
	}
	var end string
	for len(queue) > 0 && end == "" {
		cur := queue[0]
		queue = queue[1:]
		if elemNet[cur] == netB {
			end = cur
			break
		}
		for _, ed := range adj[cur] {
			if seen[ed.to] || !inComp[ed.to] {
				continue
			}
			seen[ed.to] = true
			came[ed.to] = prev{cur, ed.at}
			queue = append(queue, ed.to)
		}
	}
	if end == "" {
		return nil, nil
	}
	var path []string
	var pts []fpt
	for cur := end; ; {
		path = append([]string{cur}, path...)
		p, ok := came[cur]
		if !ok {
			break
		}
		pts = append([]fpt{p.at}, pts...)
		cur = p.from
	}
	return path, pts
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
func closestBetween(shapes []copperShape, setA, setB map[string]bool) (fpt, fpt, float64) {
	best := math.Inf(1)
	var ba, bb fpt
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
				best, ba, bb = d, pa, pb
			}
		}
	}
	return ba, bb, best
}
