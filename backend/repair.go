package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RepairMod is one allowed modification (改装) the solver may pick.
// Kind "jumper" installs an optional jumper, "populate" re-installs a
// depopulated pad (removes it from the variant DNP list), "dnp"
// depopulates a pad. Every mod carries a cost and a list of other mod
// ids it is mutually exclusive with (互斥装配条件).
type RepairMod struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"` // "jumper" | "populate" | "dnp"
	Target   string   `json:"target"`
	Cost     int      `json:"cost"`
	Excludes []string `json:"excludes"`
}

// RepairSpec states the electrical requirements (必须连通 / 禁止短接)
// and the menu of allowed modifications.
type RepairSpec struct {
	MustConnect []string    `json:"mustConnect"`
	NoShort     []string    `json:"noShort"`
	Mods        []RepairMod `json:"mods"`
}

// NetChange records one net's state transition caused by a combo.
type NetChange struct {
	NetID   string `json:"netId"`
	NetName string `json:"netName"`
	From    string `json:"from"`
	To      string `json:"to"`
}

// ModEffect previews what one modification alone does to the nets.
type ModEffect struct {
	ModID     string      `json:"modId"`
	Cost      int         `json:"cost"`
	Changes   []NetChange `json:"changes"`
	Conflicts []string    `json:"conflicts"`
}

// RepairSolution is one lowest-cost combination that satisfies every
// constraint; several solutions may share the same minimal cost.
type RepairSolution struct {
	Mods      []string    `json:"mods"`
	Cost      int         `json:"cost"`
	Changes   []NetChange `json:"changes"`
	NetStatus []NetStatus `json:"netStatus"`
}

// RepairResult is the solver verdict, stamped with the exact basis key
// (board revision + assembly + spec) so a later apply can detect that
// the board moved on and demand a re-check.
type RepairResult struct {
	Revision   int              `json:"revision"`
	VariantID  string           `json:"variantId"`
	Key        string           `json:"key"`
	Warnings   []string         `json:"warnings"`
	Solutions  []RepairSolution `json:"solutions"`
	Conflicts  []string         `json:"conflicts"`
	ModEffects []ModEffect      `json:"modEffects"`
}

const maxRepairMods = 20
const maxRepairSolutions = 20

// repairKey binds a solve/apply round to its exact basis: the full
// board (revision, geometry, every variant's assembly), the variant
// being repaired and the requirement spec. Any edit in between — saved
// or not — yields a different key, so a stale confirmation can never
// be applied silently.
func repairKey(b *Board, variantID string, spec *RepairSpec) string {
	h := sha256.New()
	_ = json.NewEncoder(h).Encode(map[string]any{
		"board": b, "variant": variantID, "spec": spec,
	})
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func containsStr(list []string, id string) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

func removeStr(list []string, id string) []string {
	out := []string{}
	for _, x := range list {
		if x != id {
			out = append(out, x)
		}
	}
	return out
}

// assemblyWithMods derives the assembly that results from applying the
// chosen mods on top of the given variant's own selection.
func assemblyWithMods(b *Board, variantID string, mods []RepairMod, chosen map[string]bool) *Variant {
	out := &Variant{DNP: []string{}, Jumpers: []string{}}
	if base := variantOf(b, variantID); base != nil {
		out.DNP = append(out.DNP, base.DNP...)
		out.Jumpers = append(out.Jumpers, base.Jumpers...)
	}
	for _, m := range mods {
		if !chosen[m.ID] {
			continue
		}
		switch m.Kind {
		case "jumper":
			if !containsStr(out.Jumpers, m.Target) {
				out.Jumpers = append(out.Jumpers, m.Target)
			}
		case "populate":
			out.DNP = removeStr(out.DNP, m.Target)
		case "dnp":
			if !containsStr(out.DNP, m.Target) {
				out.DNP = append(out.DNP, m.Target)
			}
		}
	}
	return out
}

func netStateOf(ns []NetStatus) map[string]NetStatus {
	m := map[string]NetStatus{}
	for _, n := range ns {
		m[n.NetID] = n
	}
	return m
}

// evalCombo analyzes one combination against real board geometry and
// the current assembly, then checks the requirement spec plus the
// no-regression rule: a repair must not open a previously connected
// net nor introduce a short anywhere — e.g. a jumper that fixes one
// net but bridges another net's copper is rejected and the conflict is
// spelled out in the reasons.
func evalCombo(b *Board, variantID string, spec *RepairSpec, chosen map[string]bool, base map[string]NetStatus) (bool, []string, []NetChange, []NetStatus) {
	asm := assemblyWithMods(b, variantID, spec.Mods, chosen)
	combo := AnalyzeBoard(boardForVariant(b, asm))
	states := netStateOf(combo.NetStatus)
	must := map[string]bool{}
	for _, id := range spec.MustConnect {
		must[id] = true
	}
	noShort := map[string]bool{}
	for _, id := range spec.NoShort {
		noShort[id] = true
	}
	reasons := []string{}
	for _, id := range spec.MustConnect {
		st, ok := states[id]
		if !ok {
			reasons = append(reasons, "网络 "+b.netName(id)+" ("+id+") 在板上不存在, 无法判定连通")
			continue
		}
		switch st.State {
		case "connected":
		case "short":
			reasons = append(reasons, "网络 "+st.NetName+" 未满足必须连通: 与 "+netNames(b, st.ShortedWith)+" 短接")
		default:
			reasons = append(reasons, "网络 "+st.NetName+" 未满足必须连通: 仍为开路 ("+fmt.Sprint(st.Components)+" 个独立铜皮域)")
		}
	}
	for _, id := range spec.NoShort {
		if st, ok := states[id]; ok && st.State == "short" {
			reasons = append(reasons, "网络 "+st.NetName+" 违反禁止短接: 与 "+netNames(b, st.ShortedWith)+" 短接")
		}
	}
	// No-regression: nets outside the requirements must not get worse.
	for id, now := range states {
		was, existed := base[id]
		if !existed {
			continue
		}
		if !must[id] && was.State == "connected" && now.State == "open" {
			reasons = append(reasons, "改装使原本连通的网络 "+now.NetName+" 断开")
		}
		if was.State != "short" && now.State == "short" && !noShort[id] && !must[id] {
			reasons = append(reasons, "改装引入网络 "+now.NetName+" 与 "+netNames(b, now.ShortedWith)+" 的短路")
		}
	}
	changes := []NetChange{}
	for id, now := range states {
		was, existed := base[id]
		from := "open"
		if existed {
			from = was.State
		}
		if from != now.State {
			changes = append(changes, NetChange{NetID: id, NetName: now.NetName, From: from, To: now.State})
		}
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].NetID < changes[j].NetID })
	return len(reasons) == 0, reasons, changes, combo.NetStatus
}

func netNames(b *Board, ids []string) string {
	names := make([]string, 0, len(ids))
	for _, id := range ids {
		names = append(names, b.netName(id))
	}
	return strings.Join(names, "、")
}

// validateSpec weeds out unusable input. Missing geometry or dangling
// references are reported as warnings and the affected mods are
// dropped — they are never silently treated as satisfied.
func validateSpec(b *Board, spec *RepairSpec) ([]RepairMod, []string) {
	warnings := []string{}
	jumpers := map[string]bool{}
	for _, j := range b.Jumpers {
		jumpers[j.ID] = true
	}
	pads := map[string]bool{}
	for _, p := range b.Pads {
		pads[p.ID] = true
	}
	nets := map[string]bool{}
	for _, n := range b.Nets {
		nets[n.ID] = true
	}
	for _, id := range spec.MustConnect {
		if !nets[id] {
			warnings = append(warnings, "必须连通的网络 "+id+" 在板上不存在")
		}
	}
	for _, id := range spec.NoShort {
		if !nets[id] {
			warnings = append(warnings, "禁止短接的网络 "+id+" 在板上不存在")
		}
	}
	seen := map[string]bool{}
	usable := []RepairMod{}
	for _, m := range spec.Mods {
		if m.ID == "" {
			warnings = append(warnings, "存在没有 id 的改装项, 已忽略")
			continue
		}
		if seen[m.ID] {
			warnings = append(warnings, "改装 "+m.ID+" 重复定义, 已忽略后者")
			continue
		}
		seen[m.ID] = true
		switch m.Kind {
		case "jumper":
			if !jumpers[m.Target] {
				warnings = append(warnings, "改装 "+m.ID+" 引用的跳线 "+m.Target+" 在板上不存在, 该改装不可用")
				continue
			}
		case "populate", "dnp":
			if !pads[m.Target] {
				warnings = append(warnings, "改装 "+m.ID+" 引用的器件 "+m.Target+" 在板上不存在, 该改装不可用")
				continue
			}
		default:
			warnings = append(warnings, "改装 "+m.ID+" 的类型 "+m.Kind+" 未知, 已忽略")
			continue
		}
		if m.Cost < 0 {
			warnings = append(warnings, "改装 "+m.ID+" 成本为负, 已按 0 处理")
			m.Cost = 0
		}
		usable = append(usable, m)
	}
	return usable, warnings
}

// SolveRepair searches every mutually compatible combination of the
// allowed modifications and returns every combination that satisfies
// all electrical constraints at the minimum total cost — not just a
// fix for the first open found. When nothing passes, it reports a
// reviewable set of conflicting requirements instead.
func SolveRepair(b *Board, variantID string, spec *RepairSpec) RepairResult {
	usable, warnings := validateSpec(b, spec)
	res := RepairResult{
		Revision: b.Revision, VariantID: variantID,
		Key: repairKey(b, variantID, spec), Warnings: warnings,
		Solutions: []RepairSolution{}, Conflicts: []string{}, ModEffects: []ModEffect{},
	}
	mods := usable
	if len(mods) > maxRepairMods {
		res.Warnings = append(res.Warnings, fmt.Sprintf("改装项超过 %d 个, 仅考虑前 %d 个", maxRepairMods, maxRepairMods))
		mods = mods[:maxRepairMods]
	}
	specUsable := &RepairSpec{MustConnect: spec.MustConnect, NoShort: spec.NoShort, Mods: mods}

	baseStates := netStateOf(AnalyzeVariant(b, variantID).NetStatus)

	// Per-mod preview: what each modification alone does to the nets.
	for _, m := range mods {
		pass, reasons, changes, _ := evalCombo(b, variantID, specUsable, map[string]bool{m.ID: true}, baseStates)
		eff := ModEffect{ModID: m.ID, Cost: m.Cost, Changes: changes, Conflicts: []string{}}
		if !pass {
			eff.Conflicts = reasons
		}
		res.ModEffects = append(res.ModEffects, eff)
	}

	// Symmetric mutual exclusion between mods.
	excl := map[string]map[string]bool{}
	for _, m := range mods {
		for _, other := range m.Excludes {
			if excl[m.ID] == nil {
				excl[m.ID] = map[string]bool{}
			}
			excl[m.ID][other] = true
			if excl[other] == nil {
				excl[other] = map[string]bool{}
			}
			excl[other][m.ID] = true
		}
	}

	n := len(mods)
	bestCost := -1
	type reject struct {
		cost    int
		reasons []string
	}
	rejects := []reject{}
	connectedEver := map[string]bool{}
	for mask := 0; mask < (1 << n); mask++ {
		chosen := map[string]bool{}
		cost := 0
		ok := true
		for i := 0; i < n; i++ {
			if mask&(1<<i) == 0 {
				continue
			}
			for other := range excl[mods[i].ID] {
				if chosen[other] {
					ok = false
					break
				}
			}
			if !ok {
				break
			}
			chosen[mods[i].ID] = true
			cost += mods[i].Cost
		}
		if !ok {
			continue
		}
		if bestCost >= 0 && cost > bestCost {
			continue
		}
		pass, reasons, changes, ns := evalCombo(b, variantID, specUsable, chosen, baseStates)
		if !pass {
			rejects = append(rejects, reject{cost, reasons})
			continue
		}
		for _, id := range spec.MustConnect {
			if st, ok := netStateOf(ns)[id]; ok && st.State == "connected" {
				connectedEver[id] = true
			}
		}
		if bestCost < 0 || cost < bestCost {
			bestCost = cost
			res.Solutions = []RepairSolution{}
		}
		if len(res.Solutions) < maxRepairSolutions {
			ids := make([]string, 0, len(chosen))
			for id := range chosen {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			res.Solutions = append(res.Solutions, RepairSolution{Mods: ids, Cost: cost, Changes: changes, NetStatus: ns})
		}
	}

	if len(res.Solutions) > 0 {
		return res
	}
	// No combination passed: collect a reviewable conflict set from the
	// cheapest near-miss combinations plus per-requirement diagnostics.
	sort.Slice(rejects, func(i, j int) bool { return rejects[i].cost < rejects[j].cost })
	seenReason := map[string]bool{}
	for _, rj := range rejects {
		if len(res.Conflicts) >= 12 {
			break
		}
		if rj.cost > 0 && len(res.Conflicts) > 0 && rj.cost > rejects[0].cost+1 {
			break
		}
		for _, r := range rj.reasons {
			if !seenReason[r] {
				seenReason[r] = true
				res.Conflicts = append(res.Conflicts, r)
			}
		}
	}
	for _, id := range spec.MustConnect {
		if !connectedEver[id] && containsNet(b, id) {
			res.Conflicts = append(res.Conflicts, "没有任何允许的改装组合能使网络 "+b.netName(id)+" 连通")
		}
	}
	if len(res.Conflicts) == 0 {
		res.Conflicts = append(res.Conflicts, "没有可行的改装组合 (可能全部受互斥条件限制)")
	}
	return res
}

func containsNet(b *Board, id string) bool {
	for _, n := range b.Nets {
		if n.ID == id {
			return true
		}
	}
	return false
}
