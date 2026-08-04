package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// lockScan takes an exclusive flock so a manual scan and a daemon tick can
// never interleave appends into the same shard. Non-blocking: the loser
// reports instead of queueing a redundant scan behind the winner.
func lockScan(dataDir string) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, ".sessions.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another scan is already running")
	}
	return func() {
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
