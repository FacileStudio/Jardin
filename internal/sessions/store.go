package sessions

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const stateName = ".sessions-state.json"

func statePath(dataDir string) string {
	return filepath.Join(dataDir, stateName)
}

// LoadState restores the scanner's resume point, or a fresh empty state when
// none exists.
func LoadState(dataDir string) *ScanState {
	state := newScanState()
	data, err := os.ReadFile(statePath(dataDir))
	if err != nil {
		return state
	}
	if err := json.Unmarshal(data, state); err != nil {
		return newScanState()
	}
	if state.Files == nil {
		state.Files = make(map[string]*FileState)
	}
	if state.Open == nil {
		state.Open = make(map[string]*Block)
	}
	if state.Projects == nil {
		state.Projects = make(map[string]string)
	}
	return state
}

// SaveState persists the scanner's resume point to disk.
func SaveState(dataDir string, state *ScanState) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(dataDir), data, 0o600)
}

func machineDir(dataDir, machine string) string {
	return filepath.Join(dataDir, "sessions", machine)
}

func appendBlocks(dataDir, machine string, blocks []Block) error {
	if len(blocks) == 0 {
		return nil
	}
	dir := machineDir(dataDir, machine)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	byMonth := make(map[string][]Block)
	for _, b := range blocks {
		month := b.StartedAt.UTC().Format("2006-01")
		byMonth[month] = append(byMonth[month], b)
	}
	for month, group := range byMonth {
		if err := writeShard(filepath.Join(dir, month+".jsonl"), group); err != nil {
			return err
		}
	}
	return nil
}

// writeShard appends one month's blocks to its shard. The file is closed on
// every path, including a mid-write failure, since a leaked descriptor on an
// append-only shard is how a later write silently goes missing.
func writeShard(path string, group []Block) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	for _, b := range group {
		line, err := json.Marshal(b)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := f.Write(append(line, '\n')); err != nil {
			f.Close()
			return err
		}
	}
	return f.Close()
}

// ReadBlocks returns every sealed block from every machine's shards, deduped
// by deterministic ID and sorted by start time. A missing sessions dir yields
// an empty slice, not an error.
func ReadBlocks(dataDir string) ([]Block, error) {
	root := filepath.Join(dataDir, "sessions")
	seen := make(map[string]bool)
	var blocks []Block
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := filepath.Base(path)
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		blocks = append(blocks, readShard(path, seen)...)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].StartedAt.Before(blocks[j].StartedAt) })
	return blocks, nil
}

// readShard returns the blocks in one shard that have not been seen already,
// recording each ID as it goes. A shard it cannot open is skipped rather than
// failing the read: shards are written by other machines and one being
// unreadable at this instant says nothing about the rest.
func readShard(path string, seen map[string]bool) []Block {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var blocks []Block
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var b Block
		if json.Unmarshal(scanner.Bytes(), &b) == nil && b.ID != "" && !seen[b.ID] {
			seen[b.ID] = true
			blocks = append(blocks, b)
		}
	}
	return blocks
}
