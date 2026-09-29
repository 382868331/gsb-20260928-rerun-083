package main

import "math"

type fpt struct{ x, y float64 }

func fsub(a, b fpt) fpt  { return fpt{a.x - b.x, a.y - b.y} }
func flen(a fpt) float64 { return math.Hypot(a.x, a.y) }
func fmid(a, b fpt) fpt  { return fpt{(a.x + b.x) / 2, (a.y + b.y) / 2} }

func closestOnSeg(a, b, p fpt) fpt {
	ab := fsub(b, a)
	d := ab.x*ab.x + ab.y*ab.y
	if d == 0 {
		return a
	}
	t := ((p.x-a.x)*ab.x + (p.y-a.y)*ab.y) / d
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	return fpt{a.x + ab.x*t, a.y + ab.y*t}
}

func cross(a, b fpt) float64 { return a.x*b.y - a.y*b.x }

// segIntersection returns a point where segments ab and cd intersect
// (including collinear overlap, where it returns the overlap midpoint).
func segIntersection(a, b, c, d fpt) (fpt, bool) {
	r := fsub(b, a)
	s := fsub(d, c)
	denom := cross(r, s)
	if math.Abs(denom) < 1e-12 {
		if math.Abs(cross(fsub(c, a), r)) > 1e-9*math.Max(1, flen(r)) {
			return fpt{}, false
		}
		if flen(r) == 0 {
			q := closestOnSeg(c, d, a)
			if flen(fsub(q, a)) < 1e-9 {
				return a, true
			}
			return fpt{}, false
		}
		var t0, t1 float64
		if math.Abs(r.x) >= math.Abs(r.y) {
			t0 = (c.x - a.x) / r.x
			t1 = (d.x - a.x) / r.x
		} else {
			t0 = (c.y - a.y) / r.y
			t1 = (d.y - a.y) / r.y
		}
		if t0 > t1 {
			t0, t1 = t1, t0
		}
		lo := math.Max(0, t0)
		hi := math.Min(1, t1)
		if lo > hi {
			return fpt{}, false
		}
		tm := (lo + hi) / 2
		return fpt{a.x + r.x*tm, a.y + r.y*tm}, true
	}
	t := cross(fsub(c, a), s) / denom
	u := cross(fsub(c, a), r) / denom
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return fpt{}, false
	}
	return fpt{a.x + r.x*t, a.y + r.y*t}, true
}

// segSegDist returns the minimum distance between segments ab and cd and
// the closest point pair (pa on ab, pb on cd).
func segSegDist(a, b, c, d fpt) (float64, fpt, fpt) {
	if p, ok := segIntersection(a, b, c, d); ok {
		return 0, p, p
	}
	best := math.Inf(1)
	var ba, bb fpt
	try := func(p, q fpt) {
		if d := flen(fsub(p, q)); d < best {
			best, ba, bb = d, p, q
		}
	}
	try(a, closestOnSeg(c, d, a))
	try(b, closestOnSeg(c, d, b))
	q := closestOnSeg(a, b, c)
	try(q, c)
	q = closestOnSeg(a, b, d)
	try(q, d)
	return best, ba, bb
}

const (
	shapeCircle = iota
	shapeRect
	shapeCapsule
)

// shape is a copper shape on one layer: circle, axis-aligned rect, or
// capsule (segment with round caps, half-width r).
type shape struct {
	kind int
	c1   fpt // center / capsule start
	c2   fpt // capsule end
	r    float64
	w, h float64 // rect
}

func circleShape(x, y, r float64) shape { return shape{kind: shapeCircle, c1: fpt{x, y}, r: r} }
func rectShape(x, y, w, h float64) shape {
	return shape{kind: shapeRect, c1: fpt{x, y}, w: w, h: h}
}
func capsuleShape(a, b fpt, halfWidth float64) shape {
	return shape{kind: shapeCapsule, c1: a, c2: b, r: halfWidth}
}

func (s shape) rectBounds() (minX, minY, maxX, maxY float64) {
	return s.c1.x - s.w/2, s.c1.y - s.h/2, s.c1.x + s.w/2, s.c1.y + s.h/2
}

func clampPtRect(p fpt, s shape) fpt {
	minX, minY, maxX, maxY := s.rectBounds()
	return fpt{math.Max(minX, math.Min(maxX, p.x)), math.Max(minY, math.Min(maxY, p.y))}
}

func ptInRect(p fpt, s shape) bool {
	minX, minY, maxX, maxY := s.rectBounds()
	return p.x >= minX && p.x <= maxX && p.y >= minY && p.y <= maxY
}

// shapeDist returns the minimum distance between two shapes and the
// closest point pair. Overlapping shapes return 0 and a contact point
// inside the overlap region.
func shapeDist(a, b shape) (float64, fpt, fpt) {
	switch {
	case a.kind == shapeCircle && b.kind == shapeCircle:
		return circleCircle(a.c1, a.r, b.c1, b.r)
	case a.kind == shapeCapsule && b.kind == shapeCapsule:
		d, pa, pb := segSegDist(a.c1, a.c2, b.c1, b.c2)
		return offset(d, pa, pb, a.r, b.r)
	case a.kind == shapeCircle && b.kind == shapeCapsule:
		q := closestOnSeg(b.c1, b.c2, a.c1)
		return circleCircle(a.c1, a.r, q, b.r)
	case a.kind == shapeCapsule && b.kind == shapeCircle:
		d, pb, pa := shapeDist(b, a)
		return d, pa, pb
	case a.kind == shapeRect && b.kind == shapeRect:
		return rectRect(a, b)
	case a.kind == shapeCircle && b.kind == shapeRect:
		return circleRect(a, b)
	case a.kind == shapeRect && b.kind == shapeCircle:
		d, pb, pa := circleRect(b, a)
		return d, pa, pb
	case a.kind == shapeCapsule && b.kind == shapeRect:
		return capsuleRect(a, b)
	case a.kind == shapeRect && b.kind == shapeCapsule:
		d, pb, pa := capsuleRect(b, a)
		return d, pa, pb
	}
	return math.Inf(1), fpt{}, fpt{}
}

// offset converts a centerline distance/pair into a surface distance/pair
// for two round-ended shapes with radii r1, r2.
func offset(d float64, pa, pb fpt, r1, r2 float64) (float64, fpt, fpt) {
	if d == 0 {
		m := fmid(pa, pb)
		return 0, m, m
	}
	dir := fpt{(pb.x - pa.x) / d, (pb.y - pa.y) / d}
	sa := fpt{pa.x + dir.x*r1, pa.y + dir.y*r1}
	sb := fpt{pb.x - dir.x*r2, pb.y - dir.y*r2}
	if d-r1-r2 <= 0 {
		m := fmid(sa, sb)
		return 0, m, m
	}
	return d - r1 - r2, sa, sb
}

func circleCircle(c1 fpt, r1 float64, c2 fpt, r2 float64) (float64, fpt, fpt) {
	d := flen(fsub(c2, c1))
	if d == 0 {
		return 0, c1, c1
	}
	return offset(d, c1, c2, r1, r2)
}

func rectRect(a, b shape) (float64, fpt, fpt) {
	aminX, aminY, amaxX, amaxY := a.rectBounds()
	bminX, bminY, bmaxX, bmaxY := b.rectBounds()
	ox := math.Min(amaxX, bmaxX) - math.Max(aminX, bminX)
	oy := math.Min(amaxY, bmaxY) - math.Max(aminY, bminY)
	if ox >= 0 && oy >= 0 {
		m := fpt{(math.Max(aminX, bminX) + math.Min(amaxX, bmaxX)) / 2,
			(math.Max(aminY, bminY) + math.Min(amaxY, bmaxY)) / 2}
		return 0, m, m
	}
	pa := clampPtRect(b.c1, a)
	pb := clampPtRect(pa, b)
	pa = clampPtRect(pb, a)
	return flen(fsub(pa, pb)), pa, pb
}

func circleRect(c shape, r shape) (float64, fpt, fpt) {
	if ptInRect(c.c1, r) {
		return 0, c.c1, c.c1
	}
	q := clampPtRect(c.c1, r)
	d := flen(fsub(c.c1, q))
	if d <= c.r {
		m := fmid(c.c1, q)
		return 0, m, m
	}
	dir := fpt{(q.x - c.c1.x) / d, (q.y - c.c1.y) / d}
	sa := fpt{c.c1.x + dir.x*c.r, c.c1.y + dir.y*c.r}
	return d - c.r, sa, q
}

func capsuleRect(cap shape, r shape) (float64, fpt, fpt) {
	if ptInRect(cap.c1, r) {
		return 0, cap.c1, cap.c1
	}
	if ptInRect(cap.c2, r) {
		return 0, cap.c2, cap.c2
	}
	minX, minY, maxX, maxY := r.rectBounds()
	corners := []fpt{{minX, minY}, {maxX, minY}, {maxX, maxY}, {minX, maxY}}
	best := math.Inf(1)
	var ba, bb fpt
	for i := 0; i < 4; i++ {
		c, d := corners[i], corners[(i+1)%4]
		if p, ok := segIntersection(cap.c1, cap.c2, c, d); ok {
			return 0, p, p
		}
		dist, pa, pb := segSegDist(cap.c1, cap.c2, c, d)
		if dist < best {
			best, ba, bb = dist, pa, pb
		}
	}
	return offset(best, ba, bb, cap.r, 0)
}

// touchEps is the contact tolerance in nm. Coordinates are integer nm, so
// any real gap is >= 1 nm and this epsilon only absorbs float error.
const touchEps = 1e-4

func shapesTouch(a, b shape) (bool, fpt) {
	d, pa, pb := shapeDist(a, b)
	if d <= touchEps {
		return true, fmid(pa, pb)
	}
	return false, fpt{}
}
