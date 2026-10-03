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
	st    *store
	board *Board
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
			sanitizeBoard(b)
		} else {
			var posted Board
			if err := json.Unmarshal(data, &posted); err != nil {
				writeJSON(w, 400, map[string]string{"error": err.Error()})
				return
			}
			b = &posted
			sanitizeBoard(b)
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
	if r.ContentLength > 0 {
		var posted Board
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		b = &posted
		sanitizeBoard(b)
	}
	writeJSON(w, 200, CompareVariants(b))
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
	s := &server{st: st, board: board}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/board", s.getBoard)
	mux.HandleFunc("PUT /api/board", s.putBoard)
	mux.HandleFunc("POST /api/board/check", s.checkBoard)
	mux.HandleFunc("POST /api/board/variants/compare", s.compareVariants)
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
