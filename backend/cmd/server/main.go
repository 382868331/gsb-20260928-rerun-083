package main

import (
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"strings"

	"pcbworkbench/internal/drc"
	"pcbworkbench/internal/model"
	"pcbworkbench/internal/store"
)

type Server struct {
	boards *store.Store
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (s *Server) handleBoards(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, s.boards.List())
	case http.MethodPost:
		var b model.Board
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		created, err := s.boards.Create(b)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusCreated, created)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	id, _ := strings.CutPrefix(r.URL.Path, "/api/boards/")
	id, action, _ := strings.Cut(id, "/")
	if id == "" {
		s.handleBoards(w, r)
		return
	}
	switch {
	case action == "" && r.Method == http.MethodGet:
		b, err := s.boards.Get(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, b)
	case action == "" && r.Method == http.MethodPut:
		var b model.Board
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		saved, err := s.boards.Save(id, b)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, saved)
	case action == "undo" && r.Method == http.MethodPost:
		b, err := s.boards.Undo(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "nothing to undo or board not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, b)
	case action == "redo" && r.Method == http.MethodPost:
		b, err := s.boards.Redo(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "nothing to redo or board not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, b)
	case action == "drc" && r.Method == http.MethodPost:
		var cfg drc.Config
		if r.ContentLength != 0 {
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}
		b, err := s.boards.Get(id)
		if errors.Is(err, store.ErrNotFound) {
			http.Error(w, "board not found", http.StatusNotFound)
			return
		}
		writeJSON(w, http.StatusOK, drc.Analyze(&b, cfg))
	case action == "import" && r.Method == http.MethodPost:
		var b model.Board
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		overwrite := r.URL.Query().Get("overwrite") == "true"
		out, err := s.boards.Import(b, overwrite)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, http.StatusOK, out)
	default:
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data", "data", "directory for board JSON files")
	flag.Parse()

	boards, err := store.New(*dataDir)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	s := &Server{boards: boards}

	mux := newMux(s)

	log.Printf("PCB workbench API listening on %s (data: %s)", *addr, *dataDir)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func newMux(s *Server) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/boards", s.handleBoards)
	mux.HandleFunc("/api/boards/", s.handleBoard)
	return mux
}
