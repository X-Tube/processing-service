package observability

import "time"

func DurationMillis(startedAt time.Time) int64 {
	return time.Since(startedAt).Milliseconds()
}
