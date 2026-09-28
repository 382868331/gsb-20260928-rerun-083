package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"pcbworkbench/internal/model"
)

var ErrNotFound = errors.New("board not found")

type doc struct {
	board model.Board
	undo  []model.Board
	redo  []model.Board
}

type Store struct {
	mu       sync.Mutex
	dir      string
	docs     map[string]*doc
	maxStack int
}

func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, docs: map[string]*doc{}, maxStack: 200}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		var b model.Board
		if err := json.Unmarshal(raw, &b); err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		s.docs[b.ID] = &doc{board: b}
	}
	return s, nil
}

func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func (s *Store) List() []model.Board {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.docs))
	for id := range s.docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]model.Board, 0, len(ids))
	for _, id := range ids {
		out = append(out, s.docs[id].board)
	}
	return out
}

func (s *Store) Get(id string) (model.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok {
		return model.Board{}, ErrNotFound
	}
	return d.board, nil
}

func (s *Store) Create(b model.Board) (model.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.ID == "" {
		b.ID = NewID()
	}
	if _, exists := s.docs[b.ID]; exists {
		return model.Board{}, fmt.Errorf("board %s already exists", b.ID)
	}
	if err := s.writeLocked(b); err != nil {
		return model.Board{}, err
	}
	s.docs[b.ID] = &doc{board: b}
	return b, nil
}

// Save replaces the board and records the previous state for undo.
func (s *Store) Save(id string, b model.Board) (model.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok {
		return model.Board{}, ErrNotFound
	}
	b.ID = id
	d.undo = append(d.undo, d.board)
	if len(d.undo) > s.maxStack {
		d.undo = d.undo[len(d.undo)-s.maxStack:]
	}
	d.redo = nil
	if err := s.writeLocked(b); err != nil {
		return model.Board{}, err
	}
	d.board = b
	return b, nil
}

// Import stores an exported board verbatim, preserving every ID. With
// overwrite it replaces an existing board and records an undo entry.
func (s *Store) Import(b model.Board, overwrite bool) (model.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b.ID == "" {
		return model.Board{}, errors.New("imported board has no id")
	}
	if existing, ok := s.docs[b.ID]; ok {
		if !overwrite {
			return model.Board{}, fmt.Errorf("board %s already exists", b.ID)
		}
		existing.undo = append(existing.undo, existing.board)
		existing.redo = nil
	} else {
		s.docs[b.ID] = &doc{}
	}
	if err := s.writeLocked(b); err != nil {
		return model.Board{}, err
	}
	s.docs[b.ID].board = b
	return b, nil
}

func (s *Store) Undo(id string) (model.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok || len(d.undo) == 0 {
		return model.Board{}, ErrNotFound
	}
	d.redo = append(d.redo, d.board)
	d.board = d.undo[len(d.undo)-1]
	d.undo = d.undo[:len(d.undo)-1]
	if err := s.writeLocked(d.board); err != nil {
		return model.Board{}, err
	}
	return d.board, nil
}

func (s *Store) Redo(id string) (model.Board, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok || len(d.redo) == 0 {
		return model.Board{}, ErrNotFound
	}
	d.undo = append(d.undo, d.board)
	d.board = d.redo[len(d.redo)-1]
	d.redo = d.redo[:len(d.redo)-1]
	if err := s.writeLocked(d.board); err != nil {
		return model.Board{}, err
	}
	return d.board, nil
}

func (s *Store) StackDepth(id string) (int, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.docs[id]
	if !ok {
		return 0, 0, ErrNotFound
	}
	return len(d.undo), len(d.redo), nil
}

func (s *Store) writeLocked(b model.Board) error {
	raw, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, b.ID+".json.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, b.ID+".json"))
}
