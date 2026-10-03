package main

import (
	"fmt"
	"log"
	"sort"
)

// normalizeBoard repairs a partially missing board in place: nil slices
// become empty (so JSON round-trips as [] and range/map rendering never
// crashes), and elements with unusable geometry are dropped while every
// other element is kept. Each repair is returned as a located warning
// naming the element (and board position when known) so the user sees
// exactly what was skipped instead of losing the whole canvas.
func normalizeBoard(b *Board) []string {
	var warns []string
	if b.Outline == nil {
		b.Outline = []Point{}
	}
	if b.Layers == nil {
		b.Layers = []string{}
	}
	if len(b.Layers) == 0 {
		b.Layers = []string{"top", "bottom"}
		warns = append(warns, "板资料缺少层定义, 已按默认 top/bottom 两层处理")
	}
	if b.Nets == nil {
		b.Nets = []Net{}
	}
	if b.Assemblies == nil {
		b.Assemblies = []Assembly{}
	}

	loc := func(kind, id string, idx int, x, y int64) string {
		if id != "" {
			return fmt.Sprintf("%s %s (%.3f, %.3f) mm", kind, id, float64(x)/1e6, float64(y)/1e6)
		}
		return fmt.Sprintf("%s 第 %d 项 (%.3f, %.3f) mm", kind, idx+1, float64(x)/1e6, float64(y)/1e6)
	}

	pads := b.Pads[:0]
	for i, p := range b.Pads {
		if p.Shape == "rect" && (p.W <= 0 || p.H <= 0) {
			warns = append(warns, "焊盘尺寸缺失或无效, 已跳过: "+loc("焊盘", p.ID, i, p.X, p.Y))
			continue
		}
		if p.Shape != "rect" && p.Diameter <= 0 {
			warns = append(warns, "焊盘直径缺失或无效, 已跳过: "+loc("焊盘", p.ID, i, p.X, p.Y))
			continue
		}
		if p.Shape != "rect" && p.Shape != "circle" {
			p.Shape = "circle"
		}
		if p.Layer == "" {
			p.Layer = b.Layers[0]
			warns = append(warns, "焊盘缺少所在层, 已归入 "+p.Layer+": "+loc("焊盘", p.ID, i, p.X, p.Y))
		}
		pads = append(pads, p)
	}
	b.Pads = pads
	if b.Pads == nil {
		b.Pads = []Pad{}
	}

	traces := b.Traces[:0]
	for i, t := range b.Traces {
		if len(t.Points) == 0 {
			warns = append(warns, fmt.Sprintf("铜线 %s (第 %d 项) 缺少路径点, 已跳过", t.ID, i+1))
			continue
		}
		if t.Width <= 0 {
			t.Width = 500_000
			warns = append(warns, "铜线缺少线宽, 已按 0.5 mm 处理: "+
				loc("铜线", t.ID, i, t.Points[0].X, t.Points[0].Y))
		}
		if t.Layer == "" {
			t.Layer = b.Layers[0]
			warns = append(warns, "铜线缺少所在层, 已归入 "+t.Layer+": "+
				loc("铜线", t.ID, i, t.Points[0].X, t.Points[0].Y))
		}
		traces = append(traces, t)
	}
	b.Traces = traces
	if b.Traces == nil {
		b.Traces = []Trace{}
	}

	vias := b.Vias[:0]
	for i, v := range b.Vias {
		if v.Diameter <= 0 {
			warns = append(warns, "过孔直径缺失或无效, 已跳过: "+loc("过孔", v.ID, i, v.X, v.Y))
			continue
		}
		if len(v.Layers) == 0 {
			v.Layers = append([]string{}, b.Layers...)
			warns = append(warns, "过孔缺少连接层声明, 已按全部层桥接: "+loc("过孔", v.ID, i, v.X, v.Y))
		}
		vias = append(vias, v)
	}
	b.Vias = vias
	if b.Vias == nil {
		b.Vias = []Via{}
	}

	// Drop assembly exclusions that reference elements no longer present,
	// so each variant only names real mounted choices.
	present := map[string]bool{}
	for _, p := range b.Pads {
		present[p.ID] = true
	}
	for _, t := range b.Traces {
		present[t.ID] = true
	}
	for _, v := range b.Vias {
		present[v.ID] = true
	}
	for ai := range b.Assemblies {
		kept := b.Assemblies[ai].Exclude[:0]
		for _, id := range b.Assemblies[ai].Exclude {
			if present[id] {
				kept = append(kept, id)
			} else {
				warns = append(warns, fmt.Sprintf("装配方案 %s 引用了不存在的元件 %s, 已忽略",
					b.Assemblies[ai].Name, id))
			}
		}
		b.Assemblies[ai].Exclude = kept
		if b.Assemblies[ai].Exclude == nil {
			b.Assemblies[ai].Exclude = []string{}
		}
	}
	sort.Strings(warns)
	return warns
}

// normalizeLogged normalizes and logs repairs for server-side paths.
func normalizeLogged(b *Board, where string) {
	for _, w := range normalizeBoard(b) {
		log.Printf("normalize (%s): %s", where, w)
	}
}
