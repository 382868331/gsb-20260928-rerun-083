package main

import (
	"fmt"
	"sort"
	"strings"
)

// RepairMod is one allowed modification the solver may pick: install an
// optional jumper, depopulate a pad (DNP) or re-populate a pad the
// current assembly leaves unpopulated. Every mod carries a cost and may
// declare other mods it is mutually exclusive with (assembly conflict).
type RepairMod struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"` // "jumper" | "dnp" | "populate"
	JumperID string   `json:"jumperId,omitempty"`
	PadID    string   `json:"padId,omitempty"`
	Cost     int64    `json:"cost"`
	Excludes []string `json:"excludes,omitempty"`
	Note     string   `json:"note,omitempty"`
}

// RepairRequest describes the electrical constraints and the allowed
// modifications for one solve run against a specific board revision and
// assembly variant.
type RepairRequest struct {
	Board            *Board      `json:"board"`
	VariantID        string      `json:"variantId"`
	RequireConnected []string    `json:"requireConnected"`
	ForbidShort      []string    `json:"forbidShort"`
	Mods             []RepairMod `json:"mods"`
	MaxSolutions     int         `json:"maxSolutions"`
}

// RepairSolution is one minimum-cost combination that satisfies every
// constraint, together with the resulting per-net states so the user
// can preview the effect before applying anything.
type RepairSolution struct {
	ModIDs    []string    `json:"modIds"`
	Cost      int64       `json:"cost"`
	NetStatus []NetStatus `json:"netStatus"`
}

// RepairResult is the solver verdict: minimum-cost combinations when
// they exist, a reviewable conflict set when the requirements cannot be
// satisfied together, or "incomplete" when the basis data is missing
// pieces (which is never treated as a pass).
type RepairResult struct {
	Status    string           `json:"status"` // "ok" | "no-solution" | "incomplete"
	Revision  int              `json:"revision"`
	VariantID string           `json:"variantId"`
	Warnings  []string         `json:"warnings"`
	Solutions []RepairSolution `json:"solutions"`
	Conflicts []string         `json:"conflicts"`
}

// maxRepairMods caps the enumeration so a runaway request cannot
// explode the search space; 2^20 subsets is already far beyond a
// realistic manual repair plan.
const maxRepairMods = 20

func modLabel(m RepairMod) string {
	switch m.Kind {
	case "jumper":
		return "安装跳线 " + m.JumperID
	case "dnp":
		return "不装器件 " + m.PadID
	case "populate":
		return "补装器件 " + m.PadID
	}
	return m.ID
}

// validateRepair checks the request against the actual board: every
// required/forbidden net must exist, every mod must reference a real
// jumper or pad. Anything missing makes the basis incomplete — the
// solver must not treat absent data as satisfied.
func validateRepair(b *Board, req *RepairRequest) []string {
	w := []string{}
	nets := map[string]bool{}
	for _, n := range b.Nets {
		nets[n.ID] = true
	}
	for _, id := range req.RequireConnected {
		if !nets[id] {
			w = append(w, "要求连通的网络 "+id+" 在板上不存在")
		}
	}
	for _, id := range req.ForbidShort {
		if !nets[id] {
			w = append(w, "禁止短接的网络 "+id+" 在板上不存在")
		}
	}
	jumpers := map[string]bool{}
	for _, j := range b.Jumpers {
		jumpers[j.ID] = true
	}
	pads := map[string]bool{}
	for _, p := range b.Pads {
		pads[p.ID] = true
	}
	seen := map[string]bool{}
	for _, m := range req.Mods {
		if m.ID == "" {
			w = append(w, "存在没有 id 的改装项")
			continue
		}
		if seen[m.ID] {
			w = append(w, "改装项 id 重复: "+m.ID)
		}
		seen[m.ID] = true
		switch m.Kind {
		case "jumper":
			if !jumpers[m.JumperID] {
				w = append(w, "改装 "+m.ID+" 引用的跳线 "+m.JumperID+" 在板上不存在")
			}
		case "dnp", "populate":
			if !pads[m.PadID] {
				w = append(w, "改装 "+m.ID+" 引用的焊盘 "+m.PadID+" 在板上不存在")
			}
		default:
			w = append(w, "改装 "+m.ID+" 的类型未知: "+m.Kind)
		}
		for _, x := range m.Excludes {
			if x == m.ID {
				w = append(w, "改装 "+m.ID+" 与自身互斥, 配置无效")
			}
		}
	}
	return w
}

// applyMods derives the conductive view of the base assembly plus one
// combination of modifications. The input board is never mutated.
func applyMods(base *Board, b *Board, mods []RepairMod) *Board {
	out := *base
	dnp := map[string]bool{}
	for _, m := range mods {
		if m.Kind == "dnp" {
			dnp[m.PadID] = true
		}
	}
	populate := map[string]bool{}
	for _, m := range mods {
		if m.Kind == "populate" {
			populate[m.PadID] = true
		}
	}
	pads := make([]Pad, 0, len(base.Pads)+len(populate))
	have := map[string]bool{}
	for _, p := range base.Pads {
		if dnp[p.ID] {
			continue
		}
		have[p.ID] = true
		pads = append(pads, p)
	}
	for _, p := range b.Pads {
		if populate[p.ID] && !have[p.ID] {
			pads = append(pads, p)
		}
	}
	out.Pads = pads
	traces := make([]Trace, len(base.Traces), len(base.Traces)+len(mods))
	copy(traces, base.Traces)
	for _, m := range mods {
		if m.Kind != "jumper" {
			continue
		}
		for _, j := range b.Jumpers {
			if j.ID == m.JumperID {
				traces = append(traces, Trace{
					ID: j.ID, NetID: j.NetID, Layer: j.Layer,
					Width: j.Width, Points: j.Points,
				})
			}
		}
	}
	out.Traces = traces
	return &out
}

// excludesConflict reports whether a chosen set of mods violates any
// declared mutual-exclusion assembly condition (checked both ways).
func excludesConflict(mods []RepairMod, chosen map[int]bool) (string, bool) {
	for i, m := range mods {
		if !chosen[i] {
			continue
		}
		for _, x := range m.Excludes {
			for j, other := range mods {
				if chosen[j] && other.ID == x {
					return fmt.Sprintf("改装 %s 与 %s 互斥, 不能同时装配", m.ID, other.ID), true
				}
			}
		}
	}
	return "", false
}

// constraintsOK evaluates one derived board against the electrical
// requirements and returns the unmet ones as reviewable reasons.
func constraintsOK(rep Report, req *RepairRequest, b *Board) []string {
	state := map[string]NetStatus{}
	for _, ns := range rep.NetStatus {
		state[ns.NetID] = ns
	}
	fails := []string{}
	for _, id := range req.RequireConnected {
		ns, ok := state[id]
		if !ok || ns.State != "connected" {
			st := "open"
			if ok {
				st = ns.State
			}
			fails = append(fails, fmt.Sprintf("网络 %s 未连通 (状态 %s)", b.netName(id), st))
		}
	}
	for _, id := range req.ForbidShort {
		ns, ok := state[id]
		if !ok {
			continue
		}
		if ns.State == "short" {
			names := []string{}
			for _, o := range ns.ShortedWith {
				names = append(names, b.netName(o))
			}
			fails = append(fails, fmt.Sprintf("网络 %s 与 %s 发生禁止的短接",
				b.netName(id), strings.Join(names, ", ")))
		}
	}
	return fails
}

// SolveRepair searches every mutually-compatible combination of the
// allowed modifications against the real board geometry and the current
// assembly, and returns all minimum-cost combinations that satisfy
// every electrical constraint — not just the first open it happens to
// fix. When nothing works it returns a reviewable conflict set instead
// of a guess, and missing board/assembly data is reported as
// "incomplete" rather than treated as a pass.
func SolveRepair(b *Board, req *RepairRequest) RepairResult {
	res := RepairResult{
		Status: "ok", Revision: b.Revision, VariantID: req.VariantID,
		Warnings: []string{}, Solutions: []RepairSolution{}, Conflicts: []string{},
	}
	res.Warnings = append(res.Warnings, variantWarnings(b, variantOf(b, req.VariantID))...)
	res.Warnings = append(res.Warnings, validateRepair(b, req)...)
	if len(req.Mods) > maxRepairMods {
		res.Warnings = append(res.Warnings,
			fmt.Sprintf("改装项过多 (%d > %d), 无法穷举校验", len(req.Mods), maxRepairMods))
	}
	if len(res.Warnings) > 0 {
		res.Status = "incomplete"
		return res
	}

	base := boardForVariant(b, variantOf(b, req.VariantID))
	mods := req.Mods
	n := len(mods)
	maxSol := req.MaxSolutions
	if maxSol <= 0 {
		maxSol = 8
	}

	cache := map[uint64]evalResult{}
	evaluate := func(mask uint64) evalResult {
		if e, ok := cache[mask]; ok {
			return e
		}
		chosen := map[int]bool{}
		cost := int64(0)
		picked := []RepairMod{}
		for i := 0; i < n; i++ {
			if mask&(1<<uint(i)) != 0 {
				chosen[i] = true
				cost += mods[i].Cost
				picked = append(picked, mods[i])
			}
		}
		if why, bad := excludesConflict(mods, chosen); bad {
			e := evalResult{ok: false, fails: []string{why}, cost: cost}
			cache[mask] = e
			return e
		}
		rep := AnalyzeBoard(applyMods(base, b, picked))
		fails := constraintsOK(rep, req, b)
		e := evalResult{ok: len(fails) == 0, fails: fails, cost: cost, rep: rep}
		cache[mask] = e
		return e
	}

	best := int64(-1)
	var bestMasks []uint64
	for mask := uint64(0); mask < (uint64(1) << uint(n)); mask++ {
		e := evaluate(mask)
		if !e.ok {
			continue
		}
		if best < 0 || e.cost < best {
			best = e.cost
			bestMasks = []uint64{mask}
		} else if e.cost == best {
			bestMasks = append(bestMasks, mask)
		}
	}

	if best >= 0 {
		sort.Slice(bestMasks, func(i, j int) bool { return bestMasks[i] < bestMasks[j] })
		for _, mask := range bestMasks {
			if len(res.Solutions) >= maxSol {
				res.Warnings = append(res.Warnings,
					fmt.Sprintf("最低成本组合超过 %d 个, 仅列出前 %d 个", maxSol, maxSol))
				break
			}
			e := evaluate(mask)
			ids := []string{}
			for i := 0; i < n; i++ {
				if mask&(1<<uint(i)) != 0 {
					ids = append(ids, mods[i].ID)
				}
			}
			res.Solutions = append(res.Solutions, RepairSolution{
				ModIDs: ids, Cost: e.cost, NetStatus: e.rep.NetStatus,
			})
		}
		return res
	}

	res.Status = "no-solution"
	res.Conflicts = repairConflicts(b, req, evaluate)
	return res
}

// evalResult caches one combination's verdict so the conflict explainer
// can reuse the solver's evaluations.
type evalResult struct {
	ok    bool
	fails []string
	cost  int64
	rep   Report
}

// repairConflicts explains why no combination works, requirement by
// requirement, so the user can review and relax the conflicting demands
// instead of trusting a partial fix. Every statement is grounded in an
// actually evaluated combination against the real geometry.
func repairConflicts(b *Board, req *RepairRequest, evaluate func(uint64) evalResult) []string {
	mods := req.Mods
	n := len(mods)
	conflicts := []string{}
	seen := map[string]bool{}
	add := func(s string) {
		if !seen[s] {
			seen[s] = true
			conflicts = append(conflicts, s)
		}
	}
	netState := func(rep Report, id string) string {
		for _, ns := range rep.NetStatus {
			if ns.NetID == id {
				return ns.State
			}
		}
		return "open"
	}
	labels := func(mask uint64) string {
		names := []string{}
		for i := 0; i < n; i++ {
			if mask&(1<<uint(i)) != 0 {
				names = append(names, mods[i].ID+" ("+modLabel(mods[i])+")")
			}
		}
		if len(names) == 0 {
			return "当前装配 (不加改装)"
		}
		return strings.Join(names, " + ")
	}
	// Small combinations in size order, so explanations point at the
	// minimal change that would address each requirement.
	small := []uint64{0}
	for size := 1; size <= 2 && size <= n; size++ {
		for mask := uint64(1); mask < (uint64(1) << uint(n)); mask++ {
			if bitsOn(mask) == size {
				small = append(small, mask)
			}
		}
	}
	for _, id := range req.RequireConnected {
		fixers := []uint64{}
		for _, mask := range small {
			e := evaluate(mask)
			if e.rep.NetStatus == nil {
				continue // 互斥组合, 未做几何分析
			}
			if netState(e.rep, id) == "connected" {
				fixers = append(fixers, mask)
			}
		}
		if len(fixers) == 0 {
			add("网络 " + b.netName(id) + " 无法连通: 允许的改装中没有任何组合能连通它")
			continue
		}
		shown := 0
		for _, mask := range fixers {
			if shown >= 3 {
				break
			}
			e := evaluate(mask)
			if e.ok {
				continue // 有完整解时不会走到这里
			}
			add(fmt.Sprintf("连通 %s 需要 %s, 但 %s",
				b.netName(id), labels(mask), strings.Join(e.fails, "; ")))
			shown++
		}
	}
	for _, id := range req.ForbidShort {
		base := evaluate(0)
		if base.rep.NetStatus == nil || netState(base.rep, id) != "short" {
			continue
		}
		fixed := false
		for _, mask := range small {
			e := evaluate(mask)
			if e.rep.NetStatus != nil && netState(e.rep, id) != "short" {
				fixed = true
				break
			}
		}
		if !fixed {
			add("网络 " + b.netName(id) + " 的短接在当前装配下已存在, 允许的改装无法消除")
		}
	}
	if len(conflicts) == 0 {
		conflicts = append(conflicts, "约束彼此冲突: 没有任何改装组合能同时满足全部要求")
	}
	return conflicts
}

func bitsOn(mask uint64) int {
	c := 0
	for mask != 0 {
		c += int(mask & 1)
		mask >>= 1
	}
	return c
}
