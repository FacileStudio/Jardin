package flow

import (
	"os"
	"sort"
	"strings"
	"time"

	"github.com/FacileStudio/Jardin/internal/config"
)

// QueryOptions narrows a search across every flow's history. A zero value
// matches everything.
type QueryOptions struct {
	Flow   string
	Status string
	// Since is a cutoff, not a window: the zero time means no cutoff. It is
	// produced by sessions.ParseSince, which already understands "7d" and "all".
	Since time.Time
	Limit int
}

// Query answers questions that span flows — "which runs failed this week" is
// one command rather than one command per flow.
//
// It reads every artifact it considers. That is affordable because RunRetention
// bounds each flow's history; if lifetimes ever let that grow, this is the
// thing that needs an index rather than a bigger loop.
//
// Results come back newest first, ordered by the parsed start time: run IDs are
// RFC3339Nano, which drops trailing zeros and so does not sort
// lexicographically.
func Query(opts QueryOptions) ([]*Run, error) {
	names, err := flowsWithRuns()
	if err != nil {
		return nil, err
	}
	var found []*Run
	for _, name := range names {
		if opts.Flow != "" && name != opts.Flow {
			continue
		}
		runs, err := ListRuns(name, RunRetention)
		if err != nil {
			return nil, err
		}
		for _, r := range runs {
			if matches(r, opts) {
				found = append(found, r)
			}
		}
	}

	sort.SliceStable(found, func(i, j int) bool {
		return found[i].StartedAt.After(found[j].StartedAt)
	})
	if opts.Limit > 0 && len(found) > opts.Limit {
		found = found[:opts.Limit]
	}
	return found, nil
}

func matches(r *Run, opts QueryOptions) bool {
	if opts.Status != "" && !strings.EqualFold(r.Status, opts.Status) {
		return false
	}
	if !opts.Since.IsZero() && r.StartedAt.Before(opts.Since) {
		return false
	}
	return true
}

// flowsWithRuns lists the flows that have a history, which is not the same set
// as the flows that exist: a deleted flow keeps its runs until they age out,
// and those runs are still the answer to "what happened last week".
func flowsWithRuns() ([]string, error) {
	entries, err := os.ReadDir(config.RunsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
