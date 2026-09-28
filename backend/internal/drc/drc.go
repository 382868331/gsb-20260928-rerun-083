package drc

import (
	"fmt"
	"sort"

	"pcbworkbench/internal/geom"
	"pcbworkbench/internal/model"
)

type Config struct {
	MinClearanceMM float64 `json:"minClearanceMm"`
}

type unionFind struct{ parent []int }

func newUnionFind(n int) *unionFind {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return &unionFind{parent: p}
}

func (u *unionFind) find(x int) int {
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b int) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[rb] = ra
	}
}

type nodeKinds int

const (
	kindPad nodeKinds = iota
	kindTrace
	kindVia
)

type node struct {
	kind nodeKinds
	idx  int
}

// Analyze derives physical connectivity purely from geometry. Net labels are
// never treated as proof of connection; they are checked against what the
// copper actually touches.
func Analyze(b *model.Board, cfg Config) *model.DRCReport {
	np, nt, nv := len(b.Pads), len(b.Traces), len(b.Vias)
	n := np + nt + nv
	uf := newUnionFind(n)
	nodeAt := func(k nodeKinds, i int) int {
		switch k {
		case kindPad:
			return i
		case kindTrace:
			return np + i
		default:
			return np + nt + i
		}
	}

	var contacts []model.Contact
	add := func(c model.Contact, ka nodeKinds, ia int, kb nodeKinds, ib int) {
		contacts = append(contacts, c)
		uf.union(nodeAt(ka, ia), nodeAt(kb, ib))
	}

	for i := range b.Pads {
		for j := range b.Traces {
			if at, ok := geom.PadTraceContact(b.Pads[i], b.Traces[j]); ok {
				add(model.Contact{
					Kind: model.ContactPadTrace, At: at, Layer: b.Traces[j].Layer,
					A: b.Pads[i].ID, B: b.Traces[j].ID,
					NetA: b.Pads[i].Net, NetB: b.Traces[j].Net,
				}, kindPad, i, kindTrace, j)
			}
		}
	}
	for i := 0; i < nt; i++ {
		for j := i + 1; j < nt; j++ {
			if at, ok := geom.TraceTraceContact(b.Traces[i], b.Traces[j]); ok {
				add(model.Contact{
					Kind: model.ContactTraceTrace, At: at, Layer: b.Traces[i].Layer,
					A: b.Traces[i].ID, B: b.Traces[j].ID,
					NetA: b.Traces[i].Net, NetB: b.Traces[j].Net,
				}, kindTrace, i, kindTrace, j)
			}
		}
	}
	for i := range b.Traces {
		for j := range b.Vias {
			if at, ok := geom.TraceViaContact(b.Traces[i], b.Vias[j]); ok {
				add(model.Contact{
					Kind: model.ContactTraceVia, At: at, Layer: b.Traces[i].Layer,
					A: b.Traces[i].ID, B: b.Vias[j].ID,
					NetA: b.Traces[i].Net, NetB: b.Vias[j].Net,
				}, kindTrace, i, kindVia, j)
			}
		}
	}
	for i := range b.Pads {
		for j := range b.Vias {
			if at, ok := geom.PadViaContact(b.Pads[i], b.Vias[j]); ok {
				layer := ""
				for _, l := range geom.PadLayers(b.Pads[i]) {
					if geom.HasLayer(b.Vias[j].Layers, l) {
						layer = l
						break
					}
				}
				add(model.Contact{
					Kind: model.ContactPadVia, At: at, Layer: layer,
					A: b.Pads[i].ID, B: b.Vias[j].ID,
					NetA: b.Pads[i].Net, NetB: b.Vias[j].Net,
				}, kindPad, i, kindVia, j)
			}
		}
	}
	for i := 0; i < np; i++ {
		for j := i + 1; j < np; j++ {
			if at, ok := geom.PadPadContact(b.Pads[i], b.Pads[j]); ok {
				layer := ""
				for _, l := range geom.PadLayers(b.Pads[i]) {
					if geom.HasLayer(geom.PadLayers(b.Pads[j]), l) {
						layer = l
						break
					}
				}
				add(model.Contact{
					Kind: model.ContactPadPad, At: at, Layer: layer,
					A: b.Pads[i].ID, B: b.Pads[j].ID,
					NetA: b.Pads[i].Net, NetB: b.Pads[j].Net,
				}, kindPad, i, kindPad, j)
			}
		}
	}

	report := &model.DRCReport{Contacts: contacts}

	// Declared nets -> pad node roots.
	netPads := map[string][]int{}
	for i := range b.Pads {
		if b.Pads[i].Net != "" {
			netPads[b.Pads[i].Net] = append(netPads[b.Pads[i].Net], nodeAt(kindPad, i))
		}
	}
	netNames := make([]string, 0, len(netPads))
	for name := range netPads {
		netNames = append(netNames, name)
	}
	sort.Strings(netNames)
	for _, name := range netNames {
		pads := netPads[name]
		groups := map[int][]string{}
		padByNode := map[int]int{}
		for _, pn := range pads {
			root := uf.find(pn)
			groups[root] = append(groups[root], b.Pads[pn].ID)
			padByNode[pn] = pn
		}
		st := model.NetStatus{Net: name, Connected: len(groups) == 1, PadCount: len(pads)}
		var roots []int
		for r := range groups {
			roots = append(roots, r)
		}
		sort.Slice(roots, func(a, c int) bool {
			return groups[roots[a]][0] < groups[roots[c]][0]
		})
		for _, r := range roots {
			ids := groups[r]
			sort.Strings(ids)
			st.Groups = append(st.Groups, ids)
		}
		if !st.Connected {
			biggest := 0
			for _, r := range roots {
				if len(groups[r]) > len(groups[biggest]) {
					biggest = r
				}
			}
			for _, r := range roots {
				if r == biggest {
					continue
				}
				for _, id := range groups[r] {
					st.MissingPads = append(st.MissingPads, id)
					var idx int
					for k := range b.Pads {
						if b.Pads[k].ID == id {
							idx = k
							break
						}
					}
					at := model.Point{X: b.Pads[idx].X, Y: b.Pads[idx].Y}
					report.Issues = append(report.Issues, model.Issue{
						Severity: model.SeverityError, Code: "net-open",
						Message: fmt.Sprintf("pad %s declares net %s but copper does not reach the other pad(s) of that net", id, name),
						At:      &at, Features: []string{id},
					})
				}
			}
		}
		report.NetStatuses = append(report.NetStatuses, st)
	}

	// Shorts: every concrete contact bridging two different declared nets.
	for _, c := range contacts {
		if c.NetA != "" && c.NetB != "" && c.NetA != c.NetB {
			at := c.At
			report.Issues = append(report.Issues, model.Issue{
				Severity: model.SeverityError, Code: "short",
				Message: fmt.Sprintf("%s: net %s touches net %s at (%.3f, %.3f) on layer %s",
					c.Kind, c.NetA, c.NetB, c.At.X, c.At.Y, c.Layer),
				At: &at, Layer: c.Layer, Features: []string{c.A, c.B},
			})
		}
	}

	// Clearance: narrow edge gap between different nets on the same layer,
	// excluding pairs already shorted by contact.
	if cfg.MinClearanceMM > 0 {
		for i := 0; i < nt; i++ {
			for j := i + 1; j < nt; j++ {
				a, bb := b.Traces[i], b.Traces[j]
				if a.Layer != bb.Layer || a.Net == "" || bb.Net == "" || a.Net == bb.Net {
					continue
				}
				d, at, ok := geom.TraceTraceGap(a, bb)
				if !ok {
					continue
				}
				edge := d - (a.Width+bb.Width)/2
				if edge > geom.Epsilon && edge < cfg.MinClearanceMM {
					p := at
					report.Issues = append(report.Issues, model.Issue{
						Severity: model.SeverityError, Code: "clearance",
						Message: fmt.Sprintf("trace %s (net %s) passes %.3f mm from trace %s (net %s) on layer %s; minimum clearance %.3f mm",
							a.ID, a.Net, edge, bb.ID, bb.Net, a.Layer, cfg.MinClearanceMM),
						At: &p, Layer: a.Layer, Features: []string{a.ID, bb.ID},
					})
				}
			}
		}
	}

	// Warnings for via declarations that cannot bridge anything.
	for i := range b.Vias {
		v := b.Vias[i]
		uniq := map[string]bool{}
		for _, l := range v.Layers {
			uniq[l] = true
		}
		if len(uniq) < 2 {
			at := model.Point{X: v.X, Y: v.Y}
			report.Issues = append(report.Issues, model.Issue{
				Severity: model.SeverityWarning, Code: "via-single-layer",
				Message: fmt.Sprintf("via %s declares %d layer(s); a via must name at least two layers to cross layers", v.ID, len(uniq)),
				At:      &at, Features: []string{v.ID},
			})
		}
	}

	sort.SliceStable(report.Issues, func(i, j int) bool {
		if report.Issues[i].Code != report.Issues[j].Code {
			return report.Issues[i].Code < report.Issues[j].Code
		}
		return report.Issues[i].Message < report.Issues[j].Message
	})
	return report
}
