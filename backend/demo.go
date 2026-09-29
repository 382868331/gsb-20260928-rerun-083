package main

const mm = 1_000_000

func pt(xmm, ymm float64) Point { return Point{int64(xmm * mm), int64(ymm * mm)} }

// demoBoard is the validation board: a cross-layer crossing that must NOT
// connect, a via connecting declared layers, and a tiny 0.1 mm gap that
// must be reported as an open.
func demoBoard() *Board {
	return &Board{
		ID:   "board-demo",
		Name: "验证板: 跨层交叉 + 细小间隙",
		Outline: []Point{
			pt(0, 0), pt(60, 0), pt(60, 40), pt(0, 40),
		},
		Layers: []string{"top", "bottom"},
		Nets: []Net{
			{ID: "net-gnd", Name: "GND"},
			{ID: "net-vcc", Name: "VCC"},
			{ID: "net-sig", Name: "SIG"},
		},
		Pads: []Pad{
			{ID: "pad-sig-in", NetID: "net-sig", Layer: "top", Shape: "circle",
				X: 10 * mm, Y: 10 * mm, Diameter: 2 * mm, Drill: 0},
			{ID: "pad-sig-out", NetID: "net-sig", Layer: "bottom", Shape: "rect",
				X: 55 * mm, Y: 30 * mm, W: 3 * mm, H: 2 * mm, Drill: 0},
			{ID: "pad-gnd-a", NetID: "net-gnd", Layer: "top", Shape: "circle",
				X: 10 * mm, Y: 25 * mm, Diameter: 1.5 * mm, Drill: 0},
			// Tiny-gap pad: edge at 28.3 mm, trace end edge at 28.2 mm.
			{ID: "pad-gnd-gap", NetID: "net-gnd", Layer: "top", Shape: "circle",
				X: 29_050_000, Y: 25 * mm, Diameter: 1.5 * mm, Drill: 0},
			{ID: "pad-vcc-th", NetID: "net-vcc", Layer: "through", Shape: "circle",
				X: 10 * mm, Y: 35 * mm, Diameter: 2.2 * mm, Drill: 1 * mm},
		},
		Traces: []Trace{
			// Top-layer SIG trace, crossed by a bottom-layer SIG trace:
			// same net name but NOT connected (different layers).
			{ID: "trace-sig-top", NetID: "net-sig", Layer: "top", Width: 500_000,
				Points: []Point{pt(10, 10), pt(40, 10)}},
			{ID: "trace-sig-cross", NetID: "net-sig", Layer: "bottom", Width: 500_000,
				Points: []Point{pt(25, 2), pt(25, 18)}},
			// Continuation after the via (top -> via -> bottom).
			{ID: "trace-sig-via", NetID: "net-sig", Layer: "top", Width: 500_000,
				Points: []Point{pt(40, 10), pt(50, 10), pt(50, 15)}},
			{ID: "trace-sig-bot", NetID: "net-sig", Layer: "bottom", Width: 500_000,
				Points: []Point{pt(50, 15), pt(50, 25), pt(55, 30)}},
			// GND trace stopping 0.1 mm short of pad-gnd-gap.
			{ID: "trace-gnd-gap", NetID: "net-gnd", Layer: "top", Width: 400_000,
				Points: []Point{pt(10, 25), pt(28, 25)}},
		},
		Vias: []Via{
			{ID: "via-sig", NetID: "net-sig", X: 50 * mm, Y: 15 * mm,
				Diameter: 1.2 * mm, Drill: 600_000, Layers: []string{"top", "bottom"}},
		},
	}
}
