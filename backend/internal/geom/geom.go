package geom

import (
	"math"

	"pcbworkbench/internal/model"
)

const Epsilon = 1e-9

func Sub(a, b model.Point) model.Point {
	return model.Point{X: a.X - b.X, Y: a.Y - b.Y}
}

func Dot(a, b model.Point) float64 { return a.X*b.X + a.Y*b.Y }

// SegmentDistance returns the minimum distance between segment p1-p2 and
// segment q1-q2, along with the closest points on each segment.
func SegmentDistance(p1, p2, q1, q2 model.Point) (float64, model.Point, model.Point) {
	d1 := Sub(p2, p1)
	d2 := Sub(q2, q1)
	a := Dot(d1, d1)
	e := Dot(d2, d2)
	c := Dot(d1, Sub(p1, q1))
	f := Dot(d2, Sub(p1, q1))
	if a <= Epsilon && e <= Epsilon {
		return Norm(Sub(p1, q1)), p1, q1
	}
	if a <= Epsilon {
		t := clamp01(f / e)
		cq := model.Point{X: q1.X + d2.X*t, Y: q1.Y + d2.Y*t}
		return Norm(Sub(p1, cq)), p1, cq
	}
	if e <= Epsilon {
		s := clamp01(-c / a)
		cp := model.Point{X: p1.X + d1.X*s, Y: p1.Y + d1.Y*s}
		return Norm(Sub(cp, q1)), cp, q1
	}
	b := Dot(d1, d2)
	denom := a*e - b*b
	s := 0.0
	if math.Abs(denom) <= Epsilon {
		s = clamp01(-c / a)
	} else {
		s = clamp01((b*f - c*e) / denom)
	}
	t := clamp01((b*s + f) / e)
	s = clamp01((b*t - c) / a)
	t = clamp01((b*s + f) / e)
	cp := model.Point{X: p1.X + d1.X*s, Y: p1.Y + d1.Y*s}
	cq := model.Point{X: q1.X + d2.X*t, Y: q1.Y + d2.Y*t}
	return Norm(Sub(cp, cq)), cp, cq
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func Norm(a model.Point) float64 { return math.Hypot(a.X, a.Y) }

func PointSegmentDistance(p, a, b model.Point) (float64, model.Point) {
	dx, dy := b.X-a.X, b.Y-a.Y
	l2 := dx*dx + dy*dy
	if l2 <= Epsilon {
		return Norm(Sub(p, a)), a
	}
	t := clamp01(((p.X-a.X)*dx + (p.Y-a.Y)*dy) / l2)
	c := model.Point{X: a.X + dx*t, Y: a.Y + dy*t}
	return Norm(Sub(p, c)), c
}

type SegmentApproach struct {
	I, J     int
	Distance float64
	At       model.Point
}

// PolylineApproach returns every pair of segments from the two polylines
// whose centerline distance is <= gap, with the closest-approach midpoint.
func PolylineApproach(pointsA, pointsB []model.Point, gap float64) []SegmentApproach {
	var out []SegmentApproach
	for i := 0; i+1 < len(pointsA); i++ {
		for j := 0; j+1 < len(pointsB); j++ {
			d, ca, cb := SegmentDistance(pointsA[i], pointsA[i+1], pointsB[j], pointsB[j+1])
			if d <= gap+Epsilon {
				out = append(out, SegmentApproach{
					I: i, J: j, Distance: d,
					At: model.Point{X: (ca.X + cb.X) / 2, Y: (ca.Y + cb.Y) / 2},
				})
			}
		}
	}
	return out
}

func HasLayer(layers []string, layer string) bool {
	for _, l := range layers {
		if l == layer {
			return true
		}
	}
	return false
}

func PadLayers(p model.Pad) []string {
	if len(p.Layers) > 0 {
		return p.Layers
	}
	return []string{"top"}
}

// PadTraceContact reports corridor overlap of an annular/disc pad with a
// trace on any shared layer.
func PadTraceContact(p model.Pad, tr model.Trace) (model.Point, bool) {
	if !HasLayer(PadLayers(p), tr.Layer) {
		return model.Point{}, false
	}
	pp := model.Point{X: p.X, Y: p.Y}
	for i := 0; i+1 < len(tr.Points); i++ {
		d, closest := PointSegmentDistance(pp, tr.Points[i], tr.Points[i+1])
		if d <= p.Radius+tr.Width/2+Epsilon {
			if p.HoleRadius > 0 && d < p.HoleRadius-Epsilon {
				continue
			}
			return closest, true
		}
	}
	return model.Point{}, false
}

// TraceTraceContact tests two traces on the SAME layer: copper corridors
// overlap when centerline distance <= (w1+w2)/2.
func TraceTraceContact(a, b model.Trace) (model.Point, bool) {
	if a.Layer != b.Layer {
		return model.Point{}, false
	}
	approaches := PolylineApproach(a.Points, b.Points, (a.Width+b.Width)/2)
	if len(approaches) == 0 {
		return model.Point{}, false
	}
	best := approaches[0]
	for _, ap := range approaches[1:] {
		if ap.Distance < best.Distance {
			best = ap
		}
	}
	return best.At, true
}

// TraceTraceGap returns the minimum centerline gap between same-layer traces.
func TraceTraceGap(a, b model.Trace) (float64, model.Point, bool) {
	if a.Layer != b.Layer {
		return 0, model.Point{}, false
	}
	approaches := PolylineApproach(a.Points, b.Points, math.Inf(1))
	if len(approaches) == 0 {
		return 0, model.Point{}, false
	}
	best := approaches[0]
	for _, ap := range approaches[1:] {
		if ap.Distance < best.Distance {
			best = ap
		}
	}
	return best.Distance, best.At, true
}

func TraceViaContact(tr model.Trace, v model.Via) (model.Point, bool) {
	if !HasLayer(v.Layers, tr.Layer) {
		return model.Point{}, false
	}
	vp := model.Point{X: v.X, Y: v.Y}
	for i := 0; i+1 < len(tr.Points); i++ {
		d, closest := PointSegmentDistance(vp, tr.Points[i], tr.Points[i+1])
		if d <= v.Radius+tr.Width/2+Epsilon {
			return closest, true
		}
	}
	return model.Point{}, false
}

func PadViaContact(p model.Pad, v model.Via) (model.Point, bool) {
	shared := false
	for _, l := range PadLayers(p) {
		if HasLayer(v.Layers, l) {
			shared = true
		}
	}
	if !shared {
		return model.Point{}, false
	}
	d := math.Hypot(p.X-v.X, p.Y-v.Y)
	if d <= p.Radius+v.Radius+Epsilon {
		if p.HoleRadius > 0 && d < p.HoleRadius-Epsilon {
			return model.Point{}, false
		}
		return model.Point{X: (p.X + v.X) / 2, Y: (p.Y + v.Y) / 2}, true
	}
	return model.Point{}, false
}

func PadPadContact(a, b model.Pad) (model.Point, bool) {
	shared := false
	for _, la := range PadLayers(a) {
		if HasLayer(PadLayers(b), la) {
			shared = true
		}
	}
	if !shared {
		return model.Point{}, false
	}
	d := math.Hypot(a.X-b.X, a.Y-b.Y)
	if d <= a.Radius+b.Radius+Epsilon {
		if (a.HoleRadius > 0 && d < a.HoleRadius-Epsilon) || (b.HoleRadius > 0 && d < b.HoleRadius-Epsilon) {
			return model.Point{}, false
		}
		return model.Point{X: (a.X + b.X) / 2, Y: (a.Y + b.Y) / 2}, true
	}
	return model.Point{}, false
}
