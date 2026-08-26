package journal

import (
	"errors"
	"fmt"
	"time"

	"github.com/FacileStudio/Mycelium/internal/lockfile"
)

const (
	lockName = ".journal.lock"
	lockWait = 2 * time.Second
)

// lockJournal takes an exclusive lock so two mycelium processes cannot commit at
// the same moment.
//
// Bounded rather than indefinite. Waiting forever would let the journal hang a
// sync, which is the one thing it must never do. Giving up on the first attempt
// would drop a commit boundary on every collision, and those changes would then
// land in the next commit under a message describing something else.
func lockJournal(dataDir string) (func(), error) {
	release, err := lockfile.Take(dataDir, lockName, lockWait)
	if errors.Is(err, lockfile.ErrHeld) {
		return nil, fmt.Errorf("another mycelium process is writing the history")
	}
	return release, err
}
