package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
)

// Vector is one chunk's embedding.
type Vector []float32

// ModelID identifies the model that produced an index. Vectors from different
// models are not comparable, so an index carrying a different ModelID than the
// live backend must be rebuilt rather than queried.
type ModelID struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Dims   int    `json:"dims"`
}

// Matches reports whether two model identities are the same. A digest is
// compared when both sides have one, because a moving `:latest` tag is exactly
// how an index silently starts lying.
func (m ModelID) Matches(other ModelID) bool {
	if m.Name != other.Name || m.Dims != other.Dims {
		return false
	}
	if m.Digest == "" || other.Digest == "" {
		return true
	}
	return m.Digest == other.Digest
}

// Backend turns text into vectors. Implementations are expected to be remote
// and fallible; every caller must degrade to lexical search on error.
type Backend interface {
	Embed(ctx context.Context, texts []string) ([]Vector, error)
	Model(ctx context.Context) (ModelID, error)
}

// Entry is one indexed chunk. Hash is the content hash of the embedded text,
// which is what lets an incremental index skip unchanged blocks.
type Entry struct {
	Key     string `json:"key"`
	Path    string `json:"path"`
	Heading string `json:"heading"`
	Line    int    `json:"line"`
	Hash    string `json:"hash"`
	Vector  Vector `json:"-"`
}

// Scored is one ranked chunk, from either retrieval half.
type Scored struct {
	Key   string
	Path  string
	Line  int
	Score float64
}

// Store holds vectors and answers nearest-neighbour queries. The shipped
// implementation scans every vector, which is exact; an approximate index
// would be a different implementation of this interface, not a change to it.
//
// Nearest returns an error rather than an empty slice on failure: a caller
// reporting whether search degraded cannot tell "the backend is down" from
// "nothing matched" if both look like no results.
type Store interface {
	Model() ModelID
	Hashes() map[string]string
	Upsert(entries []Entry) error
	DeletePaths(paths []string) error
	Nearest(query Vector, limit int) ([]Scored, error)
}

// ChunkKey identifies a chunk stably across runs.
func ChunkKey(c Chunk) string {
	return fmt.Sprintf("%s#%d", c.Path, c.Line)
}

// ChunkHash is the content hash of what would be embedded, so a block whose
// text has not changed is never embedded twice.
func ChunkHash(c Chunk) string {
	sum := sha256.Sum256([]byte(c.Text()))
	return hex.EncodeToString(sum[:])
}

// Cosine is the similarity used for ranking. Vectors of differing length score
// zero rather than panicking, because a mismatched index must be inert, not
// fatal.
func Cosine(a, b Vector) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
