package sessions

import (
	"errors"
	"fmt"

	"github.com/FacileStudio/Mycelium/internal/lockfile"
)

const scanLockName = ".sessions.lock"

// lockScan takes an exclusive lock so a manual scan and a daemon tick can never
// interleave appends into the same shard. It does not wait: the loser reports
// instead of queueing a redundant scan behind the winner.
func lockScan(dataDir string) (func(), error) {
	release, err := lockfile.Take(dataDir, scanLockName, 0)
	if errors.Is(err, lockfile.ErrHeld) {
		return nil, fmt.Errorf("another scan is already running")
	}
	return release, err
}
