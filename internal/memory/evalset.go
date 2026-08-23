package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// EvalCase is one query and the page or pages that should answer it. The live
// golden set is a list of these.
type EvalCase struct {
	Query  string   `json:"query"`
	Expect []string `json:"expect"`
}

// EvalSet is the state of the live-wiki golden set on this machine.
//
// Named counts page references rather than distinct pages, because that is what
// the eval's own corpus guard counts and the two must agree. A set naming one
// page from two cases is two references.
type EvalSet struct {
	Present bool
	Cases   int
	Named   int
	Found   int
}

// EvalSetPath is where the live-wiki golden set lives. It is under the data
// directory rather than in the repository because the repository is public and
// the set describes, in plain English, every page of a private wiki. Here it
// reaches the machines that can run the eval through the sync the wiki already
// uses, and reaches nobody else.
func EvalSetPath(dataDir string) string {
	return filepath.Join(dataDir, "eval", "golden.json")
}

// InspectEvalSet reports whether the golden set still names pages this wiki
// holds. It exists because the eval skips itself when the corpus has moved on,
// which is correct behaviour that is invisible: the package still reports ok and
// the skip line scrolls past. That is how retrieval ranking changed twice
// between 2026-08-19 and 2026-08-23 with nothing measuring it.
//
// An absent set is not a failure. Most machines running mycelium never touch the
// ranker and will never have one.
func InspectEvalSet(dataDir string) (EvalSet, error) {
	path := EvalSetPath(dataDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return EvalSet{}, nil
		}
		return EvalSet{}, err
	}
	var cases []EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return EvalSet{}, fmt.Errorf("%s is not a list of eval cases: %w", path, err)
	}
	set := EvalSet{Present: true, Cases: len(cases)}
	memoryDir := filepath.Join(dataDir, "memory")
	for _, c := range cases {
		for _, want := range c.Expect {
			set.Named++
			if _, err := os.Stat(filepath.Join(memoryDir, filepath.FromSlash(want))); err == nil {
				set.Found++
			}
		}
	}
	return set, nil
}

// EvalCorpusFloor is the share of named pages that must still exist for the eval
// to measure anything, matching the eval's own corpus guard.
const EvalCorpusFloor = 0.25

// EvalMinCases is the smallest golden set the eval will run. Below it the eval
// fails outright rather than skipping, so it has to be modelled here too: the
// first version of this check knew only about EvalCorpusFloor and reported a
// 10-case set as healthy while the eval refused to run on it, which is the
// green-tick-measuring-nothing state the check exists to prevent.
const EvalMinCases = 50

// Unusable reports why the eval cannot run against this set, or empty when it
// can. It models both of the eval's guards, and both of its failure shapes: too
// few surviving pages makes the eval skip, too few cases makes it fail.
//
// Prefer this to reading the fields. Present, Cases, Found and Named each
// answer only part of the question.
func (s EvalSet) Unusable() string {
	if !s.Present {
		return ""
	}
	if s.Cases < EvalMinCases {
		return fmt.Sprintf("%d cases, under the %d the eval needs to run at all", s.Cases, EvalMinCases)
	}
	if s.Named == 0 {
		return "names no pages, so it measures nothing"
	}
	if share := float64(s.Found) / float64(s.Named); share < EvalCorpusFloor {
		return fmt.Sprintf("%d of %d pages present, under the %.0f%% floor, so the eval skips",
			s.Found, s.Named, EvalCorpusFloor*100)
	}
	return ""
}
