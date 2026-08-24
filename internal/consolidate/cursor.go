package consolidate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Position records how far one source has been consolidated: the last
// processed timestamp and the hash of the exact line it came from, so a tie
// at the same second still advances deterministically.
type Position struct {
	Timestamp time.Time `json:"timestamp"`
	LastHash  string    `json:"last_hash"`
}

// Cursor is the persisted watermark state for every source. It is advanced
// only after the write phase succeeds; deleting the file reprocesses
// everything and is the documented escape hatch.
type Cursor struct {
	Sources map[string]Position `json:"sources"`
	path    string
}

// CursorPath is where the watermark file lives under the data dir.
func CursorPath(dataDir string) string {
	return filepath.Join(dataDir, ".consolidate-cursor.json")
}

// LoadCursor reads the watermark file, returning an empty cursor when none
// exists yet.
func LoadCursor(dataDir string) (*Cursor, error) {
	path := CursorPath(dataDir)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return &Cursor{Sources: map[string]Position{}, path: path}, nil
	}
	if err != nil {
		return nil, err
	}
	var c Cursor
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.Sources == nil {
		c.Sources = map[string]Position{}
	}
	c.path = path
	return &c, nil
}

// MarkProcessed advances the watermark for source, but only forward: a stale
// position can never rewind progress.
func (c *Cursor) MarkProcessed(source string, ts time.Time, lastLineHash string) {
	existing, ok := c.Sources[source]
	if ok && !ts.After(existing.Timestamp) && !(ts.Equal(existing.Timestamp) && lastLineHash > existing.LastHash) {
		return
	}
	c.Sources[source] = Position{Timestamp: ts, LastHash: lastLineHash}
}

// PositionFor returns the recorded position for source, if any.
func (c *Cursor) PositionFor(source string) (Position, bool) {
	p, ok := c.Sources[source]
	return p, ok
}

// Save persists the cursor atomically so a crash mid-write cannot corrupt it.
func (c *Cursor) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

// HashLine hashes one raw JSONL line for cursor bookkeeping.
func HashLine(line string) string {
	sum := sha256.Sum256([]byte(line))
	return hex.EncodeToString(sum[:])
}
