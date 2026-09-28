package model

// All coordinates and sizes are stored in millimetres. Geometry decisions are
// made exclusively from these values; UI zoom/pan never reaches this layer.

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rect is a board outline rectangle.
type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type Component struct {
	ID        string `json:"id"`
	Reference string `json:"reference"`
	Footprint string `json:"footprint"`
}

type Pad struct {
	ID          string  `json:"id"`
	ComponentID string  `json:"componentId"`
	Name        string  `json:"name"`
	Net         string  `json:"net,omitempty"`
	X           float64 `json:"x"`
	Y           float64 `json:"y"`
	Radius      float64 `json:"radius"`
	// HoleRadius > 0 means the pad is annular; copper occupies the ring.
	HoleRadius float64  `json:"holeRadius,omitempty"`
	Layers     []string `json:"layers"`
}

type Trace struct {
	ID     string  `json:"id"`
	Net    string  `json:"net,omitempty"`
	Layer  string  `json:"layer"`
	Width  float64 `json:"width"`
	Points []Point `json:"points"`
}

type Via struct {
	ID     string  `json:"id"`
	Net    string  `json:"net,omitempty"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Radius float64 `json:"radius"`
	// Layers declares exactly which layers this via electrically connects.
	Layers []string `json:"layers"`
}

type Board struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Outline    Rect        `json:"outline"`
	Components []Component `json:"components"`
	Pads       []Pad       `json:"pads"`
	Traces     []Trace     `json:"traces"`
	Vias       []Via       `json:"vias"`
}

type ContactKind string

const (
	ContactPadTrace   ContactKind = "pad-trace"
	ContactTraceTrace ContactKind = "trace-trace"
	ContactTraceVia   ContactKind = "trace-via"
	ContactPadVia     ContactKind = "pad-via"
	ContactPadPad     ContactKind = "pad-pad"
)

// Contact is an actual geometric touch between copper features. It records the
// exact point in board coordinates and the layers involved so DRC messages
// can point at a concrete location rather than a net name.
type Contact struct {
	Kind  ContactKind `json:"kind"`
	At    Point       `json:"at"`
	Layer string      `json:"layer,omitempty"`
	A     string      `json:"a"`
	B     string      `json:"b"`
	NetA  string      `json:"netA,omitempty"`
	NetB  string      `json:"netB,omitempty"`
}

type IssueSeverity string

const (
	SeverityError   IssueSeverity = "error"
	SeverityWarning IssueSeverity = "warning"
)

type Issue struct {
	Severity IssueSeverity `json:"severity"`
	Code     string        `json:"code"`
	Message  string        `json:"message"`
	At       *Point        `json:"at,omitempty"`
	Layer    string        `json:"layer,omitempty"`
	Features []string      `json:"features,omitempty"`
}

type DRCReport struct {
	NetStatuses []NetStatus `json:"netStatuses"`
	Contacts    []Contact   `json:"contacts"`
	Issues      []Issue     `json:"issues"`
}

type NetStatus struct {
	Net         string     `json:"net"`
	Connected   bool       `json:"connected"`
	PadCount    int        `json:"padCount"`
	Groups      [][]string `json:"groups"`
	MissingPads []string   `json:"missingPads,omitempty"`
}
