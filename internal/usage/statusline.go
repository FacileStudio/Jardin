package usage

import (
	"encoding/json"
	"fmt"
	"io"
)

type statusLineBucket struct {
	UsedPercentage float64 `json:"used_percentage"`
	ResetsAt       int64   `json:"resets_at"`
}

type statusLinePayload struct {
	Model struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`
	RateLimits map[string]statusLineBucket `json:"rate_limits"`
}

// ParseStatusLine reads the JSON blob Claude Code pipes to a status-line
// command. rate_limits and every bucket inside it are optional; resets_at
// crosses the wire as Unix epoch seconds.
func ParseStatusLine(r io.Reader) (Snapshot, error) {
	var payload statusLinePayload
	if err := json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(&payload); err != nil {
		return Snapshot{Source: SourceStatusLine}, fmt.Errorf("decode status line payload: %w", err)
	}
	snapshot := Snapshot{
		Source:  SourceStatusLine,
		Model:   payload.Model.DisplayName,
		Windows: []Window{},
	}
	if len(payload.RateLimits) == 0 {
		return snapshot, ErrNoRateLimits
	}
	for key, bucket := range payload.RateLimits {
		snapshot.Windows = append(snapshot.Windows, Window{
			Key:            key,
			Label:          Label(key),
			UsedPercentage: bucket.UsedPercentage,
			ResetsAt:       epochToTime(bucket.ResetsAt),
		})
	}
	sortWindows(snapshot.Windows)
	return snapshot, nil
}
