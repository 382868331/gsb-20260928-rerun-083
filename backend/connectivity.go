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
	Type     string     `json:"type"` // "open" | "short"
	Message  string     `json:"message"`
	Nets     []string   `json:"nets"`
	Elements []string   `json:"elements"`
	Steps    []PathStep `json:"steps"`
	X        float64    `json:"x"`
	Y        float64    `json:"y"`
	X2       float64    `json:"x2"`
	Y2       float64    `json:"y2"`
}

// PathStep explains one segment of a conductive (fault) path.
// Kind "element" identifies an object (pad/trace/via) with its declared
// net (empty for unnamed copper); kind "hop" is the geometric contact or
// gap between two consecutive elements, on the layer where it happens.
// A cross-layer hop names both layers (e.g. "top+bottom" via a via).
type PathStep struct {
	Kind    string  `json:"kind"` // "element" | "hop"
	ID      string  `json:"id"`
	NetID   string  `json:"netId"`
	NetName string  `json:"netName"`
	Layer   string  `json:"layer"`
	X       float64 `json:"x"` // nm, board space
	Y       float64 `json:"y"`
}

func elemLayer(b *Board, id string) string {
	for _, p := range b.Pads {
		if p.ID == id {
			return p.Layer
		}
	}
	for _, t := range b.Traces {
		if t.ID == id {
			return t.Layer
		}
	}
	for _, v := range b.Vias {
		if v.ID == id {
			return strings.Join(v.Layers, "+")
		}
	}
	return ""
}

// buildSteps turns a BFS element path plus per-hop contact points into a
// segment-by-segment explanation.
func buildSteps(b *Board, elemNet map[string]string, path []string, ats []fpt, hopLayer map[[2]string]string) []PathStep {
	disp := func(id string) (string, string) {
		if id == "" {
			return "", "(未命名铜皮)"
		}
		return id, b.netName(id)
	}
	var steps []PathStep
	for i, id := range path {
		nid, nname := disp(elemNet[id])
		// ats are hop contact points, indexed by the hop they lead into:
		// element i sits at hop i-1 (the contact that reached it), or at
		// hop i when it is the first element.
		at := fpt{}
		if i == 0 && len(ats) > 0 {
			at = ats[0]
		} else if i > 0 && i-1 < len(ats) {
			at = ats[i-1]
		}
		steps = append(steps, PathStep{
			Kind: "element", ID: id, NetID: nid, NetName: nname,
			Layer: elemLayer(b, id), X: at.x, Y: at.y,
		})
		if i < len(path)-1 && i < len(ats) {
			layer := hopLayer[[2]string{path[i], path[i+1]}]
			if layer == "" {
				layer = hopLayer[[2]string{path[i+1], path[i]}]
			}
			steps = append(steps, PathStep{
				Kind: "hop", Layer: layer, X: ats[i].x, Y: ats[i].y,
			})
		}
	}
	return steps
}

// CheckBoard analyzes real copper geometry (never net names) and reports
// opens (one net split into disjoint copper) and shorts (different nets
// physically touching), each with the contact/gap location.
func CheckBoard(b *Board) []CheckError {
	return checkAnalysis(buildAnalysis(b))
}

func checkAnalysis(a *analysis) []CheckError {
	b := a.b
	shapes := a.shapes
	uf := a.uf
	adj := a.adj
	hopLayer := a.hopLayer
	elemNet := a.elemNet
	var errs []CheckError

	// Shorts: a connected component of copper that carries two or more
	// different net names is a short — even when the bridge is copper
	// with no net assigned (uncommitted trace/via). Locate the short on
	// the contact path that joins the two nets so the user can see
	// exactly which objects connect them.
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
		path, at := shortPath(adj, elemNet, root, names[0])
		m := at[len(at)/2]
		netNames := make([]string, len(names))
		for i, n := range names {
			netNames[i] = b.netName(n)
		}
		errs = append(errs, CheckError{
			Type: "short",
			Message: fmt.Sprintf("短路: 网络 %s 经 %s 在 (%.3f, %.3f) mm 处连通",
				strings.Join(netNames, " 与 "), strings.Join(path, " → "), m.x/1e6, m.y/1e6),
			Nets:     names,
			Elements: path,
			Steps:    buildSteps(b, elemNet, path, at, hopLayer),
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
			sa, sb, pa, pb, d := closestBetween(shapes, toSet(base), toSet(elems))
			if sa.elemID == "" {
				// Components live on disjoint layers (no same-layer shape
				// pair): anchor the report at representative elements and
				// mark the break as a missing cross-layer connection.
				sa, pa = anchorShape(shapes, base[0])
				sb, pb = anchorShape(shapes, elems[0])
				d = math.Inf(1)
			}
			m := fmid(pa, pb)
			// Trace the conductive route on each side of the gap so the
			// report walks the user segment by segment to the break. The
			// closest shape may belong to an unnamed conductor, so walk to
			// a named net element within each component first; when a
			// component contains no named hop target (should not happen),
			// fall back to the raw edge element.
			left, lAts := pathToNet(adj, elemNet, net, sa.elemID, toSet(base))
			right, rAts := pathToNet(adj, elemNet, net, sb.elemID, toSet(elems))
			if len(left) == 0 {
				left = []string{sa.elemID}
			}
			if len(right) == 0 {
				right = []string{sb.elemID}
			}
			openPath := append(append([]string{}, left...), right...)
			gapLayers := map[[2]string]string{}
			if len(left) > 0 && len(right) > 0 {
				layer := sa.layer
				if sb.layer != layer {
					layer = sa.layer + "+" + sb.layer
				}
				gapLayers[[2]string{left[len(left)-1], right[0]}] = layer
			}
			ats := append(append([]fpt{}, lAts...), append([]fpt{m}, rAts...)...)
			errs = append(errs, CheckError{
				Type: "open",
				Message: fmt.Sprintf("开路: 网络 %s 未连通, 最近间隙 %.3f mm 位于 (%.3f, %.3f) mm",
					b.netName(net), d/1e6, m.x/1e6, m.y/1e6),
				Nets:     []string{net},
				Elements: append(append([]string{}, base...), elems...),
				Steps:    buildSteps(b, elemNet, openPath, ats, gapLayers),
				X:        pa.x, Y: pa.y, X2: pb.x, Y2: pb.y,
			})
		}
	}
	return errs
}

type contactEdge struct {
	to string
	at fpt
}

// shortPath finds a contact path inside one component from an element of
// net `from` to an element of any other named net, returning the element
// ids in order and the contact point of each hop along the path.
func shortPath(adj map[string][]contactEdge, elemNet map[string]string, root, from string) ([]string, []fpt) {
	prev := map[string]string{}
	prevAt := map[string]fpt{}
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
			var ats []fpt
			for node := cur; ; {
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
	// Unreachable in a valid component; fall back to the root element.
	return []string{root}, []fpt{{}}
}

func toSet(ids []string) map[string]bool {
	s := map[string]bool{}
	for _, id := range ids {
		s[id] = true
	}
	return s
}

// anchorShape returns any shape owned by the element plus the element
// anchor point, used when two components share no layer at all.
func anchorShape(shapes []copperShape, id string) (copperShape, fpt) {
	for _, s := range shapes {
		if s.elemID == id {
			switch s.sh.kind {
			case shapeCircle, shapeCapsule:
				return s, s.sh.c1
			default:
				return s, s.sh.c1
			}
		}
	}
	return copperShape{elemID: id}, fpt{}
}

func nonEmpty(path []string, fallback string) []string {
	if len(path) == 0 {
		return []string{fallback}
	}
	return path
}

// pathToNet BFS-walks from start (the element at the edge of a gap)
// through its connected component until it reaches any wantNet element,
// returning ordered element ids and per-hop contact points.
func pathToNet(adj map[string][]contactEdge, elemNet map[string]string,
	wantNet, start string, scope map[string]bool) ([]string, []fpt) {
	if !scope[start] {
		return nil, nil
	}
	prev := map[string]string{}
	prevAt := map[string]fpt{}
	seen := map[string]bool{start: true}
	queue := []string{start}
	target := ""
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if elemNet[cur] == wantNet {
			target = cur
			break
		}
		nbrs := append([]contactEdge(nil), adj[cur]...)
		sort.Slice(nbrs, func(i, j int) bool { return nbrs[i].to < nbrs[j].to })
		for _, e := range nbrs {
			if scope[e.to] && !seen[e.to] {
				seen[e.to] = true
				prev[e.to] = cur
				prevAt[e.to] = e.at
				queue = append(queue, e.to)
			}
		}
	}
	if target == "" {
		return []string{start}, nil
	}
	var path []string
	var ats []fpt
	for node := target; ; {
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

// closestBetween finds the nearest same-layer shape pair between two
// element sets (cross-layer pairs can never connect, so they are ignored).
// It also returns the realizing shapes for layer labels/component walks.
func closestBetween(shapes []copperShape, setA, setB map[string]bool) (copperShape, copperShape, fpt, fpt, float64) {
	best := math.Inf(1)
	var wa, wb copperShape
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
				best, ba, bb, wa, wb = d, pa, pb, sa, sb
			}
		}
	}
	return wa, wb, ba, bb, best
}
