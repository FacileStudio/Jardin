package memory

import (
	"math"
	"strings"
	"time"
)

const (
	dayLayout = "2006-01-02"

	// freshnessHalfLife is how long a claim takes to lose half of the little
	// that recency is allowed to move. It is deliberately long. The corpus holds
	// conventions that were settled once and are still true, next to notes from
	// yesterday, and a decay tuned to promote the notes would bury the
	// conventions. The field's warning is exactly this: too aggressive loses
	// stable facts, too lax keeps stale ones forever.
	freshnessHalfLife = 180 * 24 * time.Hour

	// freshnessFloor is the most an old claim can be reduced to, and therefore
	// the width of the whole effect: scores move inside a 15% band and no
	// further. Recency breaks ties between chunks that already match about as
	// well; it never overturns a better match for a worse one. This is the
	// "decay the score, not the data" rule made numeric, and it is what keeps
	// the decay reversible, since nothing is deleted and the next confirmation
	// puts the score back.
	freshnessFloor = 0.85

	// supersededWeight is what a claim keeps once another names it as replaced.
	// Much harsher than age, because this is not a guess about staleness: a
	// second finding says outright that this one is wrong. It is still not zero,
	// so a query with no other answer can still reach it.
	supersededWeight = 0.5
)

// Date is when the claim in a chunk was last known to hold. It reads the
// metadata block first and the prose "**Date**:" line the wiki convention
// actually writes second.
//
// The fallback is not a nicety, it is the whole of the signal today: 315
// findings in the live wiki carry the prose line and zero carry the metadata
// block, so a ranker reading only the block would be measuring an empty set.
//
// A chunk with no date at all returns the zero time and is left alone rather
// than treated as ancient. Page preambles carry no date, and pushing every one
// of them down would be a ranking change dressed up as a freshness signal.
func (c Chunk) Date() time.Time {
	if day, ok := parseDay(c.Meta.Confirmed); ok {
		return day
	}
	if day, ok := parseDay(c.Meta.Date); ok {
		return day
	}
	return proseDate(c.Body)
}

func parseDay(value string) (time.Time, bool) {
	day, err := time.Parse(dayLayout, strings.TrimSpace(value))
	if err != nil {
		return time.Time{}, false
	}
	return day, true
}

// proseDate reads the date off a finding's "**Date**:" line, taking the latest
// one it carries. Five lines in the live wiki hold two, in the shape
// "2026-08-22, updated 2026-08-23", and in every one of them the later date is
// the one that says how current the claim is.
func proseDate(body string) time.Time {
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "**Date**") {
			continue
		}
		if day, ok := latestDay(line); ok {
			return day
		}
	}
	return time.Time{}
}

func latestDay(line string) (time.Time, bool) {
	var best time.Time
	for i := 0; i+len(dayLayout) <= len(line); i++ {
		if !looksLikeDay(line[i : i+len(dayLayout)]) {
			continue
		}
		day, ok := parseDay(line[i : i+len(dayLayout)])
		if ok && day.After(best) {
			best = day
		}
	}
	return best, !best.IsZero()
}

// looksLikeDay is a shape test that runs before the parser, so a long line
// costs a handful of parse attempts instead of one per character.
func looksLikeDay(s string) bool {
	if s[4] != '-' || s[7] != '-' {
		return false
	}
	for _, i := range []int{0, 1, 2, 3, 5, 6, 8, 9} {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// freshness is the multiplier a claim's age earns it: 1.0 for something written
// today, decaying by half every freshnessHalfLife toward freshnessFloor.
//
// Exponential rather than the bell curve that suits a date range, because a
// claim does not stop being true on a cliff. Old knowledge should fade and keep
// fading slowly, which is the long tail an exponential gives and a Gaussian
// does not.
func freshness(at, now time.Time) float64 {
	if at.IsZero() || !at.Before(now) {
		return 1
	}
	halfLives := now.Sub(at).Seconds() / freshnessHalfLife.Seconds()
	return freshnessFloor + (1-freshnessFloor)*math.Exp2(-halfLives)
}

// chunkWeight is everything the ranker knows about a claim other than whether
// it matches the words: whether something has replaced it, and how long ago it
// was written or last confirmed.
func chunkWeight(c Chunk, replaced map[string]bool, now time.Time) float64 {
	if c.Meta.ID != "" && replaced[c.Meta.ID] {
		return supersededWeight
	}
	return freshness(c.Date(), now)
}

// supersededIDs collects the identifiers that some other finding declares it
// replaces. The pointer runs forward, from the correction to the claim it
// corrects, so knowing which claims are dead means reading every chunk first.
//
// No page in the live wiki writes one yet. It costs one map to have this ready
// for the first that does, and until then the strikethrough convention in
// Text() is what actually carries supersession here.
func supersededIDs(chunks []Chunk) map[string]bool {
	replaced := map[string]bool{}
	for _, c := range chunks {
		for _, id := range strings.Split(c.Meta.Supersedes, ",") {
			if id = strings.TrimSpace(id); id != "" {
				replaced[id] = true
			}
		}
	}
	return replaced
}

// DayString renders a claim's date for a reader, and the empty string when it
// has none. Absolute rather than "4 months ago": it is what the page itself
// says, and it does not change between two runs of the same query.
func DayString(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.Format(dayLayout)
}
