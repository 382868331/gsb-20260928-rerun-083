package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// StoredRepair freezes one applied (or solved) repair check together
// with its basis, so the new verification is saved separately and old
// frozen compare reports keep representing their original revision.
type StoredRepair struct {
	ID              string       `json:"id"`
	Time            string       `json:"time"`
	BasisRevision   int          `json:"basisRevision"`
	AppliedRevision int          `json:"appliedRevision,omitempty"`
	VariantID       string       `json:"variantId"`
	ModIDs          []string     `json:"modIds"`
	Cost            int64        `json:"cost"`
	Result          RepairResult `json:"result"`
	Basis           Board        `json:"basis"`
}

// repairStore persists repair records next to the board file, in their
// own repairs.json — never mixed into the frozen compare reports.
type repairStore struct {
	mu   sync.Mutex
	path string
}

func newRepairStore(boardPath string) *repairStore {
	return &repairStore{path: filepath.Join(filepath.Dir(boardPath), "repairs.json")}
}

func (s *repairStore) loadAll() ([]StoredRepair, []string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return []StoredRepair{}, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	var raws []json.RawMessage
	if err := json.Unmarshal(data, &raws); err != nil {
		return []StoredRepair{}, []string{"修复记录存档损坏, 无法解析: " + err.Error()}, nil
	}
	out := []StoredRepair{}
	warnings := []string{}
	for i, raw := range raws {
		var sr StoredRepair
		if err := json.Unmarshal(raw, &sr); err != nil {
			warnings = append(warnings, fmt.Sprintf("第 %d 份修复记录损坏, 已跳过", i+1))
			continue
		}
		sanitizeBoard(&sr.Basis)
		out = append(out, sr)
	}
	return out, warnings, nil
}

func (s *repairStore) append(sr StoredRepair) (StoredRepair, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := []StoredRepair{}
	data, err := os.ReadFile(s.path)
	if err == nil {
		_ = json.Unmarshal(data, &list)
	} else if !os.IsNotExist(err) {
		return sr, err
	}
	sanitizeBoard(&sr.Basis)
	sr.ID = fmt.Sprintf("rep-%d", time.Now().UnixNano())
	sr.Time = time.Now().UTC().Format(time.RFC3339)
	list = append(list, sr)
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return sr, err
	}
	out, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return sr, err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return sr, err
	}
	return sr, os.Rename(tmp, s.path)
}

// readRepairRequest parses a solve/preview body: either an envelope
// with its own board, or constraints to run against the saved board.
func (s *server) readRepairRequest(r *http.Request) (*Board, *RepairRequest, []string, error) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, nil, nil, err
	}
	var req RepairRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, nil, nil, err
	}
	b := s.board
	warnings := []string{}
	if req.Board != nil {
		raw := map[string]json.RawMessage{}
		if json.Unmarshal(data, &raw) == nil {
			if rb, ok := raw["board"]; ok {
				warnings = missingSections(rb)
			}
		}
		sanitizeBoard(req.Board)
		b = req.Board
	}
	return b, &req, warnings, nil
}

// solveRepair runs the constraint solver against the posted (or saved)
// board revision and current assembly. Every candidate combination is
// vetted by the real geometric connectivity check.
func (s *server) solveRepair(w http.ResponseWriter, r *http.Request) {
	b, req, warnings, err := s.readRepairRequest(r)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	res := SolveRepair(b, req)
	res.Warnings = append(warnings, res.Warnings...)
	if len(warnings) > 0 && res.Status != "incomplete" {
		res.Status = "incomplete"
		res.Solutions = []RepairSolution{}
		res.Conflicts = []string{}
	}
	writeJSON(w, 200, res)
}

// ModPreview shows what one modification does to the networks when
// applied on top of the current assembly: per-net before/after states.
type ModPreview struct {
	ModID   string     `json:"modId"`
	Label   string     `json:"label"`
	Changes []NetShift `json:"changes"`
}

type NetShift struct {
	NetID   string `json:"netId"`
	NetName string `json:"netName"`
	Before  string `json:"before"`
	After   string `json:"after"`
}

// previewRepair evaluates each listed modification individually (and the
// whole listed set together) against the current board and assembly, so
// the user can inspect every mod's electrical effect before confirming.
func (s *server) previewRepair(w http.ResponseWriter, r *http.Request) {
	b, req, warnings, err := s.readRepairRequest(r)
	if err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	base := boardForVariant(b, variantOf(b, req.VariantID))
	before := map[string]string{}
	for _, ns := range AnalyzeBoard(base).NetStatus {
		before[ns.NetID] = ns.State
	}
	shift := func(mods []RepairMod) []NetShift {
		after := AnalyzeBoard(applyMods(base, b, mods)).NetStatus
		out := []NetShift{}
		for _, ns := range after {
			if before[ns.NetID] != ns.State {
				out = append(out, NetShift{
					NetID: ns.NetID, NetName: ns.NetName,
					Before: before[ns.NetID], After: ns.State,
				})
			}
		}
		return out
	}
	previews := []ModPreview{}
	for _, m := range req.Mods {
		previews = append(previews, ModPreview{
			ModID: m.ID, Label: modLabel(m), Changes: shift([]RepairMod{m}),
		})
	}
	writeJSON(w, 200, map[string]any{
		"revision": b.Revision, "variantId": req.VariantID,
		"warnings": warnings, "mods": previews,
		"combined": shift(req.Mods),
	})
}

// repairApplyRequest confirms one chosen combination. BasisRevision
// pins the apply to the exact board revision the solve ran against: if
// the board moved on, the combination must be re-solved first.
type repairApplyRequest struct {
	VariantID        string      `json:"variantId"`
	Mods             []RepairMod `json:"mods"`
	RequireConnected []string    `json:"requireConnected"`
	ForbidShort      []string    `json:"forbidShort"`
	BasisRevision    int         `json:"basisRevision"`
}

// applyRepair re-verifies the chosen combination against the CURRENT
// saved board before changing anything. A stale basis revision or a
// combination that no longer satisfies the constraints is rejected; the
// user must re-run the solve on the new revision.
func (s *server) applyRepair(w http.ResponseWriter, r *http.Request) {
	var req repairApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	b := s.board
	if b.Revision != req.BasisRevision {
		writeJSON(w, 409, map[string]any{
			"error": fmt.Sprintf("板修订已变化 (求解依据 %d, 当前 %d), 请重新求解后再应用",
				req.BasisRevision, b.Revision),
			"stale": true,
		})
		return
	}
	// Re-check the exact combination against the current assembly before
	// applying: nothing is applied on the strength of an old verdict.
	solveReq := &RepairRequest{
		VariantID: req.VariantID, RequireConnected: req.RequireConnected,
		ForbidShort: req.ForbidShort, Mods: req.Mods,
	}
	res := SolveRepair(b, solveReq)
	if res.Status != "ok" {
		writeJSON(w, 409, map[string]any{
			"error":  "改装组合未通过当前板的复核, 请重新求解",
			"result": res,
		})
		return
	}
	// SolveRepair evaluated every subset including the full chosen set,
	// so status ok means the combination itself passed the re-check.
	variant := variantOf(b, req.VariantID)
	if variant == nil && req.VariantID != "" {
		writeJSON(w, 409, map[string]any{"error": "装配方案 " + req.VariantID + " 已不存在, 请重新求解"})
		return
	}
	nb := *b
	nb.Variants = append([]Variant{}, b.Variants...)
	if variant == nil {
		// The base assembly has no record of its own; materialize the
		// repair as a new variant so the base definition stays intact.
		nv := Variant{
			ID:   fmt.Sprintf("repair-%d", time.Now().UnixNano()),
			Name: fmt.Sprintf("修复组合 (依据修订 %d)", b.Revision),
			DNP:  []string{}, Jumpers: []string{},
		}
		nb.Variants = append(nb.Variants, nv)
		variant = &nb.Variants[len(nb.Variants)-1]
	} else {
		for i := range nb.Variants {
			if nb.Variants[i].ID == variant.ID {
				variant = &nb.Variants[i]
			}
		}
	}
	modIDs := []string{}
	cost := int64(0)
	for _, m := range req.Mods {
		modIDs = append(modIDs, m.ID)
		cost += m.Cost
		switch m.Kind {
		case "jumper":
			found := false
			for _, id := range variant.Jumpers {
				if id == m.JumperID {
					found = true
				}
			}
			if !found {
				variant.Jumpers = append(variant.Jumpers, m.JumperID)
			}
		case "dnp":
			found := false
			for _, id := range variant.DNP {
				if id == m.PadID {
					found = true
				}
			}
			if !found {
				variant.DNP = append(variant.DNP, m.PadID)
			}
		case "populate":
			dnp := []string{}
			for _, id := range variant.DNP {
				if id != m.PadID {
					dnp = append(dnp, id)
				}
			}
			variant.DNP = dnp
		}
	}
	nb.Revision = b.Revision + 1
	if err := s.st.save(&nb); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.board = &nb
	// Save the verification separately; frozen compare reports of older
	// revisions are never touched.
	res.VariantID = variant.ID
	stored, err := s.repairs.append(StoredRepair{
		BasisRevision: b.Revision, AppliedRevision: nb.Revision,
		VariantID: variant.ID, ModIDs: modIDs, Cost: cost,
		Result: res, Basis: *b,
	})
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{
		"board": nb, "variantId": variant.ID, "repair": stored,
	})
}

// listRepairs returns every saved repair verification, oldest first.
func (s *server) listRepairs(w http.ResponseWriter, r *http.Request) {
	list, warnings, err := s.repairs.loadAll()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, 200, map[string]any{"repairs": list, "warnings": warnings})
}
