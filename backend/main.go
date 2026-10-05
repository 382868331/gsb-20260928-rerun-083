package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type server struct {
	st       *store
	compares *compareStore
	repairs  *repairStore
	board    *Board
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *server) getBoard(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.board)
}

func (s *server) putBoard(w http.ResponseWriter, r *http.Request) {
	var b Board
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	sanitizeBoard(&b)
	b.Revision = s.board.Revision + 1
	if err := s.st.save(&b); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.board = &b
	writeJSON(w, 200, s.board)
}

// checkEnvelope lets the client ask for one assembly variant's verdict;
// a bare Board body (no "board" field) is still accepted as before.
type checkEnvelope struct {
	Board     *Board `json:"board"`
	VariantID string `json:"variantId"`
}

// checkBoard checks the board posted by the client (unsaved edits included);
// falls back to the saved board when the body is empty. Each call rebuilds
// the analysis from the posted revision, so switching variants, moving a
// via or undoing an edit never reuses a previous variant's conclusions.
func (s *server) checkBoard(w http.ResponseWriter, r *http.Request) {
	b := s.board
	variantID := ""
	if r.ContentLength > 0 {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		var env checkEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		if env.Board != nil {
			b = env.Board
			variantID = env.VariantID
		} else {
			var posted Board
			if err := json.Unmarshal(data, &posted); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			b = &posted
		}
	}
	rp := AnalyzeVariant(b, variantID)
	writeJSON(w, 200, map[string]any{
		"errors": rp.Errors, "netStatus": rp.NetStatus,
		"variantId": variantID, "revision": b.Revision,
	})
}

// compareVariants re-checks every assembly variant of the posted (or
// saved) board against the same shared copper geometry and reports
// which nets are shorted/open only under specific assembly options.
func (s *server) compareVariants(w http.ResponseWriter, r *http.Request) {
	b := s.board
	warnings := []string{}
	if r.ContentLength > 0 {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		var posted Board
		if err := json.Unmarshal(data, &posted); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		warnings = missingSections(data)
		sanitizeBoard(&posted)
		b = &posted
	}
	rep := CompareVariants(b)
	rep.Warnings = append(rep.Warnings, warnings...)
	writeJSON(w, 200, rep)
}

// missingSections names the board sections absent from a posted
// document, so a compare/check report can flag that its basis was
// partially missing instead of looking like a clean bill of health.
func missingSections(data []byte) []string {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return []string{"板资料不是有效的 JSON 对象"}
	}
	out := []string{}
	for _, f := range []string{"outline", "layers", "nets", "pads", "traces", "vias", "jumpers", "variants"} {
		if _, ok := raw[f]; !ok {
			out = append(out, "字段 "+f+" 缺失, 已按空列表/默认值处理")
		}
	}
	return out
}

// netPathEnvelope asks for the conductive route of one net inside one
// assembly variant of a posted board revision.
type netPathEnvelope struct {
	Board     *Board `json:"board"`
	VariantID string `json:"variantId"`
	NetID     string `json:"netId"`
}

// netPath returns the segment-by-segment route a net uses in one
// variant (copper traces, vias, installed jumpers/components), so the
// UI can show why a net conducts only under a specific assembly.
func (s *server) netPath(w http.ResponseWriter, r *http.Request) {
	var env netPathEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if env.Board == nil || env.NetID == "" {
		writeJSON(w, 400, map[string]string{"error": "需要 board 与 netId"})
		return
	}
	sanitizeBoard(env.Board)
	state, steps := NetPath(env.Board, env.VariantID, env.NetID)
	writeJSON(w, 200, map[string]any{
		"variantId": env.VariantID, "netId": env.NetID,
		"revision": env.Board.Revision, "state": state, "steps": steps,
	})
}

func (s *server) exportBoard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="board.json"`)
	json.NewEncoder(w).Encode(s.board)
}

// importBoard replaces the whole board with the uploaded file. Element IDs
// are preserved verbatim so identity survives export -> import.
func (s *server) importBoard(w http.ResponseWriter, r *http.Request) {
	var b Board
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	sanitizeBoard(&b)
	b.Revision = s.board.Revision + 1
	if err := s.st.save(&b); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.board = &b
	writeJSON(w, 200, s.board)
}

func (s *server) resetBoard(w http.ResponseWriter, r *http.Request) {
	b := demoBoard()
	b.Revision = s.board.Revision + 1
	if err := s.st.save(b); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.board = b
	writeJSON(w, 200, s.board)
}

// solveEnvelope posts the board (unsaved edits included), the assembly
// variant to repair and the requirement spec.
type solveEnvelope struct {
	Board     *Board     `json:"board"`
	VariantID string     `json:"variantId"`
	Spec      RepairSpec `json:"spec"`
}

// solveRepair searches the minimum-cost modification combinations and
// freezes the round as a stored repair report; old reports keep
// representing their own revision, new checks are appended separately.
func (s *server) solveRepair(w http.ResponseWriter, r *http.Request) {
	var env solveEnvelope
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	b := s.board
	if env.Board != nil {
		b = env.Board
	}
	sanitizeBoard(b)
	spec := env.Spec
	if spec.MustConnect == nil {
		spec.MustConnect = []string{}
	}
	if spec.NoShort == nil {
		spec.NoShort = []string{}
	}
	if spec.Mods == nil {
		spec.Mods = []RepairMod{}
	}
	res := SolveRepair(b, env.VariantID, &spec)
	stored, err := s.repairs.append(StoredRepair{
		Key: res.Key, VariantID: env.VariantID, Spec: spec, Result: res, Basis: *b,
	})
	if err == nil {
		writeJSON(w, 200, map[string]any{"result": res, "reportId": stored.ID})
	} else {
		writeJSON(w, 200, map[string]any{"result": res, "reportId": ""})
	}
}

// previewRepair evaluates one arbitrary combination of mods without
// solving, so the user can inspect each modification's net impact
// before confirming anything.
func (s *server) previewRepair(w http.ResponseWriter, r *http.Request) {
	var env struct {
		Board     *Board     `json:"board"`
		VariantID string     `json:"variantId"`
		Spec      RepairSpec `json:"spec"`
		Mods      []string   `json:"mods"`
	}
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if env.Board == nil {
		writeJSON(w, 400, map[string]string{"error": "需要 board"})
		return
	}
	sanitizeBoard(env.Board)
	usable, warnings := validateSpec(env.Board, &env.Spec)
	spec := &RepairSpec{MustConnect: env.Spec.MustConnect, NoShort: env.Spec.NoShort, Mods: usable}
	chosen := map[string]bool{}
	for _, id := range env.Mods {
		chosen[id] = true
	}
	base := netStateOf(AnalyzeVariant(env.Board, env.VariantID).NetStatus)
	pass, reasons, changes, ns := evalCombo(env.Board, env.VariantID, spec, chosen, base)
	writeJSON(w, 200, map[string]any{
		"pass": pass, "reasons": reasons, "changes": changes,
		"netStatus": ns, "warnings": warnings,
		"revision": env.Board.Revision, "variantId": env.VariantID,
	})
}

// applyRepair confirms one solved combination. The basis key is
// recomputed from the posted board and spec: if the board revision,
// geometry or assembly changed since the solve, the apply is refused
// and the user must re-solve first. On success the combination becomes
// a new assembly variant and the board is saved with a bumped revision.
func (s *server) applyRepair(w http.ResponseWriter, r *http.Request) {
	var env struct {
		Board     *Board     `json:"board"`
		VariantID string     `json:"variantId"`
		Spec      RepairSpec `json:"spec"`
		Mods      []string   `json:"mods"`
		Key       string     `json:"key"`
		Name      string     `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	if env.Board == nil || env.Key == "" {
		writeJSON(w, 400, map[string]string{"error": "需要 board 与求解依据 key"})
		return
	}
	sanitizeBoard(env.Board)
	spec := env.Spec
	if spec.MustConnect == nil {
		spec.MustConnect = []string{}
	}
	if spec.NoShort == nil {
		spec.NoShort = []string{}
	}
	if spec.Mods == nil {
		spec.Mods = []RepairMod{}
	}
	if key := repairKey(env.Board, env.VariantID, &spec); key != env.Key {
		writeJSON(w, 409, map[string]string{"error": "板修订或装配在求解后已变化, 必须重新求解核对后才能应用"})
		return
	}
	usable, warnings := validateSpec(env.Board, &spec)
	if len(warnings) > 0 {
		writeJSON(w, 409, map[string]string{"error": "改装资料不完整, 不能应用: " + strings.Join(warnings, "; ")})
		return
	}
	chosen := map[string]bool{}
	cost := 0
	for _, m := range usable {
		if containsStr(env.Mods, m.ID) {
			chosen[m.ID] = true
			cost += m.Cost
		}
	}
	base := netStateOf(AnalyzeVariant(env.Board, env.VariantID).NetStatus)
	pass, reasons, _, _ := evalCombo(env.Board, env.VariantID, &spec, chosen, base)
	if !pass {
		writeJSON(w, 409, map[string]string{"error": "组合复核未通过, 不能应用: " + strings.Join(reasons, "; ")})
		return
	}
	asm := assemblyWithMods(env.Board, env.VariantID, usable, chosen)
	name := env.Name
	if name == "" {
		name = "修复组合 (" + strings.Join(env.Mods, "+") + ")"
	}
	asm.ID = fmt.Sprintf("var-repair-%d", time.Now().UnixNano())
	asm.Name = name
	env.Board.Variants = append(env.Board.Variants, *asm)
	env.Board.Revision = s.board.Revision + 1
	if err := s.st.save(env.Board); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.board = env.Board
	writeJSON(w, 200, map[string]any{"board": s.board, "variantId": asm.ID, "cost": cost})
}

// listRepairs returns every frozen repair report saved with the
// project, oldest first; a single damaged entry never costs the rest.
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

// listCompares returns every frozen compare report saved with the
// project, oldest first, so a reopened session can recover them all
// even when one entry is damaged.
func (s *server) listCompares(w http.ResponseWriter, r *http.Request) {
	list, warnings, err := s.compares.loadAll()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	if warnings == nil {
		warnings = []string{}
	}
	writeJSON(w, 200, map[string]any{"compares": list, "warnings": warnings})
}

// saveCompare freezes a compare report together with its basis (the
// compare key and the full board snapshot). Re-checking appends a new
// entry; previous reports are never overwritten.
func (s *server) saveCompare(w http.ResponseWriter, r *http.Request) {
	var sc StoredCompare
	if err := json.NewDecoder(r.Body).Decode(&sc); err != nil {
		writeJSON(w, 400, map[string]string{"error": err.Error()})
		return
	}
	stored, err := s.compares.append(sc)
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, stored)
}

func main() {
	dataPath := os.Getenv("PCB_DATA")
	if dataPath == "" {
		dataPath = "data/board.json"
	}
	st := newStore(dataPath)
	board, err := st.load()
	if err != nil {
		board = demoBoard()
		if err := st.save(board); err != nil {
			log.Printf("warn: cannot persist demo board: %v", err)
		}
	}
	s := &server{st: st, compares: newCompareStore(dataPath), repairs: newRepairStore(dataPath), board: board}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/board", s.getBoard)
	mux.HandleFunc("PUT /api/board", s.putBoard)
	mux.HandleFunc("POST /api/board/check", s.checkBoard)
	mux.HandleFunc("POST /api/board/variants/compare", s.compareVariants)
	mux.HandleFunc("GET /api/board/compares", s.listCompares)
	mux.HandleFunc("POST /api/board/compares", s.saveCompare)
	mux.HandleFunc("POST /api/board/netpath", s.netPath)
	mux.HandleFunc("GET /api/board/export", s.exportBoard)
	mux.HandleFunc("POST /api/board/import", s.importBoard)
	mux.HandleFunc("POST /api/board/reset", s.resetBoard)
	mux.HandleFunc("POST /api/repair/solve", s.solveRepair)
	mux.HandleFunc("POST /api/repair/preview", s.previewRepair)
	mux.HandleFunc("POST /api/repair/apply", s.applyRepair)
	mux.HandleFunc("GET /api/repair/reports", s.listRepairs)

	// Serve the built frontend if present.
	dist := os.Getenv("PCB_STATIC")
	if dist == "" {
		dist = filepath.Join("..", "frontend", "dist")
	}
	if info, err := os.Stat(dist); err == nil && info.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(dist)))
	}

	addr := os.Getenv("PCB_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("PCB workbench listening on %s (static: %s)", addr, dist)
	log.Fatal(http.ListenAndServe(addr, mux))
}
