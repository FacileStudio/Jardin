package usage

import (
	"testing"
	"time"
)

func TestReadCurrentAndHistory(t *testing.T) {
	dir := t.TempDir()
	base := time.Now().UTC().Truncate(time.Second)
	if err := Record(dir, "ruche", snap(50, base)); err != nil {
		t.Fatal(err)
	}
	if err := Record(dir, "lucy", snap(10, base)); err != nil {
		t.Fatal(err)
	}

	all, err := ReadCurrent(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].Machine != "lucy" {
		t.Fatalf("expected machine-sorted snapshots, got %+v", all)
	}

	h := History(dir, base.Add(-time.Hour), "")
	if len(h.Labels) != 2 || len(h.Series) != 1 {
		t.Fatalf("history shape wrong: %+v", h)
	}
	if len(h.Series[0].Values) != len(h.Labels) {
		t.Fatal("values must align to labels")
	}

	only := History(dir, base.Add(-time.Hour), "lucy")
	if len(only.Labels) != 1 {
		t.Fatalf("machine filter ignored: %+v", only)
	}
}

func TestReadCurrentEmpty(t *testing.T) {
	got, err := ReadCurrent(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("empty data must yield an empty slice, got %+v", got)
	}
	h := History(t.TempDir(), time.Time{}, "")
	if h.Labels == nil || h.Series == nil {
		t.Fatal("empty history must marshal as [] not null")
	}
}
