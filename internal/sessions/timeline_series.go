package sessions

import (
	"sort"
	"time"
)

// timelineAxis is the bucket grid a series is laid out on.
type timelineAxis struct {
	labels []string
	index  map[string]int
	layout string
	bucket string
	start  time.Time
}

func newTimelineAxis(start, now time.Time, bucket string) timelineAxis {
	labels := bucketLabels(start, now, bucket)
	index := make(map[string]int, len(labels))
	for i, label := range labels {
		index[label] = i
	}
	return timelineAxis{
		labels: labels,
		index:  index,
		layout: bucketLayout(bucket),
		bucket: bucket,
		start:  start,
	}
}

// slotOf places a block on the grid. One landing outside it is clamped to the
// nearer end rather than dropped, so its time is still counted somewhere.
func (a timelineAxis) slotOf(b *Block) int {
	if slot, ok := a.index[truncateBucket(b.StartedAt, a.bucket).Format(a.layout)]; ok {
		return slot
	}
	if b.StartedAt.UTC().Before(a.start) {
		return 0
	}
	return len(a.labels) - 1
}

// timelineKey is the series a block belongs to, and whether it belongs to one
// at all: a block with no model is dropped from a by=model timeline, where
// every other grouping keeps it under "(none)".
func timelineKey(b *Block, by string) (string, bool) {
	if by == TotalKey {
		return AllKey, true
	}
	key := groupKey(b, by)
	if key != "" {
		return key, true
	}
	if by == "model" {
		return "", false
	}
	return "(none)", true
}

// accumulate sums each block into its group's bucket, keeping groups in
// first-seen order so ranking has a stable list to sort.
func accumulate(inRange []*Block, by string, a timelineAxis) []*timelineGroup {
	groups := make(map[string]*timelineGroup)
	var order []string
	for _, b := range inRange {
		key, ok := timelineKey(b, by)
		if !ok {
			continue
		}
		g := groups[key]
		if g == nil {
			g = &timelineGroup{key: key, cells: make([]timelineCell, len(a.labels))}
			groups[key] = g
			order = append(order, key)
		}
		seconds := int64(b.Duration().Seconds())
		cell := &g.cells[a.slotOf(b)]
		cell.seconds += seconds
		cell.sessions++
		cell.tokensIn += b.TokensIn + b.CacheWrite
		cell.tokensOut += b.TokensOut
		cell.cacheRead += b.CacheRead
		cell.costTotal += b.CostTotal
		g.total += seconds
	}
	ranked := make([]*timelineGroup, 0, len(groups))
	for _, key := range order {
		ranked = append(ranked, groups[key])
	}
	return ranked
}

// rankGroups orders series by active seconds, breaking ties on the key so the
// same data answers the same way twice.
func rankGroups(ranked []*timelineGroup) {
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].total != ranked[j].total {
			return ranked[i].total > ranked[j].total
		}
		return ranked[i].key < ranked[j].key
	})
}

// foldBeyondMax collapses everything past the colour cap into one trailing
// "Other" series.
func foldBeyondMax(ranked []*timelineGroup, buckets int) []*timelineGroup {
	if len(ranked) <= MaxSeries {
		return ranked
	}
	other := &timelineGroup{key: OtherKey, cells: make([]timelineCell, buckets)}
	for _, g := range ranked[MaxSeries-1:] {
		for i, c := range g.cells {
			other.cells[i].seconds += c.seconds
			other.cells[i].sessions += c.sessions
			other.cells[i].tokensIn += c.tokensIn
			other.cells[i].tokensOut += c.tokensOut
			other.cells[i].cacheRead += c.cacheRead
			other.cells[i].costTotal += c.costTotal
		}
		other.total += g.total
	}
	return append(ranked[:MaxSeries-1:MaxSeries-1], other)
}

// series turns a group's cells into the column-per-metric shape the dashboard
// reads.
func (g *timelineGroup) series(buckets int) TimelineSeries {
	s := TimelineSeries{
		Key:       g.key,
		Seconds:   make([]int64, buckets),
		Sessions:  make([]int, buckets),
		TokensIn:  make([]int64, buckets),
		TokensOut: make([]int64, buckets),
		CacheRead: make([]int64, buckets),
		CostTotal: make([]float64, buckets),
	}
	for i, c := range g.cells {
		s.Seconds[i] = c.seconds
		s.Sessions[i] = c.sessions
		s.TokensIn[i] = c.tokensIn
		s.TokensOut[i] = c.tokensOut
		s.CacheRead[i] = c.cacheRead
		s.CostTotal[i] = c.costTotal
	}
	return s
}
