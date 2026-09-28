package geom

import (
	"math"
	"testing"

	"pcbworkbench/internal/model"
)

func TestSegmentDistanceParallel(t *testing.T) {
	a1 := model.Point{X: 0, Y: 0}
	a2 := model.Point{X: 0, Y: 2.35}
	b1 := model.Point{X: 0, Y: 2.65}
	b2 := model.Point{X: 0, Y: 5}
	d, _, _ := SegmentDistance(a1, a2, b1, b2)
	if math.Abs(d-0.3) > 1e-9 {
		t.Fatalf("parallel gap = %v, want 0.3", d)
	}
}

func TestSegmentDistanceCrossing(t *testing.T) {
	d, ca, cb := SegmentDistance(
		model.Point{X: 0, Y: 0}, model.Point{X: 10, Y: 10},
		model.Point{X: 0, Y: 10}, model.Point{X: 10, Y: 0},
	)
	if d > 1e-9 {
		t.Fatalf("crossing distance = %v, want 0 (%v %v)", d, ca, cb)
	}
}
