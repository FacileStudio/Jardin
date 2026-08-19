package sessions

import (
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

// TimelineSeries is one group's series across buckets.
type TimelineSeries struct {
	Key       string    `json:"key"`
	Seconds   []int64   `json:"seconds"`
	Sessions  []int     `json:"sessions"`
	TokensIn  []int64   `json:"tokens_in"`
	TokensOut []int64   `json:"tokens_out"`
	CacheRead []int64   `json:"cache_read"`
	CostTotal []float64 `json:"cost_total"`
}

// Series is the answer to a timeline query: labeled buckets with one series
// per group.
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
	costTotal float64
}

type timelineGroup struct {
	key   string
	total int64
	cells []timelineCell
}

// blocksInRange drops the blocks that ended before the window opened.
func blocksInRange(blocks []Block, since time.Time) []*Block {
	var inRange []*Block
	for i := range blocks {
		b := &blocks[i]
		if !since.IsZero() && b.EndedAt.Before(since) {
			continue
		}
		inRange = append(inRange, b)
	}
	return inRange
}

// timelineStart is the first bucket a series covers: the window's own start
// when one was given, otherwise the earliest block in range. It reports false
// when there is neither, which is an empty answer rather than an error.
func timelineStart(inRange []*Block, since time.Time, bucket string) (time.Time, bool) {
	if !since.IsZero() {
		return truncateBucket(since, bucket), true
	}
	if len(inRange) == 0 {
		return time.Time{}, false
	}
	start := truncateBucket(inRange[0].StartedAt, bucket)
	for _, b := range inRange {
		if s := truncateBucket(b.StartedAt, bucket); s.Before(start) {
			start = s
		}
	}
	return start, true
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

	inRange := blocksInRange(blocks, since)
	now := truncateBucket(time.Now(), bucket)
	start, ok := timelineStart(inRange, since, bucket)
	if !ok {
		return out
	}
	if start.After(now) {
		start = now
	}

	axis := newTimelineAxis(start, now, bucket)
	if len(axis.labels) == 0 {
		return out
	}
	out.Labels = axis.labels

	ranked := accumulate(inRange, by, axis)
	rankGroups(ranked)
	ranked = foldBeyondMax(ranked, len(axis.labels))
	for _, g := range ranked {
		out.Series = append(out.Series, g.series(len(axis.labels)))
	}
	return out
}
