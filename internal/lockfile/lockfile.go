// Package lockfile serialises the mycelium processes that write the same files.
//
// Every collision it exists for is between processes and never between
// goroutines: the daemon does not call the sync in-process, it runs
// "mycelium sync" as a child roughly every 60 seconds, and what that lands on
// is whatever a human or an agent just typed. An in-process mutex cannot see
// either party, which is why three separate write paths in this repository all
// reached for an OS lock and why they now reach for the same one.
package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// ErrHeld reports that another process holds the lock.
//
// Callers name the work in their own words rather than printing this: "another
// scan is already running" and "another mycelium process is writing the
// history" are the same condition and not the same sentence, and the sentence
// is the half that tells a reader what to do about it.
var ErrHeld = errors.New("lock held by another process")

// pollInterval is how often a bounded wait retries.
const pollInterval = 50 * time.Millisecond

// Take opens an exclusive lock on name inside dataDir and returns the call that
// releases it.
//
// name must be a dotfile. Sync excludes anything under the data directory
// whose path starts with a dot, so a lock named otherwise would travel to the
// server and to every other machine as an ordinary file.
//
// wait bounds the retry. Zero gives up on the first refusal, which suits work a
// second process is already doing and nobody needs done twice. A non-zero wait
// suits work that still has to happen once the other process is finished, and
// is bounded rather than indefinite because a lock that can hang a sync is
// worse than the race it closes.
//
// The lock releases when the process exits, so a mycelium killed mid-write
// leaves nothing for the next one to clear.
func Take(dataDir, name string, wait time.Duration) (func(), error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dataDir, name), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	for waited := time.Duration(0); ; waited += pollInterval {
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			return func() {
				syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
				f.Close()
			}, nil
		}
		if waited >= wait {
			break
		}
		time.Sleep(pollInterval)
	}
	f.Close()
	return nil, ErrHeld
}
