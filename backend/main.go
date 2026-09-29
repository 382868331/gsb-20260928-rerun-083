package main

import (
	"encoding/json"
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
	b.Revision = s.board.Revision + 1
	if err := s.st.save(&b); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	s.board = &b
	writeJSON(w, 200, s.board)
}

// checkBoard checks the board posted by the client (unsaved edits included);
// falls back to the saved board when the body is empty.
func (s *server) checkBoard(w http.ResponseWriter, r *http.Request) {
	b := s.board
	if r.ContentLength > 0 {
		var posted Board
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		b = &posted
	}
	writeJSON(w, 200, map[string]any{"errors": CheckBoard(b)})
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
