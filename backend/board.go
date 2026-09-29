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

type Board struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Revision int      `json:"revision"`
	Outline  []Point  `json:"outline"`
	Layers   []string `json:"layers"`
	Nets     []Net    `json:"nets"`
	Pads     []Pad    `json:"pads"`
	Traces   []Trace  `json:"traces"`
	Vias     []Via    `json:"vias"`
}

func (b *Board) netName(id string) string {
	for _, n := range b.Nets {
		if n.ID == id {
			return n.Name
		}
	}
	return id
}
