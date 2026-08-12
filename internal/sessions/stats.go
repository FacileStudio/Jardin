package sessions

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// StatRow is one aggregation bucket of time and token usage.
type StatRow struct {
	Key       string  `json:"key"`
	Sessions  int     `json:"sessions"`
	Seconds   int64   `json:"seconds"`
	TokensIn  int64   `json:"tokens_in"`
	TokensOut int64   `json:"tokens_out"`
	CacheRead int64   `json:"cache_read"`
	CostTotal float64 `json:"cost_total"`
}

var GroupKeys = []string{"project", "machine", "agent", "branch", "model"}

func groupKey(b *Block, by string) string {
	switch by {
	case "machine":
		return b.Machine
	case "agent":
		return b.Agent
	case "branch":
		return b.Branch
	case "model":
		return b.Model
	default:
		return b.Project
	}
}

// Aggregate sums blocks since the given time into per-group rows.
func Aggregate(blocks []Block, since time.Time, by string) []StatRow {
	rows := make(map[string]*StatRow)
	for i := range blocks {
		b := &blocks[i]
		if !since.IsZero() && b.EndedAt.Before(since) {
			continue
		}
		key := groupKey(b, by)
		if key == "" {
			if by == "model" {
				continue
			}
			key = "(none)"
		}
		row := rows[strings.ToLower(key)]
		if row == nil {
			row = &StatRow{Key: key}
			rows[strings.ToLower(key)] = row
		}
		row.Sessions++
		row.Seconds += int64(b.Duration().Seconds())
		row.TokensIn += b.TokensIn + b.CacheWrite
		row.TokensOut += b.TokensOut
		row.CacheRead += b.CacheRead
		row.CostTotal += b.CostTotal
	}
	out := make([]StatRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Seconds != out[j].Seconds {
			return out[i].Seconds > out[j].Seconds
		}
		return out[i].Key < out[j].Key
	})
	return out
}

// Recent returns the newest blocks, capped at limit.
func Recent(blocks []Block, limit int) []Block {
	sorted := make([]Block, len(blocks))
	copy(sorted, blocks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].EndedAt.After(sorted[j].EndedAt) })
	if len(sorted) > limit {
		sorted = sorted[:limit]
	}
	return sorted
}

// ParseSince turns "7d", "30d", "12h", or "all" into a cutoff time; "all"
// yields the zero time (no cutoff).
func ParseSince(s string, now time.Time) (time.Time, error) {
	if s == "" || s == "all" {
		return time.Time{}, nil
	}
	if strings.HasSuffix(s, "d") {
		if days, err := strconv.Atoi(strings.TrimSuffix(s, "d")); err == nil && days > 0 {
			return now.AddDate(0, 0, -days), nil
		}
	}
	if d, err := time.ParseDuration(s); err == nil && d > 0 {
		return now.Add(-d), nil
	}
	return time.Time{}, fmt.Errorf("invalid since %q (use 7d, 30d, 12h, or all)", s)
}

// FormatDuration renders a duration for display.
func FormatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// FormatCost renders a cost for display.
func FormatCost(n float64) string {
	switch {
	case n >= 100:
		return fmt.Sprintf("$%.0f", n)
	case n >= 1:
		return fmt.Sprintf("$%.2f", n)
	case n >= 0.01:
		return fmt.Sprintf("$%.2f", n)
	default:
		return fmt.Sprintf("$%.4f", n)
	}
}

// FormatTokens renders a token count for display.
func FormatTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// Recap builds the context block injected at agent session start: the last
// sealed session for this project plus 7-day totals across machines, then the
// repo's active claims. Empty when the project has neither history nor claims.
func Recap(dataDir, project string, now time.Time) string {
	if project == "" {
		return ""
	}
	blocks, err := ReadBlocks(dataDir)
	if err != nil {
		return ""
	}
	for _, open := range LoadState(dataDir).Open {
		if open.Events > 0 {
			blocks = append(blocks, finalize(open))
		}
	}
	var mine []Block
	for _, b := range blocks {
		if strings.EqualFold(b.Project, project) {
			mine = append(mine, b)
		}
	}
	claims := ReadClaimsLive(dataDir, project, now)
	if len(mine) == 0 && len(claims) == 0 {
		return ""
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Mycelium session recap — %s\n", project)
	if len(mine) > 0 {
		last := Recent(mine, 1)[0]
		week := Aggregate(mine, now.Add(-7*24*time.Hour), "project")
		fmt.Fprintf(&sb, "Last agent session: %s on %s (%s", humanAgo(now.Sub(last.EndedAt)), last.Machine, last.Agent)
		if last.Branch != "" {
			fmt.Fprintf(&sb, ", branch %s", last.Branch)
		}
		fmt.Fprintf(&sb, ", %s active, %s tokens out)\n", FormatDuration(last.Duration()), FormatTokens(last.TokensOut))
		if len(week) > 0 {
			w := week[0]
			fmt.Fprintf(&sb, "Past 7 days, all machines: %d sessions, %s active, %s tokens out\n",
				w.Sessions, FormatDuration(time.Duration(w.Seconds)*time.Second), FormatTokens(w.TokensOut))
		}
	}
	recapClaims(&sb, claims, now)
	fmt.Fprintf(&sb, "Wiki gates apply: run `mycelium memory search \"%s\"` before starting; write findings back when done.", project)
	return sb.String()
}

// recapClaims renders the repo's active claims into sb, live markers resolved
// against now, so a freshly-started agent sees in-flight work before it starts.
func recapClaims(sb *strings.Builder, claims []ClaimEntry, now time.Time) {
	if len(claims) == 0 {
		return
	}
	fmt.Fprintf(sb, "Active work on this repo:\n")
	for _, c := range claims {
		mark := "○"
		switch {
		case c.Live:
			mark = "●"
		case c.MachineOnline:
			mark = "◐"
		}
		fmt.Fprintf(sb, "  %s %s/%s", mark, c.Machine, c.Agent)
		if c.Branch != "" {
			fmt.Fprintf(sb, " (branch %s)", c.Branch)
		}
		fmt.Fprintf(sb, " since %s — %s\n", humanAgo(now.Sub(c.StartedAt)), c.Task)
		for _, line := range claimBodyLines(c.Body, 3) {
			fmt.Fprintf(sb, "      %s\n", line)
		}
	}
}

// claimBodyLines returns up to n non-empty body lines, trimmed, for surfacing
// the scratchpad in the injected context without flooding it.
func claimBodyLines(body string, n int) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= n {
			break
		}
	}
	return out
}

func humanAgo(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}
