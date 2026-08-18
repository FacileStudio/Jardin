package memory

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const flatIndexFile = "index.json"

type flatRecord struct {
	Key     string `json:"key"`
	Path    string `json:"path"`
	Heading string `json:"heading"`
	Line    int    `json:"line"`
	Hash    string `json:"hash"`
	Vector  []byte `json:"vector"`
}

type flatIndex struct {
	Model   ModelID      `json:"model"`
	Records []flatRecord `json:"records"`
}

// FlatStore is an exact vector store: every query scans every vector, so recall
// is 1.0 by construction. Under a few hundred thousand chunks that costs
// milliseconds, and an approximate index would trade recall for time nobody is
// short of.
//
// The whole index is one JSON file. Vectors ride inside it as base64 of raw
// little-endian float32, which keeps the bits exact and the file atomic: a
// separate vector file would need two renames, and a crash between them would
// pair a new manifest with stale vectors.
type FlatStore struct {
	mu      sync.RWMutex
	path    string
	model   ModelID
	entries []Entry
	byKey   map[string]int
}

// OpenFlatStore loads the index in dir, or returns an empty store when there is
// none. An index whose ModelID does not match, or whose JSON no longer parses,
// is discarded rather than reported: both are a rebuild of a derived cache, not
// a failure, and querying vectors from another model would return confident
// nonsense. Only an unreadable file is an error.
func OpenFlatStore(dir string, model ModelID) (*FlatStore, error) {
	store := &FlatStore{
		path:  filepath.Join(dir, flatIndexFile),
		model: model,
		byKey: map[string]int{},
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", store.path, err)
	}
	var index flatIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return store, nil
	}
	if !index.Model.Matches(model) {
		return store, nil
	}
	store.load(index.Records)
	return store, nil
}

func (s *FlatStore) load(records []flatRecord) {
	s.entries = make([]Entry, 0, len(records))
	for _, rec := range records {
		s.byKey[rec.Key] = len(s.entries)
		s.entries = append(s.entries, Entry{
			Key:     rec.Key,
			Path:    rec.Path,
			Heading: rec.Heading,
			Line:    rec.Line,
			Hash:    rec.Hash,
			Vector:  flatDecodeVector(rec.Vector),
		})
	}
}

// Model returns the model identity this index was built with.
func (s *FlatStore) Model() ModelID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}

// Hashes returns the content hash of every indexed chunk, keyed by chunk key,
// so an incremental reindex can skip blocks whose text has not changed.
func (s *FlatStore) Hashes() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	hashes := make(map[string]string, len(s.entries))
	for _, entry := range s.entries {
		hashes[entry.Key] = entry.Hash
	}
	return hashes
}

// Upsert replaces entries sharing a Key and appends the rest, then rewrites the
// index atomically.
func (s *FlatStore) Upsert(entries []Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, entry := range entries {
		if at, ok := s.byKey[entry.Key]; ok {
			s.entries[at] = entry
			continue
		}
		s.byKey[entry.Key] = len(s.entries)
		s.entries = append(s.entries, entry)
	}
	return s.persist()
}

// DeletePaths drops every entry belonging to one of the given paths. Paths that
// hold nothing are not an error.
func (s *FlatStore) DeletePaths(paths []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	doomed := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		doomed[path] = struct{}{}
	}
	kept := make([]Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		if _, ok := doomed[entry.Path]; ok {
			continue
		}
		kept = append(kept, entry)
	}
	if len(kept) == len(s.entries) {
		return nil
	}
	s.entries = kept
	s.reindex()
	return s.persist()
}

func (s *FlatStore) reindex() {
	s.byKey = make(map[string]int, len(s.entries))
	for at, entry := range s.entries {
		s.byKey[entry.Key] = at
	}
}

// Nearest ranks every vector by cosine similarity and returns the best limit of
// them, highest first. Equal scores break on Key, so the same index and query
// always produce the same order: a ranking that drifts between runs cannot be
// measured, let alone improved. A limit of zero or less returns everything.
func (s *FlatStore) Nearest(query Vector, limit int) []Scored {
	s.mu.RLock()
	defer s.mu.RUnlock()
	scored := make([]Scored, 0, len(s.entries))
	for _, entry := range s.entries {
		scored = append(scored, Scored{
			Key:   entry.Key,
			Path:  entry.Path,
			Line:  entry.Line,
			Score: Cosine(query, entry.Vector),
		})
	}
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Key < scored[j].Key
	})
	if limit > 0 && limit < len(scored) {
		scored = scored[:limit]
	}
	return scored
}

func (s *FlatStore) persist() error {
	index := flatIndex{Model: s.model, Records: make([]flatRecord, 0, len(s.entries))}
	for _, entry := range s.entries {
		index.Records = append(index.Records, flatRecord{
			Key:     entry.Key,
			Path:    entry.Path,
			Heading: entry.Heading,
			Line:    entry.Line,
			Hash:    entry.Hash,
			Vector:  flatEncodeVector(entry.Vector),
		})
	}
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return flatWriteAtomic(s.path, data)
}

func flatWriteAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	if err := flatFinishTemp(tmp, data); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func flatFinishTemp(tmp *os.File, data []byte) error {
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Chmod(tmp.Name(), 0600)
}

func flatEncodeVector(v Vector) []byte {
	raw := make([]byte, len(v)*4)
	for i, f := range v {
		binary.LittleEndian.PutUint32(raw[i*4:], math.Float32bits(f))
	}
	return raw
}

func flatDecodeVector(raw []byte) Vector {
	v := make(Vector, len(raw)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
	}
	return v
}
