package sessions

import (
	"sort"
	"time"
)

// MaxSeries bounds how many series a timeline answers with: muse's chartColor
// wraps past six, so a seventh series would reuse the first colour. Groups
// beyond the cap fold into a trailing "Other".
const MaxSeries = 6

// TotalKey is the group value that collapses every block into one series.
const TotalKey = "total"

// OtherKey names the folded remainder, AllKey the single by=total series.
const (
	OtherKey = "Other"
	AllKey   = "All"
)

// BucketKeys and TimelineGroupKeys are the accepted query values; a caller
// passing anything else falls back to the documented default.
var (
	BucketKeys        = []string{"day", "month"}
	TimelineGroupKeys = append(append([]string{}, GroupKeys...), TotalKey)
)

type TimelineSeries struct {
	Key       string  `json:"key"`
	Seconds   []int64 `json:"seconds"`
	Sessions  []int   `json:"sessions"`
	TokensIn  []int64 `json:"tokens_in"`
	TokensOut []int64 `json:"tokens_out"`
	CacheRead []int64 `json:"cache_read"`
}

type Series struct {
	Bucket string           `json:"bucket"`
	By     string           `json:"by"`
	Labels []string         `json:"labels"`
	Series []TimelineSeries `json:"series"`
}

func bucketLayout(bucket string) string {
	if bucket == "month" {
		return "2006-01"
	}
	return "2006-01-02"
}

func truncateBucket(t time.Time, bucket string) time.Time {
	t = t.UTC()
	if bucket == "month" {
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

func nextBucket(t time.Time, bucket string) time.Time {
	if bucket == "month" {
		return t.AddDate(0, 1, 0)
	}
	return t.AddDate(0, 0, 1)
}

func bucketLabels(start, end time.Time, bucket string) []string {
	layout := bucketLayout(bucket)
	labels := []string{}
	for t := start; !t.After(end); t = nextBucket(t, bucket) {
		labels = append(labels, t.Format(layout))
	}
	return labels
}

type timelineCell struct {
	seconds   int64
	sessions  int
	tokensIn  int64
	tokensOut int64
	cacheRead int64
}

type timelineGroup struct {
	key   string
	total int64
	cells []timelineCell
}

// Timeline buckets sessions over time, gap-filled so every bucket between the
// first one in range and today is present. Series are ranked by total active
// seconds, capped at MaxSeries with the remainder folded into "Other"; by=total
// collapses everything into a single "All" series.
func Timeline(blocks []Block, since time.Time, bucket string, by string) Series {
	if bucket != "month" {
		bucket = "day"
	}
	if by == "" {
		by = TotalKey
	}
	out := Series{Bucket: bucket, By: by, Labels: []string{}, Series: []TimelineSeries{}}

	var inRange []*Block
	for i := range blocks {
		b := &blocks[i]
		if !since.IsZero() && b.EndedAt.Before(since) {
			continue
		}
		inRange = append(inRange, b)
	}

	now := truncateBucket(time.Now(), bucket)
	var start time.Time
	switch {
	case !since.IsZero():
		start = truncateBucket(since, bucket)
	case len(inRange) > 0:
		start = truncateBucket(inRange[0].StartedAt, bucket)
		for _, b := range inRange {
			if s := truncateBucket(b.StartedAt, bucket); s.Before(start) {
				start = s
			}
		}
	default:
		return out
	}
	if start.After(now) {
		start = now
	}
	out.Labels = bucketLabels(start, now, bucket)
	if len(out.Labels) == 0 {
		return out
	}

	index := make(map[string]int, len(out.Labels))
	for i, label := range out.Labels {
		index[label] = i
	}
	layout := bucketLayout(bucket)

	groups := make(map[string]*timelineGroup)
	var order []string
	for _, b := range inRange {
		key := AllKey
		if by != TotalKey {
			key = groupKey(b, by)
			if key == "" {
				key = "(none)"
			}
		}
		g := groups[key]
		if g == nil {
			g = &timelineGroup{key: key, cells: make([]timelineCell, len(out.Labels))}
			groups[key] = g
			order = append(order, key)
		}
		slot, ok := index[truncateBucket(b.StartedAt, bucket).Format(layout)]
		if !ok {
			if b.StartedAt.UTC().Before(start) {
				slot = 0
			} else {
				slot = len(out.Labels) - 1
			}
		}
		seconds := int64(b.Duration().Seconds())
		cell := &g.cells[slot]
		cell.seconds += seconds
		cell.sessions++
		cell.tokensIn += b.TokensIn + b.CacheWrite
		cell.tokensOut += b.TokensOut
		cell.cacheRead += b.CacheRead
		g.total += seconds
	}

	ranked := make([]*timelineGroup, 0, len(groups))
	for _, key := range order {
		ranked = append(ranked, groups[key])
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].total != ranked[j].total {
			return ranked[i].total > ranked[j].total
		}
		return ranked[i].key < ranked[j].key
	})

	if len(ranked) > MaxSeries {
		other := &timelineGroup{key: OtherKey, cells: make([]timelineCell, len(out.Labels))}
		for _, g := range ranked[MaxSeries-1:] {
			for i, c := range g.cells {
				other.cells[i].seconds += c.seconds
				other.cells[i].sessions += c.sessions
				other.cells[i].tokensIn += c.tokensIn
				other.cells[i].tokensOut += c.tokensOut
				other.cells[i].cacheRead += c.cacheRead
			}
			other.total += g.total
		}
		ranked = append(ranked[:MaxSeries-1:MaxSeries-1], other)
	}

	for _, g := range ranked {
		s := TimelineSeries{
			Key:       g.key,
			Seconds:   make([]int64, len(out.Labels)),
			Sessions:  make([]int, len(out.Labels)),
			TokensIn:  make([]int64, len(out.Labels)),
			TokensOut: make([]int64, len(out.Labels)),
			CacheRead: make([]int64, len(out.Labels)),
		}
		for i, c := range g.cells {
			s.Seconds[i] = c.seconds
			s.Sessions[i] = c.sessions
			s.TokensIn[i] = c.tokensIn
			s.TokensOut[i] = c.tokensOut
			s.CacheRead[i] = c.cacheRead
		}
		out.Series = append(out.Series, s)
	}
	return out
}
