package journal

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

const (
	lockName = ".journal.lock"
	lockWait = 2 * time.Second
	lockPoll = 50 * time.Millisecond
)

// lockJournal takes an exclusive lock so two mycelium processes cannot commit at
// the same moment. The collision is between processes, not goroutines: the
// daemon does not call the sync in-process, it runs `mycelium sync` as a child
// roughly every 60 seconds, so what it lands on is whatever a human or an agent
// just typed. An in-process mutex cannot see that.
//
// Non-blocking with a bounded retry. Waiting forever would let the journal hang
// a sync, which is the one thing it must never do. Giving up on the first
// attempt would drop a commit boundary on every collision, and those changes
// would then land in the next commit under a message describing something else.
//
// The lock releases when the process exits, so a mycelium killed mid-commit
// leaves nothing for the next one to clear.
func lockJournal(dataDir string) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, lockName), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for waited := time.Duration(0); waited <= lockWait; waited += lockPoll {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() {
				syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		time.Sleep(lockPoll)
	}
	f.Close()
	return nil, fmt.Errorf("another mycelium process is writing the history")
}
