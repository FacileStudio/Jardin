package lockfile

import (
	"errors"
	"testing"
	"time"
)

func TestASecondTakeIsRefusedUntilTheFirstReleases(t *testing.T) {
	dir := t.TempDir()

	release, err := Take(dir, ".test.lock", 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Take(dir, ".test.lock", 0); !errors.Is(err, ErrHeld) {
		t.Fatalf("err = %v, want ErrHeld while the first holder is still in", err)
	}

	release()
	again, err := Take(dir, ".test.lock", 0)
	if err != nil {
		t.Fatalf("the lock did not come back after release: %v", err)
	}
	again()
}

func TestADifferentNameIsADifferentLock(t *testing.T) {
	dir := t.TempDir()

	first, err := Take(dir, ".one.lock", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer first()

	second, err := Take(dir, ".two.lock", 0)
	if err != nil {
		t.Fatalf("two names collided on one lock: %v", err)
	}
	second()
}

func TestAWaitGivesUpRatherThanHanging(t *testing.T) {
	dir := t.TempDir()

	release, err := Take(dir, ".test.lock", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	start := time.Now()
	if _, err := Take(dir, ".test.lock", 150*time.Millisecond); !errors.Is(err, ErrHeld) {
		t.Fatalf("err = %v, want ErrHeld", err)
	}
	if waited := time.Since(start); waited < 150*time.Millisecond || waited > 2*time.Second {
		t.Errorf("waited %s, want roughly the 150ms asked for and then a refusal", waited)
	}
}
