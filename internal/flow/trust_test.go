package flow

import (
	"os"
	"testing"
)

func trustDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("DATA_DIR", dir)
	return dir
}

func flowFixture(name, checksum string) *Flow {
	return &Flow{Name: name, Checksum: checksum, Steps: []Step{{Name: "s", Run: "true"}}}
}

func TestUnknownFlowIsNotTrusted(t *testing.T) {
	trustDir(t)

	pinned, err := TrustedChecksum("ghost")
	if err != nil {
		t.Fatalf("missing store must not error: %v", err)
	}
	if pinned != "" {
		t.Fatalf("checksum = %q, want empty", pinned)
	}

	ok, err := IsTrusted(flowFixture("ghost", "sha256:aaa"))
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("an unpinned flow must not be trusted")
	}
}

func TestTrustThenIsTrusted(t *testing.T) {
	trustDir(t)
	f := flowFixture("deploy", "sha256:aaa")

	if err := Trust(f); err != nil {
		t.Fatal(err)
	}
	ok, err := IsTrusted(f)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("a freshly pinned flow must be trusted")
	}
}

func TestChangedChecksumBreaksTrust(t *testing.T) {
	trustDir(t)
	f := flowFixture("deploy", "sha256:aaa")
	if err := Trust(f); err != nil {
		t.Fatal(err)
	}

	f.Checksum = "sha256:bbb"
	ok, err := IsTrusted(f)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("edited flow bytes must revoke trust")
	}
}

func TestTrustPreservesOtherEntries(t *testing.T) {
	trustDir(t)
	first := flowFixture("first", "sha256:aaa")
	if err := Trust(first); err != nil {
		t.Fatal(err)
	}
	if err := Trust(flowFixture("second", "sha256:bbb")); err != nil {
		t.Fatal(err)
	}

	pinned, err := TrustedChecksum("first")
	if err != nil {
		t.Fatal(err)
	}
	if pinned != "sha256:aaa" {
		t.Fatalf("first pin = %q, want sha256:aaa", pinned)
	}
}

func TestUntrustRemovesAndTolerates(t *testing.T) {
	trustDir(t)
	f := flowFixture("deploy", "sha256:aaa")
	if err := Trust(f); err != nil {
		t.Fatal(err)
	}

	if err := Untrust("deploy"); err != nil {
		t.Fatal(err)
	}
	ok, err := IsTrusted(f)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("an untrusted flow must not stay trusted")
	}
	if err := Untrust("never-pinned"); err != nil {
		t.Fatalf("untrusting an absent flow must not error: %v", err)
	}
}

func TestCorruptStoreFailsClosed(t *testing.T) {
	trustDir(t)
	if err := os.WriteFile(TrustPath(), []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := TrustedChecksum("deploy"); err == nil {
		t.Fatal("a corrupt trust store must be an error, not an empty pin")
	}
	if _, err := IsTrusted(flowFixture("deploy", "sha256:aaa")); err == nil {
		t.Fatal("IsTrusted must surface a corrupt store")
	}
}

func TestTrustFileIsOwnerOnly(t *testing.T) {
	trustDir(t)
	if err := Trust(flowFixture("deploy", "sha256:aaa")); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(TrustPath())
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("mode = %04o, want 0600", mode)
	}
}
