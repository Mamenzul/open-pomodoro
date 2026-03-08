package clock

import (
	"fmt"
	"strings"
	"time"
)

// FormatRemaining renders a duration as mm:ss (e.g. "04:35").
// Negative durations are clamped to "00:00".
func FormatRemaining(d time.Duration) string {
	if d <= 0 {
		return "00:00"
	}
	d = d.Round(time.Second)
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

// ParseAgo parses a "--ago" flag value like "5m" or "1h30m" and returns the
// adjusted start time (i.e. time.Now().Add(-ago)).
func ParseAgo(s string) (time.Time, error) {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --ago value %q: %w", s, err)
	}
	if d < 0 {
		return time.Time{}, fmt.Errorf("--ago must be a positive duration")
	}
	return time.Now().Add(-d), nil
}

// ParseWait parses a "--wait" flag value like "2m30s" and blocks until the
// duration elapses, calling tick every second with time remaining.
// Pass a nil tick to skip callbacks.
func ParseWait(s string, tick func(remaining time.Duration)) error {
	d, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid --wait value %q: %w", s, err)
	}
	if d <= 0 {
		return nil
	}

	deadline := time.Now().Add(d)
	t := time.NewTicker(time.Second)
	defer t.Stop()

	for range t.C {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		if tick != nil {
			tick(remaining)
		}
	}
	return nil
}

// FormatStatus renders the status format string, substituting template tags.
//
//	%s  mode emoji + label  e.g. "🍅 Work"
//	%r  remaining time      e.g. "22:10"
//	%c  cycle number        e.g. "2"
//	%C  daily completed     e.g. "5"
func FormatStatus(format, modeLabel, remaining string, cycle, dailyCount int) string {
	r := format
	r = strings.ReplaceAll(r, "%s", modeLabel)
	r = strings.ReplaceAll(r, "%r", remaining)
	r = strings.ReplaceAll(r, "%c", fmt.Sprintf("%d", cycle))
	r = strings.ReplaceAll(r, "%C", fmt.Sprintf("%d", dailyCount))
	return r
}
