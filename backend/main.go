package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
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
	mux.HandleFunc("POST /api/board/repair/solve", s.solveRepair)
	mux.HandleFunc("POST /api/board/repair/preview", s.previewRepair)
	mux.HandleFunc("POST /api/board/repair/apply", s.applyRepair)
	mux.HandleFunc("GET /api/board/repairs", s.listRepairs)
	mux.HandleFunc("GET /api/board/export", s.exportBoard)
	mux.HandleFunc("POST /api/board/import", s.importBoard)
	mux.HandleFunc("POST /api/board/reset", s.resetBoard)

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
