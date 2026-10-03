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
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Revision   int        `json:"revision"`
	Outline    []Point    `json:"outline"`
	Layers     []string   `json:"layers"`
	Nets       []Net      `json:"nets"`
	Pads       []Pad      `json:"pads"`
	Traces     []Trace    `json:"traces"`
	Vias       []Via      `json:"vias"`
	Assemblies []Assembly `json:"assemblies"`
}

// Assembly is one stuffing variant of the same board: the copper and via
// geometry is shared verbatim, but elements listed in Exclude (optional
// jumpers, do-not-populate parts) are treated as not mounted, so they
// neither conduct nor bridge anything in that variant.
type Assembly struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Exclude []string `json:"exclude"` // element ids not mounted here
}

func (b *Board) netName(id string) string {
	for _, n := range b.Nets {
		if n.ID == id {
			return n.Name
		}
	}
	return id
}
