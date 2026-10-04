package main

// All coordinates and sizes are integer nanometers (nm) in board space.
// View zoom/pan never enters this model, so geometry checks are
// resolution- and zoom-independent.

type Point struct {
	X int64 `json:"x"`
	Y int64 `json:"y"`
}

type Net struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Pad.Layer is a layer id ("top", "bottom", ...) or "through" for a
// through-hole pad that exists on every copper layer.
type Pad struct {
	ID       string `json:"id"`
	NetID    string `json:"netId"`
	Layer    string `json:"layer"`
	Shape    string `json:"shape"` // "circle" | "rect"
	X        int64  `json:"x"`
	Y        int64  `json:"y"`
	Diameter int64  `json:"diameter"` // circle
	W        int64  `json:"w"`        // rect
	H        int64  `json:"h"`        // rect
	Drill    int64  `json:"drill"`
}

type Trace struct {
	ID     string  `json:"id"`
	NetID  string  `json:"netId"`
	Layer  string  `json:"layer"`
	Width  int64   `json:"width"`
	Points []Point `json:"points"`
}

// Via.Layers declares exactly which layers the via barrel connects.
type Via struct {
	ID       string   `json:"id"`
	NetID    string   `json:"netId"`
	X        int64    `json:"x"`
	Y        int64    `json:"y"`
	Diameter int64    `json:"diameter"`
	Drill    int64    `json:"drill"`
	Layers   []string `json:"layers"`
}

// Jumper is an optional conductive link (跳线). Its copper exists only in
// assembly variants that list it as installed; geometry lives on the
// board so every variant reasons about the same coordinates.
type Jumper struct {
	ID     string  `json:"id"`
	NetID  string  `json:"netId"`
	Layer  string  `json:"layer"`
	Width  int64   `json:"width"`
	Points []Point `json:"points"`
}

// Variant is one assembly option (装配方案): which pads are left
// unpopulated (DNP) and which optional jumpers are installed. Copper
// traces and vias are shared by all variants and never duplicated here.
type Variant struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	DNP     []string `json:"dnp"`     // pad ids not populated in this variant
	Jumpers []string `json:"jumpers"` // jumper ids installed in this variant
}

type Board struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Revision int       `json:"revision"`
	Outline  []Point   `json:"outline"`
	Layers   []string  `json:"layers"`
	Nets     []Net     `json:"nets"`
	Pads     []Pad     `json:"pads"`
	Traces   []Trace   `json:"traces"`
	Vias     []Via     `json:"vias"`
	Jumpers  []Jumper  `json:"jumpers"`
	Variants []Variant `json:"variants"`
	// Compares is the frozen compare-report history, persisted with the
	// project so a reload can recover past reports and tell whether the
	// current board still matches their basis. Server-owned: a client
	// PUT without this field keeps the stored history.
	Compares []StoredCompare `json:"compares,omitempty"`
}

func (b *Board) netName(id string) string {
	for _, n := range b.Nets {
		if n.ID == id {
			return n.Name
		}
	}
	return id
}
